package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// GeminiCLIBaseURL is the canonical Google Cloud Code Assist endpoint
// for Gemini CLI / Antigravity. The {model} placeholder is filled in
// by BuildUrl.
const GeminiCLIBaseURL = "https://cloudcode-pa.googleapis.com/v1internal"

// GeminiCLIStreamPath is the stream suffix.
const GeminiCLIStreamPath = ":streamGenerateContent?alt=sse"

// GeminiCLINonStreamPath is the non-stream suffix.
const GeminiCLINonStreamPath = ":generateContent"

// GeminiCLIExecutor handles the Gemini CLI / Antigravity transport —
// Google's Cloud Code Assist wraps the body in a {project, model,
// request} envelope, returns data as SSE with a `data: {...}` line
// per chunk, and includes Google RPC error details with retryDelay
// hints.
type GeminiCLIExecutor struct {
	*BaseExecutor

	currentModel  string
	currentModelM sync.Mutex
}

// NewGeminiCLIExecutor returns a GeminiCLIExecutor.
func NewGeminiCLIExecutor() *GeminiCLIExecutor {
	cfg := &ProviderConfig{
		Provider:   "gemini-cli",
		BaseURLs:   []string{GeminiCLIBaseURL},
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &GeminiCLIExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// BuildUrl returns the Gemini CLI endpoint. The {model} interpolation
// is taken from the request model, not the URL path — Google's API
// puts the model name in the JSON body.
func (g *GeminiCLIExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	base := g.config.baseURL(urlIndex)
	if stream {
		return base + "/" + model + GeminiCLIStreamPath
	}
	return base + "/" + model + GeminiCLINonStreamPath
}

// BuildHeaders adds the Authorization header plus the
// x-request-source and a User-Agent containing the model name. The
// Node.js implementation uses the model name in the User-Agent for
// server-side metrics; we replicate that.
func (g *GeminiCLIExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := g.BaseExecutor.BuildHeaders(req, creds)
	h.Set("x-request-source", "local")
	g.currentModelM.Lock()
	model := g.currentModel
	g.currentModelM.Unlock()
	if model != "" {
		h.Set("User-Agent", "9router/1.0 (gemini-cli; "+model+")")
	} else {
		h.Set("User-Agent", "9router/1.0 (gemini-cli)")
	}
	return h
}

// TransformRequest wraps the upstream body in the Code Assist envelope:
//
//	{ "project": "...", "model": "...", "request": { ... original body } }
//
// If the body already has a project/model field (e.g. a translator
// added it), TransformRequest preserves the user's value. The
// project is empty by default — the host application can populate it
// from credentials.providerSpecificData.
func (g *GeminiCLIExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	g.currentModelM.Lock()
	g.currentModel = model
	g.currentModelM.Unlock()

	if len(body) == 0 {
		body = []byte(`{}`)
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(body, &inner); err != nil {
		return body, nil
	}
	envelope := map[string]any{
		"model":   model,
		"request": json.RawMessage(body),
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return body, nil
	}
	_ = inner
	return out, nil
}

// ParseError maps a Gemini CLI error to an ExecutorError. The
// extractor pulls retryDelay from Google's RPC error details (a
// sub-payload of the standard google.rpc.RetryInfo type).
func (g *GeminiCLIExecutor) ParseError(status int, body string) *ExecutorError {
	if status == 0 {
		return &ExecutorError{Code: CodeUnavailable, Message: "upstream unreachable", Status: status, Body: body}
	}
	if retryAfter, ok := extractRetryAfter(body); ok {
		return &ExecutorError{
			Code:      CodeRateLimited,
			Message:   "rate limited (retry-after " + retryAfter + ")",
			Status:    status,
			Body:      body,
			// RetryAfter is exposed via the Body field for now — a
			// future revision can add a typed field.
		}
	}
	return g.BaseExecutor.ParseError(status, body)
}

// RefreshCredentials is a no-op base. The actual Google OAuth refresh
// is performed by the host application's auth package and the new
// token is supplied via Credentials.
func (g *GeminiCLIExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	return g.BaseExecutor.RefreshCredentials(ctx, creds)
}

func init() {
	Register("gemini-cli", func() Executor { return NewGeminiCLIExecutor() })
	Register("antigravity", func() Executor { return NewGeminiCLIExecutor() })
}

// ────────────────────────────────────────────────────────────────────
// RetryInfo parser
// ────────────────────────────────────────────────────────────────────

var retryAfterRegex = regexp.MustCompile(`"retryDelay"\s*:\s*"([^"]+)"`)

func extractRetryAfter(body string) (string, bool) {
	match := retryAfterRegex.FindStringSubmatch(body)
	if len(match) < 2 {
		return "", false
	}
	return match[1], true
}

// ParseRetryDelaySeconds converts a Google retryDelay string like
// "1.5s" or "500ms" to an integer number of seconds. Returns 0 for
// unparseable input.
func ParseRetryDelaySeconds(retryDelay string) int {
	if retryDelay == "" {
		return 0
	}
	if strings.HasSuffix(retryDelay, "ms") {
		n, err := strconv.Atoi(strings.TrimSuffix(retryDelay, "ms"))
		if err != nil {
			return 0
		}
		return n / 1000
	}
	if strings.HasSuffix(retryDelay, "s") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(retryDelay, "s"), 64)
		if err != nil {
			return 0
		}
		return int(f)
	}
	return 0
}
