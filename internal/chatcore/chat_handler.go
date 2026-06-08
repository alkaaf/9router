package chatcore

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// ChatHandlerOptions is the functional config for the
// POST /v1/chat/completions handler.
type ChatHandlerOptions struct {
	// ComboLookup returns a combo by name (nil, nil = not found).
	ComboLookup ComboLookup
	// CredentialsSelector selects the credentials for a model
	// request. It returns (creds, rate-limit-state, updateFn).
	CredentialsSelector func(ctx context.Context, provider, model string) (Credentials, RateLimitState, func(), error)
	// Executor sends the model request upstream. The context
	// may be cancelled by the disconnect watcher.
	Executor func(ctx context.Context, creds Credentials, req ChatRequest, stream bool) (*Response, error)
	// StreamResponse writes an SSE streamed response. For
	// non-streaming requests the handler returns JSON.
	StreamResponse func(w http.ResponseWriter, resp *Response)
	// JSONResponse writes a single JSON response (non-streaming).
	JSONResponse func(w http.ResponseWriter, resp *Response)
	// APIKeyValidator validates the provided apiKey. Returns nil
	// on success and an error on failure.
	APIKeyValidator func(ctx context.Context, apiKey string) error
	// StickyLimit is the round-robin rotation count for combos. A
	// value of 1 rotates on every call.
	StickyLimit int
	// CCFilterNaming enables the naming bypass.
	CCFilterNaming bool
}

// ChatHandler returns an http.HandlerFunc for POST
// /v1/chat/completions. The handler is framework-agnostic and only
// requires net/http. It honours stream=true for SSE streaming.
func ChatHandler(opts ChatHandlerOptions) http.HandlerFunc {
	rotator := NewComboRotator()
	usage := NewUsageTracker()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		// Step 1 — parse body.
		req, err := ParseRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
			return
		}

		// Step 2 — extract API key from Authorization header.
		apiKey := extractBearerToken(r)
		if apiKey == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Missing API key"})
			return
		}

		// Step 3 — validate API key (optional; when no validator
		// is configured, requests are accepted in test mode).
		if opts.APIKeyValidator != nil {
			if err := opts.APIKeyValidator(r.Context(), apiKey); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
				return
			}
		}

		// Step 4 — validate model field.
		modelStr, _ := req.Body["model"].(string)
		if strings.TrimSpace(modelStr) == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Missing model"})
			return
		}

		// Step 5 — bypass detection runs before any routing.
		if bp := CheckBypass(req.Body, r.Header.Get("User-Agent"), opts.CCFilterNaming); bp != nil && bp.Response != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(bp.Response)
			return
		}

		// Step 6 — model resolution: combo → list, else single.
		models, comboErr := GetComboModels(modelStr, opts.ComboLookup)
		if comboErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "combo resolution error"})
			return
		}
		if models == nil {
			models = []string{modelStr}
		}

		// Step 7 — resolve provider from the first model.
		firstModel := models[0]
		info := ResolveModel(firstModel, nil)
		provider := info.Provider
		if provider == "" {
			// Provider could not be inferred — treat as bad model.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "could not resolve provider for " + firstModel})
			return
		}

		stream, _ := req.Body["stream"].(bool)

		creds, rateLimit, updateCreds, credErr := opts.CredentialsSelector(r.Context(), provider, firstModel)
		if credErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "no available credentials"})
			return
		}
		if rateLimit.AllRateLimited {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Header().Set("Retry-After", rateLimit.RetryAfterHuman)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":         "all accounts rate limited",
				"retryAfter":    rateLimit.RetryAfterHuman,
				"lastError":     rateLimit.LastError,
				"lastErrorCode": rateLimit.LastErrorCode,
			})
			return
		}

		// Step 8 — round-robin rotation for combos.
		rotatedModels := models
		if len(models) > 1 && opts.ComboLookup != nil {
			rotatedModels = rotator.Next(modelStr, models, opts.StickyLimit)
		}

		// Step 9 — disconnect detection.
		ctx, upCancel := context.WithCancel(r.Context())
		defer upCancel()
		dw := NewDisconnectWatcher(ctx, upCancel, nil, func() {
			usage.Track(firstModel, provider, creds.ConnectionID, false)
		})

		// Step 10 — usage start.
		usage.Track(firstModel, provider, creds.ConnectionID, true)

		// Step 11 — execute with fallback for combos.
		var resp *Response
		var execErr error
		if len(rotatedModels) > 1 {
			cf := NewComboFallback(func(ctx context.Context, m string) (*Response, error) {
				return opts.Executor(ctx, creds, *req, stream)
			}, WithCooldownOn503(5000))
			resp, execErr = cf.Execute(ctx, rotatedModels)
		} else {
			resp, execErr = opts.Executor(ctx, creds, *req, stream)
		}

		_ = updateCreds // caller may persist via adapter

		// Step 12 — usage end.
		usage.Track(firstModel, provider, creds.ConnectionID, false)
		dw.MarkComplete()

		if execErr != nil {
			if IsDisconnectErr(execErr) {
				// Client gone — no body to write.
				return
			}
			var ce *ComboError
			w.Header().Set("Content-Type", "application/json")
			if errors.As(execErr, &ce) && !ce.Retryable {
				_ = json.NewEncoder(w).Encode(map[string]string{"error": ce.Err.Error()})
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": execErr.Error()})
			}
			return
		}

		// Step 13 — write response.
		if stream && opts.StreamResponse != nil {
			opts.StreamResponse(w, resp)
			return
		}
		if opts.JSONResponse != nil {
			opts.JSONResponse(w, resp)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-gen",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   resp.Model,
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": string(resp.Body)},
				"finish_reason": "stop",
			}},
		})
	}
}

// extractBearerToken returns the token from the Authorization:
// Bearer <token> header, or an empty string.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
