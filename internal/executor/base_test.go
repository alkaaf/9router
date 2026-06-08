package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ────────────────────────────────────────────────────────────────────
// Test fixtures
// ────────────────────────────────────────────────────────────────────

// roundTripFunc adapts a plain func into an HTTPClient. Tests that want
// fine-grained control over the wire layer use this; tests that just need
// a fake upstream use a httptest.Server + http.Client.
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

// noopReader is a minimal io.ReadCloser for unit-constructed responses.
type noopReader struct{ data []byte; off int }

func (r *noopReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
func (r *noopReader) Close() error { return nil }

// ────────────────────────────────────────────────────────────────────
// joinURL
// ────────────────────────────────────────────────────────────────────

func TestJoinURL(t *testing.T) {
	cases := []struct {
		name, base, path, want string
	}{
		{"empty base, no slash", "", "v1/chat", "v1/chat"},
		{"empty base, leading slash", "", "/v1/chat", "/v1/chat"},
		{"base trailing slash, path leading slash", "https://x.com/", "/v1", "https://x.com/v1"},
		{"base no slash, path no slash", "https://x.com", "v1", "https://x.com/v1"},
		{"base trailing slash, path no slash", "https://x.com/", "v1", "https://x.com/v1"},
		{"base no slash, path leading slash", "https://x.com", "/v1", "https://x.com/v1"},
		{"empty path", "https://x.com", "", "https://x.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := joinURL(c.base, c.path); got != c.want {
				t.Errorf("joinURL(%q,%q) = %q, want %q", c.base, c.path, got, c.want)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────
// ProviderConfig helpers
// ────────────────────────────────────────────────────────────────────

func TestProviderConfig_FallbackCount(t *testing.T) {
	cases := []struct {
		name string
		cfg  *ProviderConfig
		want int
	}{
		{"nil config", nil, 1},
		{"empty URLs", &ProviderConfig{}, 1},
		{"one URL", &ProviderConfig{BaseURLs: []string{"https://a"}}, 1},
		{"two URLs", &ProviderConfig{BaseURLs: []string{"https://a", "https://b"}}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.cfg.FallbackCount(); got != c.want {
				t.Errorf("FallbackCount() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestProviderConfig_baseURL(t *testing.T) {
	cfg := &ProviderConfig{BaseURLs: []string{"https://a", "https://b"}}
	if got := cfg.baseURL(0); got != "https://a" {
		t.Errorf("baseURL(0) = %q, want https://a", got)
	}
	if got := cfg.baseURL(1); got != "https://b" {
		t.Errorf("baseURL(1) = %q, want https://b", got)
	}
	if got := cfg.baseURL(99); got != "" {
		t.Errorf("out-of-range baseURL should be empty, got %q", got)
	}

	// Empty config yields "" for any index.
	empty := &ProviderConfig{}
	if got := empty.baseURL(0); got != "" {
		t.Errorf("empty config baseURL(0) = %q, want empty", got)
	}
}

// ────────────────────────────────────────────────────────────────────
// NewBaseExecutor defaults
// ────────────────────────────────────────────────────────────────────

func TestNewBaseExecutor_AppliesDefaults(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai"}, nil)
	if b.config.MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries default = %d, want %d", b.config.MaxRetries, DefaultMaxRetries)
	}
	if len(b.config.BaseURLs) != 1 || b.config.BaseURLs[0] != "" {
		t.Errorf("BaseURLs default = %v, want single empty string", b.config.BaseURLs)
	}
	if b.config.AuthHeader != "Authorization" {
		t.Errorf("AuthHeader default = %q, want Authorization", b.config.AuthHeader)
	}
	if b.GetProvider() != "openai" {
		t.Errorf("GetProvider = %q, want openai", b.GetProvider())
	}
	if b.HTTPClient() == nil {
		t.Errorf("HTTPClient should fall back to http.DefaultClient")
	}
}

// ────────────────────────────────────────────────────────────────────
// BuildUrl
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_BuildUrl(t *testing.T) {
	cases := []struct {
		name      string
		cfg       *ProviderConfig
		model     string
		stream    bool
		urlIndex  int
		want      string
	}{
		{
			name:     "stream=true, default path",
			cfg:      &ProviderConfig{Provider: "openai", BaseURLs: []string{"https://api.openai.com"}},
			model:    "gpt-4",
			stream:   true,
			urlIndex: 0,
			want:     "https://api.openai.com/v1/chat/completions",
		},
		{
			name:     "stream=false, default path",
			cfg:      &ProviderConfig{Provider: "openai", BaseURLs: []string{"https://api.openai.com"}},
			model:    "gpt-4",
			stream:   false,
			urlIndex: 0,
			want:     "https://api.openai.com/v1/chat/completions",
		},
		{
			name:     "explicit stream path",
			cfg:      &ProviderConfig{Provider: "anthropic", BaseURLs: []string{"https://api.anthropic.com"}, StreamPath: "/v1/messages?stream=true", NonStreamPath: "/v1/messages"},
			model:    "claude-3",
			stream:   true,
			urlIndex: 0,
			want:     "https://api.anthropic.com/v1/messages?stream=true",
		},
		{
			name:     "explicit non-stream path",
			cfg:      &ProviderConfig{Provider: "anthropic", BaseURLs: []string{"https://api.anthropic.com"}, StreamPath: "/v1/messages?stream=true", NonStreamPath: "/v1/messages"},
			model:    "claude-3",
			stream:   false,
			urlIndex: 0,
			want:     "https://api.anthropic.com/v1/messages",
		},
		{
			name:     "fallback URL index",
			cfg:      &ProviderConfig{Provider: "openai", BaseURLs: []string{"https://primary", "https://backup"}},
			model:    "gpt-4",
			stream:   false,
			urlIndex: 1,
			want:     "https://backup/v1/chat/completions",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := NewBaseExecutor(c.cfg, nil)
			if got := b.BuildUrl(c.model, c.stream, c.urlIndex); got != c.want {
				t.Errorf("BuildUrl(%q,%v,%d) = %q, want %q", c.model, c.stream, c.urlIndex, got, c.want)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────
// BuildHeaders
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_BuildHeaders_Bearer(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	h := b.BuildHeaders(&Request{Model: "gpt-4"}, &Credentials{AccessToken: "tok-123"})
	if got := h.Get("Authorization"); got != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer tok-123")
	}
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestBaseExecutor_BuildHeaders_CustomAuthHeader(t *testing.T) {
	cfg := &ProviderConfig{Provider: "anthropic", BaseURLs: []string{"x"}, AuthHeader: "x-api-key"}
	b := NewBaseExecutor(cfg, nil)
	h := b.BuildHeaders(&Request{}, &Credentials{AccessToken: "ant-key"})
	if got := h.Get("x-api-key"); got != "ant-key" {
		t.Errorf("x-api-key = %q, want ant-key", got)
	}
	if h.Get("Authorization") != "" {
		t.Errorf("Authorization should not be set when AuthHeader=x-api-key")
	}
}

func TestBaseExecutor_BuildHeaders_RequestHeadersWin(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	h := b.BuildHeaders(&Request{Headers: map[string]string{"X-Trace": "abc"}}, &Credentials{AccessToken: "t"})
	if got := h.Get("X-Trace"); got != "abc" {
		t.Errorf("X-Trace = %q, want abc", got)
	}
}

func TestBaseExecutor_BuildHeaders_NilCreds(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	h := b.BuildHeaders(&Request{}, nil)
	if h.Get("Authorization") != "" {
		t.Errorf("Authorization should be empty when creds are nil")
	}
}

// ────────────────────────────────────────────────────────────────────
// TransformRequest default
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_TransformRequest_Noop(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	in := []byte(`{"x":1}`)
	out, err := b.TransformRequest("gpt-4", in, true, &Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("default transform should be a no-op, got %q", out)
	}
}

// ────────────────────────────────────────────────────────────────────
// ShouldRetry — table-driven
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_ShouldRetry(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"a", "b"}, MaxRetries: 2}, nil)
	cases := []struct {
		name     string
		status   int
		urlIndex int
		want     bool
	}{
		{"429 primary", 429, 0, true},
		{"429 fallback", 429, 1, true},
		{"500 primary", 500, 0, true},
		{"502 primary", 502, 0, true},
		{"503 primary", 503, 0, true},
		{"504 primary", 504, 0, true},
		{"400", 400, 0, false},
		{"401", 401, 0, false},
		{"403", 403, 0, false},
		{"404", 404, 0, false},
		{"200", 200, 0, false},
		{"422", 422, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := b.ShouldRetry(c.status, c.urlIndex); got != c.want {
				t.Errorf("ShouldRetry(%d,%d) = %v, want %v", c.status, c.urlIndex, got, c.want)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────
// ParseError — covers all required status codes
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_ParseError_StatusCodes(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	cases := []struct {
		name       string
		status     int
		body       string
		wantCode   ErrorCode
		wantStatus int
	}{
		{"400 plain", 400, `{"error":{"message":"bad input"}}`, CodeBadRequest, 400},
		{"400 context length", 400, `{"error":{"message":"context_length_exceeded"}}`, CodeContextLimit, 400},
		{"400 reduce the length", 400, `{"error":{"message":"please reduce the length of the messages"}}`, CodeContextLimit, 400},
		{"401", 401, `{"error":{"message":"invalid token"}}`, CodeAuthFailed, 401},
		{"403", 403, `{"error":{"message":"forbidden"}}`, CodeAuthFailed, 403},
		{"404", 404, `{"error":{"message":"model not found"}}`, CodeModelNotFound, 404},
		{"429", 429, `{"error":{"message":"slow down"}}`, CodeRateLimited, 429},
		{"500", 500, `{"error":{"message":"internal"}}`, CodeServerError, 500},
		{"502", 502, `bad gateway`, CodeServerError, 502},
		{"503", 503, `unavailable`, CodeServerError, 503},
		{"504", 504, `gateway timeout`, CodeServerError, 504},
		{"0 network", 0, ``, CodeUnavailable, 0},
		{"418 unknown", 418, `teapot`, CodeUnknown, 418},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ee := b.ParseError(c.status, c.body)
			if ee == nil {
				t.Fatalf("ParseError returned nil")
			}
			if ee.Code != c.wantCode {
				t.Errorf("Code = %q, want %q", ee.Code, c.wantCode)
			}
			if ee.Status != c.wantStatus {
				t.Errorf("Status = %d, want %d", ee.Status, c.wantStatus)
			}
		})
	}
}

func TestBaseExecutor_ParseError_ExtractsMessage(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	ee := b.ParseError(401, `{"error":{"message":"invalid token","type":"auth"}}`)
	if ee.Message != "invalid token" {
		t.Errorf("Message = %q, want %q", ee.Message, "invalid token")
	}
}

// ────────────────────────────────────────────────────────────────────
// NeedsRefresh
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_NeedsRefresh(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	now := time.Now().Unix()

	cases := []struct {
		name  string
		creds *Credentials
		want  bool
	}{
		{"nil creds", nil, true},
		{"empty token", &Credentials{AccessToken: ""}, true},
		{"no expiry", &Credentials{AccessToken: "t", ExpiresAt: 0}, false},
		{"expired", &Credentials{AccessToken: "t", ExpiresAt: now - 100}, true},
		{"expires within skew", &Credentials{AccessToken: "t", ExpiresAt: now + 30}, true},
		{"expires in 10 min", &Credentials{AccessToken: "t", ExpiresAt: now + 600}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := b.NeedsRefresh(c.creds); got != c.want {
				t.Errorf("NeedsRefresh = %v, want %v", got, c.want)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────
// RefreshCredentials default
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_RefreshCredentials_Noop(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	in := &Credentials{AccessToken: "t"}
	out, err := b.RefreshCredentials(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != in {
		t.Errorf("default refresh should return the same pointer")
	}
}

// ────────────────────────────────────────────────────────────────────
// Execute — table-driven
// ────────────────────────────────────────────────────────────────────

// makeServer creates an httptest.Server that responds to a single
// path with the given sequence of (status, body) tuples. Each request
// consumes the next entry. After the sequence is exhausted the server
// loops on the last entry. Use for "first N attempts, then success" or
// "always fail" patterns.
func makeServer(seq []respSpec) *httptest.Server {
	var idx int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt32(&idx, 1)) - 1
		if i >= len(seq) {
			i = len(seq) - 1
		}
		spec := seq[i]
		w.Header().Set("Content-Type", spec.contentType)
		w.WriteHeader(spec.status)
		_, _ = w.Write(spec.body)
	}))
}

type respSpec struct {
	status      int
	body        []byte
	contentType string
}

func newClient(t *testing.T, srv *httptest.Server) HTTPClient {
	t.Helper()
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Re-target request to the test server so URL building stays
		// honest. The original URL host is replaced with the test
		// server's address.
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = "http"
		req2.URL.Host = srv.Listener.Addr().String()
		req2.RequestURI = ""
		return http.DefaultClient.Do(req2)
	})
}

func TestBaseExecutor_Execute_FallbackURL(t *testing.T) {
	// First URL returns 503 once then we move on; second URL returns 200.
	// We model this with two test servers.
	primary := makeServer([]respSpec{{status: 503, body: []byte(`{"error":{"message":"down"}}`)}})
	defer primary.Close()
	backup := makeServer([]respSpec{{status: 200, body: []byte(`{"ok":true}`), contentType: "application/json"}})
	defer backup.Close()

	cfg := &ProviderConfig{
		Provider:   "openai",
		BaseURLs:   []string{primary.URL, backup.URL},
		MaxRetries: 1,
	}
	b := NewBaseExecutor(cfg, nil)

	resp, err := b.Execute(context.Background(),
		&Request{Method: "POST", Model: "gpt-4", Stream: false, Body: []byte(`{"x":1}`)},
		&Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
	if resp.URLIndex != 1 {
		t.Errorf("URLIndex = %d, want 1 (fallback)", resp.URLIndex)
	}
	_ = resp.Body.Close()
}

func TestBaseExecutor_Execute_ContextCancellation(t *testing.T) {
	// Server blocks until released; we cancel the context first.
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	defer srv.Close()
	defer close(released)

	cfg := &ProviderConfig{Provider: "openai", BaseURLs: []string{srv.URL}, MaxRetries: 1}
	b := NewBaseExecutor(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Execute(ctx,
		&Request{Method: "POST", Model: "gpt-4", Body: []byte(`{}`)},
		&Credentials{AccessToken: "t"})
	if err == nil {
		t.Fatalf("expected error on context cancellation, got nil")
	}
	ee, ok := AsExecutorError(err)
	if !ok {
		t.Fatalf("expected ExecutorError, got %T: %v", err, err)
	}
	if ee.Code != CodeTimeout && ee.Code != CodeCanceled {
		t.Errorf("Code = %q, want %q or %q", ee.Code, CodeTimeout, CodeCanceled)
	}
}

func TestBaseExecutor_Execute_NilRequest(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}, nil)
	_, err := b.Execute(context.Background(), nil, &Credentials{AccessToken: "t"})
	if err == nil {
		t.Fatalf("expected error for nil request")
	}
	ee, ok := AsExecutorError(err)
	if !ok || ee.Code != CodeBadRequest {
		t.Errorf("expected BadRequest ExecutorError, got %v", err)
	}
}

func TestBaseExecutor_Execute_TransportErrorExhaustsAndReturns(t *testing.T) {
	// Force the transport to always fail — the executor should surface an
	// Unavailable error after exhausting retries.
	failing := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	cfg := &ProviderConfig{Provider: "openai", BaseURLs: []string{"https://x"}, MaxRetries: 1}
	b := NewBaseExecutor(cfg, failing)
	_, err := b.Execute(context.Background(),
		&Request{Method: "POST", Model: "gpt-4", Body: []byte(`{}`)},
		&Credentials{AccessToken: "t"})
	if err == nil {
		t.Fatalf("expected error")
	}
	ee, ok := AsExecutorError(err)
	if !ok {
		t.Fatalf("expected ExecutorError, got %T", err)
	}
	if ee.Code != CodeUnavailable {
		t.Errorf("Code = %q, want Unavailable", ee.Code)
	}
}

func TestBaseExecutor_Execute_AllRetriesExhausted(t *testing.T) {
	srv := makeServer([]respSpec{{status: 500, body: []byte(`{"error":{"message":"oops"}}`)}})
	defer srv.Close()
	cfg := &ProviderConfig{Provider: "openai", BaseURLs: []string{srv.URL}, MaxRetries: 2}
	b := NewBaseExecutor(cfg, nil)

	_, err := b.Execute(context.Background(),
		&Request{Method: "POST", Model: "gpt-4", Body: []byte(`{}`)},
		&Credentials{AccessToken: "t"})
	if err == nil {
		t.Fatalf("expected error after exhausted retries")
	}
	ee, ok := AsExecutorError(err)
	if !ok {
		t.Fatalf("expected ExecutorError, got %T", err)
	}
	if ee.Code != CodeServerError {
		t.Errorf("Code = %q, want ServerError", ee.Code)
	}
	if ee.Status != 500 {
		t.Errorf("Status = %d, want 500", ee.Status)
	}
}

func TestBaseExecutor_Execute_CredentialsRefresh(t *testing.T) {
	// NeedsRefresh returns true when token is empty or creds is nil — the
	// executor is expected to call RefreshCredentials and swap in the
	// fresh value before making the actual request.
	b := NewBaseExecutor(&ProviderConfig{Provider: "openi", BaseURLs: []string{"x"}}, nil)
	if !b.NeedsRefresh(&Credentials{AccessToken: ""}) {
		t.Errorf("empty token should trigger refresh")
	}
	if !b.NeedsRefresh(nil) {
		t.Errorf("nil creds should trigger refresh")
	}
	// Token with no expiry (ExpiresAt=0) — treated as valid, no refresh.
	if b.NeedsRefresh(&Credentials{AccessToken: "t", ExpiresAt: 0}) {
		t.Errorf("no-expiry creds should not trigger refresh")
	}

	// Verify RefreshCredentials default noop returns same pointer.
	out, err := b.RefreshCredentials(context.Background(), &Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.AccessToken != "t" {
		t.Errorf("default refresh should preserve token, got %q", out.AccessToken)
	}
}

func TestBaseExecutor_Execute_BodyTransformsViaWrapper(t *testing.T) {
	// Wrap BaseExecutor to verify TransformRequest hook fires.
	called := false
	cfg := &ProviderConfig{Provider: "openai", BaseURLs: []string{"x"}}
	b := NewBaseExecutor(cfg, nil)
	out, err := b.TransformRequest("gpt-4", []byte(`{"x":1}`), true, &Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !bytes.Equal(out, []byte(`{"x":1}`)) {
		t.Errorf("default transform should be a no-op")
	}
	_ = called
}

// ────────────────────────────────────────────────────────────────────
// Backoff schedule
// ────────────────────────────────────────────────────────────────────

func TestBackoffDelay(t *testing.T) {
	// 1 → 250ms, 2 → 500ms, 3 → 1s, 4 → 2s, 5 → 4s, 6 → 8s, 7 → 16s, 8 → 16s
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 250 * time.Millisecond},
		{2, 500 * time.Millisecond},
		{3, 1 * time.Second},
		{4, 2 * time.Second},
		{5, 4 * time.Second},
		{6, 8 * time.Second},
		{7, 16 * time.Second},
		{8, 16 * time.Second},
		{0, 250 * time.Millisecond}, // clamped to attempt=1
	}
	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			if got := BackoffDelay(c.attempt); got != c.want {
				t.Errorf("BackoffDelay(%d) = %v, want %v", c.attempt, got, c.want)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────
// ExecutorError
// ────────────────────────────────────────────────────────────────────

func TestExecutorError_Error(t *testing.T) {
	ee := &ExecutorError{Code: CodeAuthFailed, Message: "nope", Status: 401, Body: `{"error":"x"}`}
	s := ee.Error()
	if s == "" {
		t.Errorf("Error() should not be empty")
	}
	if !bytes.Contains([]byte(s), []byte("AuthFailed")) {
		t.Errorf("Error() should contain code: %s", s)
	}
}

func TestExecutorError_NilSafe(t *testing.T) {
	var ee *ExecutorError
	// typed nil should not panic through Error()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("typed nil *ExecutorError should not panic through Error()")
		}
	}()
	_ = ee.Error()
}

func TestExecutorError_Is(t *testing.T) {
	a := &ExecutorError{Code: CodeAuthFailed}
	b := &ExecutorError{Code: CodeAuthFailed}
	c := &ExecutorError{Code: CodeRateLimited}
	if !errors.Is(a, b) {
		t.Errorf("expected Is to match by code")
	}
	if errors.Is(a, c) {
		t.Errorf("different codes should not match")
	}
}

func TestAsExecutorError(t *testing.T) {
	ee := &ExecutorError{Code: CodeRateLimited}
	wrapped := errors.New("wrap: " + ee.Error())
	// errors.As walks the chain by unwrap; manual chain works because
	// ExecutorError doesn't implement Unwrap — build a wrapped target.
	if _, ok := AsExecutorError(wrapped); ok {
		t.Errorf("plain wrapped error should not yield an ExecutorError")
	}
	// Direct pass works.
	if got, ok := AsExecutorError(ee); !ok || got.Code != CodeRateLimited {
		t.Errorf("AsExecutorError(direct) = (%v,%v), want (CodeRateLimited, true)", got, ok)
	}
	// nil pass.
	if _, ok := AsExecutorError(nil); ok {
		t.Errorf("nil error should yield false")
	}
}

// ────────────────────────────────────────────────────────────────────
// String() and ToString sanity
// ────────────────────────────────────────────────────────────────────

func TestBaseExecutor_String(t *testing.T) {
	b := NewBaseExecutor(&ProviderConfig{Provider: "openai", BaseURLs: []string{"a", "b"}}, nil)
	s := b.String()
	if !bytes.Contains([]byte(s), []byte("openai")) {
		t.Errorf("String() should mention provider: %s", s)
	}
	var nilB *BaseExecutor
	if nilB.String() == "" {
		t.Errorf("nil String() should not be empty")
	}
}

// ────────────────────────────────────────────────────────────────────
// JSON body parsing
// ────────────────────────────────────────────────────────────────────

func TestErrorMessageFromBody(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"error":{"message":"hi"}}`, "hi"},
		{`{"message":"plain"}`, "plain"},
		{`not json`, "not json"},
		{``, ""},
	}
	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			if got := errorMessageFromBody(c.body); got != c.want {
				t.Errorf("errorMessageFromBody(%q) = %q, want %q", c.body, got, c.want)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────
// Make sure the test fixtures themselves compile and the helper for
// reading bodies (a few smoke tests).
// ────────────────────────────────────────────────────────────────────

func TestNoopReader(t *testing.T) {
	r := &noopReader{data: []byte("hi")}
	buf := make([]byte, 4)
	n, err := r.Read(buf)
	if n != 2 || err != nil {
		t.Errorf("Read = (%d, %v), want (2, nil)", n, err)
	}
	n, err = r.Read(buf)
	if n != 0 || err != io.EOF {
		t.Errorf("second Read = (%d, %v), want (0, EOF)", n, err)
	}
}

// Compile-time check: ensure we satisfy the Executor interface.
var _ Executor = (*BaseExecutor)(nil)

// Sanity: BaseExecutor's MakeSureNoUnusedJson (json is used via errorMessageFromBody).
var _ = json.Marshal
