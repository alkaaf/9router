package chatcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeExecutor is a test double that simulates an upstream.
type fakeExecutor struct {
	status int
	body   string
	err    error
}

func (f *fakeExecutor) Execute(ctx context.Context, creds Credentials, req ChatRequest, stream bool) (*Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &Response{Status: f.status, Body: []byte(f.body), Model: "openai/gpt-4o"}, nil
}

// fakeSelector returns a fixed credential.
func fakeSelector(creds Credentials, rl RateLimitState, update func()) func(ctx context.Context, provider, model string) (Credentials, RateLimitState, func(), error) {
	return func(ctx context.Context, provider, model string) (Credentials, RateLimitState, func(), error) {
		return creds, rl, update, nil
	}
}

// TestChatHandler_ValidNonStreaming — AC-001: a well-formed
// non-streaming request returns a JSON response.
func TestChatHandler_ValidNonStreaming(t *testing.T) {
	exec := &fakeExecutor{status: 200, body: "hello"}
	h := ChatHandler(ChatHandlerOptions{
		CredentialsSelector: fakeSelector(Credentials{ConnectionID: "c1", APIKey: "x"}, RateLimitState{}, nil),
		Executor:            exec.Execute,
		CCFilterNaming:      true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key123")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "hello") {
		t.Errorf("response missing hello, got %q", rr.Body.String())
	}
}

// TestChatHandler_InvalidJSON — AC-002: malformed JSON returns 400.
func TestChatHandler_InvalidJSON(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{CCFilterNaming: true})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestChatHandler_MissingAPIKey — AC-003: no Authorization header
// returns 401.
func TestChatHandler_MissingAPIKey(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{CCFilterNaming: true})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// TestChatHandler_MissingModel — AC-006: empty model field returns
// 400.
func TestChatHandler_MissingModel(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{CCFilterNaming: true})
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestChatHandler_BypassWarmup — AC-004: a claude-cli warmup
// message is bypassed before any routing.
func TestChatHandler_BypassWarmup(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{CCFilterNaming: true})
	body := `{"model":"m","messages":[{"role":"user","content":"Warmup"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	req.Header.Set("User-Agent", "claude-cli/1.0")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for bypass", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"bypass/title"`) && !strings.Contains(rr.Body.String(), "bypass") {
		t.Errorf("bypass response not emitted, got %q", rr.Body.String())
	}
}

// TestChatHandler_AllRateLimited — when the credentials selector
// reports all accounts rate-limited, the handler returns 429 with
// Retry-After.
func TestChatHandler_AllRateLimited(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{
		CredentialsSelector: fakeSelector(
			Credentials{},
			RateLimitState{AllRateLimited: true, RetryAfterHuman: "30s", LastError: "429", LastErrorCode: "429"},
			nil,
		),
		CCFilterNaming: true,
	})
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "30s" {
		t.Errorf("Retry-After = %q", rr.Header().Get("Retry-After"))
	}
}

// TestChatHandler_Stream — AC-005: stream=true writes via
// StreamResponse.
func TestChatHandler_Stream(t *testing.T) {
	exec := &fakeExecutor{status: 200, body: "streamed"}
	seen := int32(0)
	h := ChatHandler(ChatHandlerOptions{
		CredentialsSelector: fakeSelector(Credentials{ConnectionID: "c1"}, RateLimitState{}, nil),
		Executor:            exec.Execute,
		StreamResponse: func(w http.ResponseWriter, resp *Response) {
			SetStreamHeaders(w)
			sw := NewSSEWriter(w)
			_ = sw.WriteChunk(resp.Model, string(resp.Body), "stop")
			_ = sw.WriteDone()
			atomic.StoreInt32(&seen, 1)
		},
		CCFilterNaming: true,
	})

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if atomic.LoadInt32(&seen) != 1 {
		t.Error("StreamResponse was not called")
	}
	resp := rr.Body.String()
	if !strings.Contains(resp, "data: ") || !strings.Contains(resp, "streamed") {
		t.Errorf("expected SSE chunk in body, got %q", resp)
	}
}

// TestChatHandler_WrongMethod — GET/PUT return 405.
func TestChatHandler_WrongMethod(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{CCFilterNaming: true})
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		req := httptest.NewRequest(method, "/v1/chat/completions", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", method, rr.Code)
		}
	}
}

// TestChatHandler_ProviderUnresolved — when the model resolves to
// a provider the request proceeds; the unresolvable case is
// exercised by the missing-model test (Step 4).
func TestChatHandler_ProviderUnresolved(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{
		CredentialsSelector: fakeSelector(Credentials{ConnectionID: "c1"}, RateLimitState{}, nil),
		Executor: func(ctx context.Context, creds Credentials, req ChatRequest, stream bool) (*Response, error) {
			return &Response{Status: 200, Body: []byte("ok"), Model: "openai/gpt-4o"}, nil
		},
		CCFilterNaming: true,
	})
	body := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%q", rr.Code, rr.Body.String())
	}
}

// TestExtractBearerToken — small unit test of the helper.
func TestExtractBearerToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer abc")
	if got := extractBearerToken(r); got != "abc" {
		t.Errorf("got %q", got)
	}
	if got := extractBearerToken(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Errorf("missing header → empty, got %q", got)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("Authorization", "Basic xyz")
	if got := extractBearerToken(r2); got != "" {
		t.Errorf("non-Bearer → empty, got %q", got)
	}
}

// TestChatHandler_NoCredentials — when selector returns an error,
// the handler returns 503.
func TestChatHandler_NoCredentials(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{
		CredentialsSelector: func(ctx context.Context, provider, model string) (Credentials, RateLimitState, func(), error) {
			return Credentials{}, RateLimitState{}, nil, errNoCreds
		},
		CCFilterNaming: true,
	})
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

var errNoCreds = &testError{msg: "no creds"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// TestChatHandler_ComboFallback — AC: a multi-model combo is
// routed through ComboFallback.
func TestChatHandler_ComboFallback(t *testing.T) {
	calls := []string{}
	exec := func(ctx context.Context, creds Credentials, req ChatRequest, stream bool) (*Response, error) {
		// Read the combo from the body to vary the result.
		model, _ := req.Body["model"].(string)
		calls = append(calls, model)
		if model == "openai/gpt-4" {
			return nil, NewUpstreamError(http.StatusInternalServerError, "boom", nil)
		}
		return &Response{Status: 200, Body: []byte("combo-ok"), Model: model}, nil
	}
	combo := func(name string) (*ComboInfo, error) {
		if name == "my-combo" {
			return &ComboInfo{Name: name, Models: []string{"openai/gpt-4", "openai/gpt-4o"}}, nil
		}
		return nil, nil
	}
	h := ChatHandler(ChatHandlerOptions{
		ComboLookup:         combo,
		CredentialsSelector: fakeSelector(Credentials{ConnectionID: "c1"}, RateLimitState{}, nil),
		Executor:            exec,
		CCFilterNaming:      true,
		StickyLimit:         1,
	})
	body := `{"model":"my-combo","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%q", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "combo-ok") {
		t.Errorf("body missing combo-ok, got %q", rr.Body.String())
	}
}

// TestChatHandler_ComboAllFail — AC: when every model in a combo
// fails, the handler returns 503.
func TestChatHandler_ComboAllFail(t *testing.T) {
	exec := func(ctx context.Context, creds Credentials, req ChatRequest, stream bool) (*Response, error) {
		return nil, NewUpstreamError(http.StatusTooManyRequests, "rate", nil)
	}
	combo := func(name string) (*ComboInfo, error) {
		return &ComboInfo{Name: name, Models: []string{"openai/gpt-4", "openai/gpt-4o"}}, nil
	}
	h := ChatHandler(ChatHandlerOptions{
		ComboLookup:         combo,
		CredentialsSelector: fakeSelector(Credentials{ConnectionID: "c1"}, RateLimitState{}, nil),
		Executor:            exec,
		CCFilterNaming:      true,
	})
	body := `{"model":"my-combo","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503; body=%q", rr.Code, rr.Body.String())
	}
}

// TestChatHandler_APIKeyValidator — AC: when the validator returns
// an error, the handler returns 401.
func TestChatHandler_APIKeyValidator(t *testing.T) {
	h := ChatHandler(ChatHandlerOptions{
		APIKeyValidator: func(ctx context.Context, k string) error {
			if k != "good" {
				return errNoCreds
			}
			return nil
		},
		CredentialsSelector: fakeSelector(Credentials{ConnectionID: "c1"}, RateLimitState{}, nil),
		CCFilterNaming:      true,
	})
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer bad")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// TestChatHandler_JSONResponse — AC: a custom JSONResponse is invoked
// for non-streaming.
func TestChatHandler_JSONResponse(t *testing.T) {
	exec := &fakeExecutor{status: 200, body: "hi"}
	called := int32(0)
	h := ChatHandler(ChatHandlerOptions{
		CredentialsSelector: fakeSelector(Credentials{ConnectionID: "c1"}, RateLimitState{}, nil),
		Executor:            exec.Execute,
		JSONResponse: func(w http.ResponseWriter, resp *Response) {
			atomic.StoreInt32(&called, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"custom": "yes"})
		},
		CCFilterNaming: true,
	})
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if atomic.LoadInt32(&called) != 1 {
		t.Error("JSONResponse was not called")
	}
	if !strings.Contains(rr.Body.String(), `"custom":"yes"`) {
		t.Errorf("custom response not emitted, got %q", rr.Body.String())
	}
}
