package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicBaseURL is the canonical Claude messages endpoint.
const AnthropicBaseURL = "https://api.anthropic.com/v1/messages"

// AnthropicAPIVersion is the date-stamped API version header the
// executor sends with every request. The value matches the one used by
// the Node.js BaseExecutor in open-sse/executors/anthropic.js.
const AnthropicAPIVersion = "2023-06-01"

// AnthropicDirectAccessHeader bypasses Anthropic's normal proxy
// routing. The Node.js implementation sets this header to enable
// direct access to the underlying API.
const AnthropicDirectAccessHeader = "anthropic-dangerous-direct-access"

// AnthropicBetaHeader is sent on some models that need beta access
// (e.g. extended-context or vision models). The executor does not set
// it by default — callers can opt in via the request headers.
const AnthropicBetaHeader = "anthropic-beta"

// AnthropicExecutor is the Claude Messages API executor. It differs
// from the OpenAI executor in three ways:
//
//   - Authentication uses x-api-key, not Bearer.
//   - The body uses Anthropic's content-block schema (content: [],
//     content blocks of type "text" / "tool_use" / "tool_result" /
//     "thinking"). The translator upstream of the executor produces
//     this shape; TransformRequest in this executor adds `max_tokens`
//     if the request omits it.
//   - SSE events use a typed event: prefix (message_start,
//     content_block_start, content_block_delta, ...). The
//     AnthropicSSEAdapter re-emits the same content as OpenAI-shaped
//     SSE so the rest of the pipeline can stay wire-format agnostic.
type AnthropicExecutor struct {
	*BaseExecutor
}

// NewAnthropicExecutor returns an AnthropicExecutor wired to the
// canonical Claude API endpoint.
func NewAnthropicExecutor() *AnthropicExecutor {
	cfg := &ProviderConfig{
		Provider:   "anthropic",
		BaseURLs:   []string{AnthropicBaseURL},
		AuthHeader: "x-api-key",
		MaxRetries: DefaultMaxRetries,
	}
	return &AnthropicExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// NewAnthropicCompatExecutor builds an AnthropicExecutor for an
// Anthropic-compatible endpoint (e.g. a private Bedrock proxy or a
// self-hosted Claude clone). baseURL should NOT include the /v1/messages
// suffix — the executor appends it.
func NewAnthropicCompatExecutor(provider, baseURL string) *AnthropicExecutor {
	cfg := &ProviderConfig{
		Provider:   provider,
		BaseURLs:   []string{baseURL},
		AuthHeader: "x-api-key",
		MaxRetries: DefaultMaxRetries,
	}
	return &AnthropicExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// BuildUrl returns the Anthropic messages URL. The Anthropic base URL
// already contains /v1/messages so the executor returns it as-is — the
// stream parameter goes in the body, not the URL.
func (a *AnthropicExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	if a.config == nil || len(a.config.BaseURLs) == 0 {
		return "/v1/messages"
	}
	if urlIndex < 0 || urlIndex >= len(a.config.BaseURLs) {
		return "/v1/messages"
	}
	return a.config.BaseURLs[urlIndex]
}

// BuildHeaders adds the Anthropic-specific auth and version headers.
// Per-request headers (anthropic-beta, etc.) win on conflict.
func (a *AnthropicExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := a.BaseExecutor.BuildHeaders(req, creds)
	h.Set("anthropic-version", AnthropicAPIVersion)
	h.Set(AnthropicDirectAccessHeader, "true")
	// The default AuthHeader for Anthropic is "x-api-key" with the
	// token as a plain value (not Bearer-prefixed). The base
	// BuildHeaders already handled the non-Bearer case via the
	// AuthHeader config — this is just a defensive re-assertion.
	return h
}

// TransformRequest mutates the body to add the `max_tokens` field if
// the upstream translator omitted it. The Anthropic API rejects
// requests without max_tokens, so this is a safety net rather than a
// full transform pipeline.
func (a *AnthropicExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		// Body is not JSON — pass it through unchanged.
		return body, nil
	}
	if _, ok := msg["max_tokens"]; ok {
		return body, nil
	}
	// Pick a max_tokens default based on model family. Claude 3.x
	// supports up to 4K output, but we use 4096 as a safe default that
	// works across all current Claude models.
	msg["max_tokens"] = json.RawMessage(`4096`)
	out, err := json.Marshal(msg)
	if err != nil {
		return body, nil
	}
	return out, nil
}

// init registers the AnthropicExecutor with the package-level
// registry.
func init() {
	Register("anthropic", func() Executor { return NewAnthropicExecutor() })
}

// ────────────────────────────────────────────────────────────────────
// Anthropic SSE event types
// ────────────────────────────────────────────────────────────────────

// AnthropicEventType is the value of the SSE "event:" field. The
// executor only cares about a subset — the others (ping, error) are
// passed through as-is.
type AnthropicEventType string

const (
	EventMessageStart      AnthropicEventType = "message_start"
	EventContentBlockStart AnthropicEventType = "content_block_start"
	EventContentBlockDelta AnthropicEventType = "content_block_delta"
	EventContentBlockStop  AnthropicEventType = "content_block_stop"
	EventMessageDelta      AnthropicEventType = "message_delta"
	EventMessageStop       AnthropicEventType = "message_stop"
	EventError             AnthropicEventType = "error"
	EventPing              AnthropicEventType = "ping"
)

// AnthropicSSEEvent is one parsed event from a Claude streaming
// response. The Data field carries the raw JSON payload as the
// executor received it — translators downstream may want the verbatim
// bytes for the OpenAI → Claude direction.
type AnthropicSSEEvent struct {
	Event AnthropicEventType
	Data  string
}

// AnthropicSSEAdapter reads an Anthropic-format SSE stream and emits
// OpenAI-format SSE bytes. It does not parse the JSON payloads — the
// OpenAI SSE writer can do that downstream — but it does know enough
// to recognize the [DONE] sentinel and to surface errors as a single
// error chunk.
//
// The adapter writes to w in OpenAI SSE form: each event becomes a
// "data: <json>\n\n" line. When a Claude message_stop event arrives,
// the adapter emits a final "data: [DONE]\n\n" so downstream consumers
// can rely on the OpenAI termination protocol.
//
// Usage:
//
//	adapt := NewAnthropicSSEAdapter(w)
//	for adapt.Scan(r) { ... }
type AnthropicSSEAdapter struct {
	w          io.Writer
	scanBuf    []byte
	hasWritten bool
}

// NewAnthropicSSEAdapter returns an adapter writing to w.
func NewAnthropicSSEAdapter(w io.Writer) *AnthropicSSEAdapter {
	return &AnthropicSSEAdapter{w: w, scanBuf: make([]byte, 0, 64*1024)}
}

// Run reads Anthropic SSE from r, transforms each event, and writes
// OpenAI-shaped SSE bytes to the adapter's writer. It returns when
// the upstream stream ends (io.EOF), the context is canceled, or a
// write error occurs.
func (a *AnthropicSSEAdapter) Run(ctx context.Context, r io.Reader) error {
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		eventType string
		dataBuf   strings.Builder
	)

	flush := func() error {
		if dataBuf.Len() == 0 && eventType == "" {
			return nil
		}
		ev := AnthropicSSEEvent{
			Event: AnthropicEventType(eventType),
			Data:  dataBuf.String(),
		}
		dataBuf.Reset()
		eventType = ""
		return a.writeEvent(ev)
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		if strings.HasPrefix(line, ":") {
			continue
		}
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		var field, value string
		if idx := strings.IndexByte(line, ':'); idx >= 0 {
			field = line[:idx]
			value = line[idx+1:]
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
		} else {
			field = line
		}
		switch field {
		case "event":
			eventType = value
		case "data":
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("anthropic sse: %w", err)
	}
	return flush()
}

// writeEvent emits the OpenAI-shaped SSE bytes for one Anthropic
// event. The data is forwarded as-is, except for message_stop which
// is replaced with [DONE] so downstream OpenAI clients see the
// expected termination sentinel.
func (a *AnthropicSSEAdapter) writeEvent(ev AnthropicSSEEvent) error {
	if ev.Event == EventMessageStop {
		_, err := io.WriteString(a.w, "data: [DONE]\n\n")
		return err
	}
	if ev.Data == "" {
		return nil
	}
	if _, err := io.WriteString(a.w, "data: "); err != nil {
		return err
	}
	if _, err := io.WriteString(a.w, ev.Data); err != nil {
		return err
	}
	if _, err := io.WriteString(a.w, "\n\n"); err != nil {
		return err
	}
	a.hasWritten = true
	return nil
}
