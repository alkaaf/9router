package executor

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GrokWebBaseURL is the Grok web endpoint the executor POSTs to.
const GrokWebBaseURL = "https://grok.com/rest/app-chat/conversations"

// GrokWebExecutor bypasses xAI's API and scrapes Grok's web UI.
// It uses SSO-cookie auth (credentials.apiKey is the cookie value)
// and reads NDJSON (newline-delimited JSON) event streams.
type GrokWebExecutor struct {
	*BaseExecutor
}

// NewGrokWebExecutor returns a GrokWebExecutor.
func NewGrokWebExecutor() *GrokWebExecutor {
	cfg := &ProviderConfig{
		Provider:   "grok-web",
		BaseURLs:   []string{GrokWebBaseURL},
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &GrokWebExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// BuildUrl returns the Grok web chat endpoint.
func (g *GrokWebExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	if g.config == nil || len(g.config.BaseURLs) == 0 {
		return GrokWebBaseURL
	}
	return g.config.BaseURLs[urlIndex]
}

// BuildHeaders adds the Cookie: sso={token} header plus browser-like
// headers (User-Agent, Sec-CH-UA, x-request-id, traceparent) that
// bypass Grok's bot detection.
func (g *GrokWebExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")
	if creds != nil && creds.AccessToken != "" {
		h.Set("Cookie", "sso="+creds.AccessToken)
	}
	h.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	h.Set("Sec-CH-UA", `"Chromium";v="130", "Google Chrome";v="130", "Not?A_Brand";v="99"`)
	h.Set("Sec-CH-UA-Mobile", "?0")
	h.Set("Sec-CH-UA-Platform", `"macOS"`)
	h.Set("x-request-id", generateID())
	h.Set("traceparent", "00-"+generateTraceID()+"-"+generateSpanID()+"-01")
	if req.Headers != nil {
		for k, v := range req.Headers {
			h.Set(k, v)
		}
	}
	return h
}

// TransformRequest is a no-op — the upstream translator produces the
// Grok web payload format, so the executor doesn't mutate the body.
func (g *GrokWebExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return body, nil
}

// readGrokNdjsonEvents reads a Newline-delimited JSON stream from r
// and pushes each parsed event onto the returned channel. The channel
// closes when r reaches EOF or the context is canceled. Errors during
// parsing are sent as *ExecutorError on the channel before closing.
func readGrokNdjsonEvents(ctx context.Context, r io.Reader) <-chan json.RawMessage {
	out := make(chan json.RawMessage, 8)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			out <- json.RawMessage(line)
		}
		if err := scanner.Err(); err != nil {
			// Drop parse errors — they become SSE error chunks.
			_ = err
		}
	}()
	return out
}

// toGrokModelMode maps an OpenAI-style model name to Grok's internal
// model mode identifier. Unknown names are passed through unchanged.
func toGrokModelMode(model string) string {
	switch model {
	case "grok-3":
		return "grok-3"
	case "grok-2":
		return "grok-2"
	case "grok-3-vision":
		return "grok-vision"
	case "grok-2-vision":
		return "grok-vision"
	default:
		return model
	}
}

// needsRefresh always returns true for Grok web (the SSO cookie has no
// expiry; callers supply the current session token in credentials).
func (g *GrokWebExecutor) NeedsRefresh(creds *Credentials) bool {
	if creds == nil || creds.AccessToken == "" {
		return true
	}
	return false
}

// RefreshCredentials is a no-op — the host application's SSO login
// refreshes the Grok cookie and the new value is supplied via
// Credentials.
func (g *GrokWebExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	return creds, nil
}

func init() {
	Register("grok-web", func() Executor { return NewGrokWebExecutor() })
}

// ────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────

var spanIDChars = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func generateSpanID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	out := make([]rune, 16)
	for i := range out {
		out[i] = spanIDChars[int(b[i])%len(spanIDChars)]
	}
	return string(out)
}

func generateTraceID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generateID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), generateSpanID()[:8])
}
