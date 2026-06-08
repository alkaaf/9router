package executor

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// OllamaDefaultHost is the default Ollama daemon address when the user
// does not supply one in credentials.
const OllamaDefaultHost = "localhost:11434"

// OllamaLocalExecutor talks to a local Ollama instance. The wire
// format differs from OpenAI:
//
//   - Endpoint: POST /api/chat (not /v1/chat/completions).
//   - Body: OpenAI shape but Ollama returns NDJSON, not SSE.
//   - Auth: none required for local mode.
//   - The host is resolved from credentials.Extra["host"] or the
//     OllamaDefaultHost default.
type OllamaLocalExecutor struct {
	*BaseExecutor
}

// NewOllamaExecutor returns an OllamaLocalExecutor wired to the
// default host.
func NewOllamaExecutor() *OllamaLocalExecutor {
	return NewOllamaExecutorWithHost(OllamaDefaultHost)
}

// NewOllamaExecutorWithHost returns an OllamaLocalExecutor pointing
// at the supplied address (host:port).
func NewOllamaExecutorWithHost(host string) *OllamaLocalExecutor {
	url := "http://" + host + "/api/chat"
	cfg := &ProviderConfig{
		Provider:   "ollama",
		BaseURLs:   []string{url},
		AuthHeader: "api-key",
		MaxRetries: DefaultMaxRetries,
	}
	return &OllamaLocalExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// BuildUrl returns the Ollama /api/chat URL. When stream=true it
// appends ?stream=true to the URL — Ollama uses the query parameter
// rather than a body flag for streaming.
func (o *OllamaLocalExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	if o.config == nil || len(o.config.BaseURLs) == 0 {
		base := "http://" + OllamaDefaultHost + "/api/chat"
		if stream {
			return base + "?stream=true"
		}
		return base
	}
	base := o.config.BaseURLs[urlIndex]
	if stream {
		if strings.Contains(base, "?") {
			return base + "&stream=true"
		}
		return base + "?stream=true"
	}
	return base
}

// BuildHeaders sets no auth headers for the local executor.
func (o *OllamaLocalExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	if req.Headers != nil {
		for k, v := range req.Headers {
			h.Set(k, v)
		}
	}
	return h
}

// TransformRequest is a passthrough — the Ollama translator is
// responsible for producing the right body shape.
func (o *OllamaLocalExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return body, nil
}

// NeedsRefresh returns false — local Ollama never needs auth.
func (o *OllamaLocalExecutor) NeedsRefresh(creds *Credentials) bool {
	return false
}

// RefreshCredentials is a no-op.
func (o *OllamaLocalExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	return creds, nil
}

// Execute builds and dispatches the request, returning the response
// stream. The Ollama response body is NDJSON; downstream translators
// (in the chat handler) parse it. The executor itself does not parse
// the body — that keeps the streaming path unbuffered.
func (o *OllamaLocalExecutor) Execute(ctx context.Context, req *Request, creds *Credentials) (*Response, error) {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &ExecutorError{Code: CodeTimeout, Message: "context deadline exceeded"}
		}
		return nil, &ExecutorError{Code: CodeCanceled, Message: "context canceled"}
	}
	body, err := o.TransformRequest(req.Model, req.Body, req.Stream, creds)
	if err != nil {
		return nil, &ExecutorError{Code: CodeBadRequest, Message: "transform: " + err.Error()}
	}
	httpReq, err := o.BaseExecutor.buildHTTPRequest(ctx, req, body, creds, 0)
	if err != nil {
		return nil, &ExecutorError{Code: CodeBadRequest, Message: err.Error()}
	}
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, &ExecutorError{Code: CodeUnavailable, Message: err.Error()}
	}
	return o.BaseExecutor.toExecutorResponse(resp, 0, 1), nil
}

func init() {
	Register("ollama", func() Executor { return NewOllamaExecutor() })
}

// ParseOllamaResponse unmarshals one NDJSON line from an Ollama
// response. Returns an error when the line is not valid JSON.
func ParseOllamaResponse(line []byte) (map[string]json.RawMessage, error) {
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("ollama ndjson: %w", err)
	}
	return msg, nil
}

// OllamaModelHash computes a stable hash of an Ollama model name for
// cache-key purposes.
func OllamaModelHash(model string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(model))
	return hex.EncodeToString(h.Sum(nil))[:8]
}
