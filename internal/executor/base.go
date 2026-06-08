// Package executor defines the Executor interface and the BaseExecutor
// implementation that all provider executors build on.
//
// The package is the Go equivalent of open-sse/executors/BaseExecutor.js —
// it owns URL building, header construction, request transformation, retry
// orchestration across fallback URLs, credential-refresh hooks, and
// error mapping. Concrete executors (OpenAI, Anthropic, Gemini, ...) plug
// in by embedding *BaseExecutor and overriding only the methods whose
// default behavior does not fit the upstream provider.
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProviderConfig is the static, per-provider configuration an executor needs
// to build requests. A single ProviderConfig describes the upstream — it
// does NOT hold credentials, those live in the per-call Credentials value.
//
//	BaseURLs   — list of upstream endpoints; the first non-errored response
//	             wins. Empty BaseURLs is treated as [""] so callers can pass
//	             a single URL with no fallback semantics.
//	AuthHeader — which header name to inject the bearer token into
//	             (e.g. "Authorization", "x-api-key").
//	StreamPath — URL path used when stream=true (overrides the default path).
//	NonStreamPath — URL path used when stream=false.
//	MaxRetries — number of times the executor is allowed to retry on a
//	             retryable status before surfacing the last error to the
//	             caller. Default 2.
type ProviderConfig struct {
	Provider      string
	BaseURLs      []string
	AuthHeader    string
	StreamPath    string
	NonStreamPath string
	MaxRetries    int
}

// FallbackCount returns the number of fallback URLs the executor may
// iterate through when the primary URL fails.
func (c *ProviderConfig) FallbackCount() int {
	if c == nil || len(c.BaseURLs) == 0 {
		return 1
	}
	return len(c.BaseURLs)
}

// baseURL returns the URL at the given index, or "" if the index is out of
// range. An empty BaseURLs list is treated as a single-element list of "".
func (c *ProviderConfig) baseURL(idx int) string {
	if c == nil || len(c.BaseURLs) == 0 {
		return ""
	}
	if idx < 0 || idx >= len(c.BaseURLs) {
		return ""
	}
	return c.BaseURLs[idx]
}

// Credentials is the per-call auth payload. AccessToken is always present
// (the executor returns AuthFailed if it is empty). ExpiresAt is the unix
// time (seconds) at which the token is no longer valid; NeedsRefresh uses
// it to decide whether RefreshCredentials should be invoked before the
// request is sent.
type Credentials struct {
	AccessToken string
	ExpiresAt   int64
	Extra       map[string]string
}

// ErrorCode is a stable, machine-readable identifier for executor errors.
// Callers (the chat handler) pattern-match on Code to decide which
// credential rotation / combo fallback / circuit-breaker behavior to run.
type ErrorCode string

const (
	CodeAuthFailed    ErrorCode = "AuthFailed"
	CodeRateLimited   ErrorCode = "RateLimited"
	CodeBadRequest    ErrorCode = "BadRequest"
	CodeContextLimit  ErrorCode = "ContextLimit"
	CodeModelNotFound ErrorCode = "ModelNotFound"
	CodeContentFilter ErrorCode = "ContentFilter"
	CodeBilling       ErrorCode = "BillingError"
	CodeServerError   ErrorCode = "ServerError"
	CodeUnavailable   ErrorCode = "Unavailable"
	CodeTimeout       ErrorCode = "Timeout"
	CodeCanceled      ErrorCode = "Canceled"
	CodeUnknown       ErrorCode = "Unknown"
)

// ExecutorError carries the wire-level error back to the caller alongside
// a typed Code so handler code can switch on Code without parsing
// human-readable messages.
type ExecutorError struct {
	Code    ErrorCode
	Message string
	Status  int
	Body    string
}

func (e *ExecutorError) Error() string {
	if e == nil {
		return "<nil ExecutorError>"
	}
	if e.Status > 0 {
		return fmt.Sprintf("executor %s: %s (status %d): %s", e.Code, e.Message, e.Status, e.Body)
	}
	return fmt.Sprintf("executor %s: %s: %s", e.Code, e.Message, e.Body)
}

// Is allows errors.Is to walk through the chain.
func (e *ExecutorError) Is(target error) bool {
	var t *ExecutorError
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// AsExecutorError unwraps an error chain to the first *ExecutorError. It
// returns (nil, false) when the chain does not contain one.
func AsExecutorError(err error) (*ExecutorError, bool) {
	if err == nil {
		return nil, false
	}
	var ee *ExecutorError
	if errors.As(err, &ee) {
		return ee, true
	}
	return nil, false
}

// Request is what the chat handler hands to Execute. The body is already
// in the executor's expected wire format (OpenAI / Anthropic / ...) because
// the translators run upstream of the executor.
type Request struct {
	Method  string
	Model   string
	Stream  bool
	Body    []byte
	Headers map[string]string
}

// Response is the executor's return value. Body holds the full upstream
// payload; for streaming requests the handler reads Body directly because
// the executor preserves the upstream's chunked transfer encoding by
// returning the response body un-buffered.
type Response struct {
	Status      int
	Headers     http.Header
	Body        io.ReadCloser
	URLIndex    int
	Attempts    int
	ContentType string
}

// Executor is the contract every concrete provider executor must satisfy.
// The contract is deliberately minimal — most of the work lives in
// BaseExecutor, which is meant to be embedded by concrete executors.
type Executor interface {
	// Execute runs the request to completion, iterating over fallback
	// URLs and applying retry/backoff where the upstream signals a
	// transient failure. Context cancellation aborts the in-flight HTTP
	// call and returns a CodeCanceled error.
	Execute(ctx context.Context, req *Request, creds *Credentials) (*Response, error)

	// GetProvider returns the provider identifier (matches ProviderConfig.
	// Provider). Used for logging / observability / combo routing.
	GetProvider() string

	// RefreshCredentials exchanges a soon-to-expire token for a fresh one.
	// The default BaseExecutor implementation is a no-op that returns the
	// supplied creds unchanged. OAuth-driven executors override this.
	RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error)

	// NeedsRefresh reports whether the supplied credentials are within
	// the freshness window. The default implementation triggers a refresh
	// when the token is unparseable or expires within RefreshSkew
	// (default 60 seconds).
	NeedsRefresh(creds *Credentials) bool
}

// RefreshSkew is the time window before expiry during which NeedsRefresh
// returns true. Sixty seconds is enough headroom for a refresh round-trip
// on most providers.
const RefreshSkew = 60 * time.Second

// DefaultMaxRetries is applied when ProviderConfig.MaxRetries is zero.
const DefaultMaxRetries = 2

// HTTPClient is the interface BaseExecutor uses for the upstream call.
// Production wiring passes *http.Client (with whatever timeout / transport
// the host wants); tests pass an httptest.Server-backed client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// BaseExecutor is the default implementation of Executor. Concrete
// provider executors embed it and override only the methods whose upstream
// behavior diverges from the defaults.
//
// Embedding is value-typed (*BaseExecutor) so executors get value-method
// dispatch on URL / header / retry helpers, but they can still override
// any method by defining a value-receiver on the outer struct (Go's
// method promotion handles the rest).
type BaseExecutor struct {
	provider string
	config   *ProviderConfig
	client   HTTPClient
}

// NewBaseExecutor returns a BaseExecutor wired with the supplied
// ProviderConfig and HTTPClient. A nil client falls back to http.DefaultClient.
func NewBaseExecutor(cfg *ProviderConfig, client HTTPClient) *BaseExecutor {
	if client == nil {
		client = http.DefaultClient
	}
	if cfg != nil && cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultMaxRetries
	}
	if cfg != nil && len(cfg.BaseURLs) == 0 {
		cfg.BaseURLs = []string{""}
	}
	if cfg != nil && cfg.AuthHeader == "" {
		cfg.AuthHeader = "Authorization"
	}
	return &BaseExecutor{
		provider: providerName(cfg),
		config:   cfg,
		client:   client,
	}
}

func providerName(cfg *ProviderConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.Provider
}

// GetProvider returns the provider identifier.
func (b *BaseExecutor) GetProvider() string { return b.provider }

// Config returns the underlying ProviderConfig. Read-only by convention —
// callers must not mutate it.
func (b *BaseExecutor) Config() *ProviderConfig { return b.config }

// HTTPClient returns the underlying HTTP client. Used by tests to swap in
// a custom transport.
func (b *BaseExecutor) HTTPClient() HTTPClient { return b.client }

// BaseUrls returns the fallback URL list (the executor may iterate through
// these on 429 / 5xx).
func (b *BaseExecutor) BaseUrls() []string {
	if b.config == nil {
		return []string{""}
	}
	out := make([]string, len(b.config.BaseURLs))
	copy(out, b.config.BaseURLs)
	return out
}

// BuildUrl constructs the request URL for the given model, stream flag,
// and fallback URL index. The default base URL is taken from
// ProviderConfig.BaseURLs[urlIndex]; the path is StreamPath / NonStreamPath
// (or the default "/v1/chat/completions" when both are empty).
func (b *BaseExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	base := b.config.baseURL(urlIndex)
	path := b.config.NonStreamPath
	if stream {
		path = b.config.StreamPath
	}
	if path == "" {
		path = "/v1/chat/completions"
	}
	return joinURL(base, path)
}

// joinURL concatenates a base URL and a path with exactly one slash between
// them. Empty base is treated as the document root.
func joinURL(base, path string) string {
	if base == "" {
		return path
	}
	if path == "" {
		return base
	}
	baseSlash := strings.HasSuffix(base, "/")
	pathSlash := strings.HasPrefix(path, "/")
	switch {
	case baseSlash && pathSlash:
		return base + path[1:]
	case !baseSlash && !pathSlash:
		return base + "/" + path
	default:
		return base + path
	}
}

// BuildHeaders constructs the per-request header set. It always includes
// Content-Type: application/json and the auth header sourced from
// creds.AccessToken via ProviderConfig.AuthHeader. Per-call overrides in
// req.Headers win on conflict.
func (b *BaseExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")

	authHeader := "Authorization"
	if b.config != nil && b.config.AuthHeader != "" {
		authHeader = b.config.AuthHeader
	}
	if creds != nil && creds.AccessToken != "" {
		switch strings.ToLower(authHeader) {
		case "authorization":
			h.Set(authHeader, "Bearer "+creds.AccessToken)
		default:
			h.Set(authHeader, creds.AccessToken)
		}
	}
	for k, v := range req.Headers {
		h.Set(k, v)
	}
	return h
}

// TransformRequest mutates the request body for the given model / stream
// flag and credentials. The default implementation is a no-op: concrete
// executors override this when the upstream expects provider-specific
// fields (e.g. anthropic_executor injects max_tokens).
func (b *BaseExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return body, nil
}

// ShouldRetry reports whether the executor should advance to the next
// fallback URL or retry the same one after a backoff. The default policy:
//
//   - 429 always retries while urlIndex < len(BaseURLs)-1 OR while
//     attempts < MaxRetries (so a single URL with retries still works).
//   - 500/502/503/504 retry on the same URL until MaxRetries is exhausted.
//   - 4xx other than 429 do not retry.
//
// The bool is the same value used by Execute's main loop.
func (b *BaseExecutor) ShouldRetry(status int, urlIndex int) bool {
	switch status {
	case http.StatusTooManyRequests:
		// 429 — try a fallback URL if any remain, else back off and retry.
		return true
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// MaxRetriesFor returns the configured retry budget, falling back to the
// package default when the config is missing.
func (b *BaseExecutor) MaxRetriesFor() int {
	if b.config != nil && b.config.MaxRetries > 0 {
		return b.config.MaxRetries
	}
	return DefaultMaxRetries
}

// ParseError maps an upstream status + body into a typed ExecutorError.
// The mapping is intentionally provider-agnostic: the chat handler can
// branch on Code without knowing which executor raised the error.
//
//	400 → BadRequest   (400 + "context_length_exceeded" → ContextLimit)
//	401 → AuthFailed
//	403 → AuthFailed   (insufficient scope is still auth failure for our
//	                   routing purposes)
//	404 → ModelNotFound
//	429 → RateLimited
	// 500/502/503/504 → ServerError
	// 0 (network error) → Unavailable
func (b *BaseExecutor) ParseError(status int, body string) *ExecutorError {
	msg := errorMessageFromBody(body)
	switch status {
	case 0:
		return &ExecutorError{Code: CodeUnavailable, Message: "upstream unreachable", Status: status, Body: body}
	case http.StatusBadRequest:
		if looksLikeContextOverflow(body) {
			return &ExecutorError{Code: CodeContextLimit, Message: msg, Status: status, Body: body}
		}
		return &ExecutorError{Code: CodeBadRequest, Message: msg, Status: status, Body: body}
	case http.StatusUnauthorized:
		return &ExecutorError{Code: CodeAuthFailed, Message: msg, Status: status, Body: body}
	case http.StatusForbidden:
		return &ExecutorError{Code: CodeAuthFailed, Message: msg, Status: status, Body: body}
	case http.StatusNotFound:
		return &ExecutorError{Code: CodeModelNotFound, Message: msg, Status: status, Body: body}
	case http.StatusTooManyRequests:
		return &ExecutorError{Code: CodeRateLimited, Message: msg, Status: status, Body: body}
	case http.StatusRequestTimeout:
		return &ExecutorError{Code: CodeTimeout, Message: msg, Status: status, Body: body}
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return &ExecutorError{Code: CodeServerError, Message: msg, Status: status, Body: body}
	default:
		return &ExecutorError{Code: CodeUnknown, Message: msg, Status: status, Body: body}
	}
}

// errorMessageFromBody extracts a human-readable message from a JSON error
// body. Falls back to the raw body when JSON parsing fails or the body is
// empty — the chat handler may still want to surface it.
func errorMessageFromBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var obj struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &obj); err == nil {
		if obj.Error.Message != "" {
			return obj.Error.Message
		}
		if obj.Message != "" {
			return obj.Message
		}
	}
	return body
}

// looksLikeContextOverflow is a heuristic that matches the upstream error
// markers several providers use when the prompt exceeds the model's
// window. We intentionally over-match on "context length" / "context
// window" / "too long" so we err on the side of returning ContextLimit
// (the user can retry with a smaller prompt) rather than BadRequest.
func looksLikeContextOverflow(body string) bool {
	b := strings.ToLower(body)
	if strings.Contains(b, "context_length") {
		return true
	}
	if strings.Contains(b, "context window") {
		return true
	}
	if strings.Contains(b, "maximum context") {
		return true
	}
	if strings.Contains(b, "prompt is too long") {
		return true
	}
	if strings.Contains(b, "reduce the length") {
		return true
	}
	if strings.Contains(b, "string too long") {
		return true
	}
	return false
}

// NeedsRefresh reports whether the supplied credentials are within the
// freshness window. A nil creds returns true (caller must obtain a
// credential). A zero ExpiresAt means "no expiry" and returns false.
func (b *BaseExecutor) NeedsRefresh(creds *Credentials) bool {
	if creds == nil || creds.AccessToken == "" {
		return true
	}
	if creds.ExpiresAt == 0 {
		return false
	}
	expiry := time.Unix(creds.ExpiresAt, 0)
	return time.Until(expiry) <= RefreshSkew
}

// RefreshCredentials is a no-op in BaseExecutor. OAuth-backed executors
// override this to actually exchange the refresh token for a new access
// token.
func (b *BaseExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	return creds, nil
}

// Execute implements the Executor contract on BaseExecutor. It is
// concurrency-safe: each call constructs a fresh sequence of HTTP
// requests and never mutates the BaseExecutor's mutable state.
//
// The main loop walks fallback URLs in order, applying backoff between
// retries. The exact algorithm:
//
//	for urlIndex in 0..len(BaseURLs)-1:
//	    for attempt in 0..MaxRetries:
//	        if ctx.Done: return CodeCanceled
//	        if NeedsRefresh(creds): refresh
//	        transformed, _ := TransformRequest(...)
//	        req := build(BuildUrl, BuildHeaders)
//	        resp, err := client.Do(req)
//	        if net-err: backoff and continue (transient)
//	        if !ShouldRetry(resp.Status, urlIndex): return resp
//	        if urlIndex < len(BaseURLs)-1: break inner loop (try next URL)
//	        backoff and continue inner loop
//
// The first non-retryable response (success or permanent failure) is
// returned to the caller as-is — for failures the error is wrapped in
// *ExecutorError so the chat handler can switch on Code.
func (b *BaseExecutor) Execute(ctx context.Context, req *Request, creds *Credentials) (*Response, error) {
	if req == nil {
		return nil, &ExecutorError{Code: CodeBadRequest, Message: "nil request"}
	}
	if b.NeedsRefresh(creds) {
		var err error
		creds, err = b.RefreshCredentials(ctx, creds)
		if err != nil {
			return nil, &ExecutorError{Code: CodeAuthFailed, Message: "credential refresh failed: " + err.Error()}
		}
	}

	body, err := b.TransformRequest(req.Model, req.Body, req.Stream, creds)
	if err != nil {
		return nil, &ExecutorError{Code: CodeBadRequest, Message: "transform request: " + err.Error()}
	}

	urls := b.config.FallbackCount()
	maxRetries := b.MaxRetriesFor()
	totalAttempts := 0

	for urlIndex := 0; urlIndex < urls; urlIndex++ {
		for attempt := 0; attempt <= maxRetries; attempt++ {
			totalAttempts++

			if err := ctx.Err(); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					return nil, &ExecutorError{Code: CodeTimeout, Message: "context deadline exceeded"}
				}
				return nil, &ExecutorError{Code: CodeCanceled, Message: "context canceled"}
			}

			// Backoff before any attempt beyond the first on a given URL.
			if attempt > 0 {
				delay := backoffDelay(attempt)
				select {
				case <-ctx.Done():
					return nil, &ExecutorError{Code: CodeCanceled, Message: "context canceled during backoff"}
				case <-time.After(delay):
				}
			}

			httpReq, err := b.buildHTTPRequest(ctx, req, body, creds, urlIndex)
			if err != nil {
				return nil, &ExecutorError{Code: CodeBadRequest, Message: err.Error()}
			}

			resp, err := b.client.Do(httpReq)
			if err != nil {
				// Transport-level error: try next URL / retry. If we
				// exhaust both axes, surface as Unavailable.
				if urlIndex == urls-1 && attempt == maxRetries {
					return nil, &ExecutorError{Code: CodeUnavailable, Message: err.Error()}
				}
				continue
			}

			if !b.ShouldRetry(resp.StatusCode, urlIndex) {
				return b.toExecutorResponse(resp, urlIndex, totalAttempts), nil
			}

			// Retryable: drain + close the body so the connection can
			// be reused, then decide whether to advance URL or back off.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			// If another fallback URL is available, abandon retries on
			// the current one and try the fallback. Otherwise we've
			// burned the per-URL retry budget.
			if urlIndex < urls-1 {
				break
			}
			if attempt == maxRetries {
				return nil, &ExecutorError{
					Code:    CodeServerError,
					Message: fmt.Sprintf("upstream %d after %d attempts", resp.StatusCode, totalAttempts),
					Status:  resp.StatusCode,
				}
			}
		}
	}

	// All URLs and retries exhausted without a terminal response — this
	// should be unreachable given the loop above, but if the executor
	// was configured with zero URLs we land here.
	return nil, &ExecutorError{Code: CodeUnavailable, Message: "no fallback URLs configured"}
}

// buildHTTPRequest assembles the *http.Request that the client will send.
// The body is io.NopCloser(bytes.NewReader) so it can be re-read by Go's
// http stack on connection-reuse redirects if those are enabled in the
// transport.
func (b *BaseExecutor) buildHTTPRequest(ctx context.Context, req *Request, body []byte, creds *Credentials, urlIndex int) (*http.Request, error) {
	method := req.Method
	if method == "" {
		method = http.MethodPost
	}
	target := b.BuildUrl(req.Model, req.Stream, urlIndex)
	headers := b.BuildHeaders(req, creds)

	httpReq, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	return httpReq, nil
}

// toExecutorResponse converts an *http.Response into the executor's
// Response value. The body is returned as a ReadCloser — streaming
// executors (and their callers) read it incrementally without buffering.
func (b *BaseExecutor) toExecutorResponse(resp *http.Response, urlIndex, attempts int) *Response {
	contentType := resp.Header.Get("Content-Type")
	return &Response{
		Status:      resp.StatusCode,
		Headers:     resp.Header.Clone(),
		Body:        resp.Body,
		URLIndex:    urlIndex,
		Attempts:    attempts,
		ContentType: contentType,
	}
}

// backoffDelay returns the wait time before retry number `attempt` (1-based
// before this call — callers pass attempt = retry number, 1 = first retry).
// The schedule is 250ms, 500ms, 1s, 2s, 4s ... capped at 16s.
func backoffDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 250 * time.Millisecond * time.Duration(1<<uint(attempt-1))
	if d > 16*time.Second {
		d = 16 * time.Second
	}
	return d
}

// BackoffDelay is exported for tests.
func BackoffDelay(attempt int) time.Duration { return backoffDelay(attempt) }

// ParseUpstreamError is a convenience wrapper: it returns the
// *ExecutorError that Execute would have produced for the given
// (status, body) pair. Useful for callers that want to reuse the
// BaseExecutor mapping without going through the HTTP stack (e.g. tests,
// CLI tools that talk to providers directly).
func (b *BaseExecutor) ParseUpstreamError(status int, body string) error {
	ee := b.ParseError(status, body)
	if ee == nil {
		return nil
	}
	return ee
}

// String returns a debug-friendly representation.
func (b *BaseExecutor) String() string {
	if b == nil {
		return "<nil BaseExecutor>"
	}
	return fmt.Sprintf("BaseExecutor{provider=%q, urls=%d, maxRetries=%d}",
		b.provider, b.config.FallbackCount(), b.MaxRetriesFor())
}

// Sanity check: a typed-nil must not panic through Error().
var _ error = (*ExecutorError)(nil)
