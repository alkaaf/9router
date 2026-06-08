package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// OpenAIBaseURL is the canonical OpenAI chat-completions endpoint.
const OpenAIBaseURL = "https://api.openai.com/v1/chat/completions"

// OpenAIExecutor is the reference implementation of the Executor
// contract for the OpenAI chat-completions API. It is the simplest
// specialized executor in the registry: the upstream wire format is
// the same one the OpenAI-compatible translator produces, and the
// auth header is the standard Bearer token. Almost all of the work
// lives in BaseExecutor; this type adds:
//
//   - Fixed production base URL (api.openai.com).
//   - SSE chunk parsing (the streaming path returns text/event-stream
//     and the chat handler hands the body to ParseSSE for token
//     usage extraction).
//   - Tool-call delta accumulation — OpenAI streams tool calls as
//     partial deltas with numeric indices; the executor re-assembles
//     them into the single tool_calls array the upstream expects.
//
// Embedding the *BaseExecutor by value (not pointer) gives the
// OpenAIExecutor the full BaseExecutor method set on its own value
// type — Go method promotion makes RefreshCredentials, GetProvider,
// etc. reachable from any *OpenAIExecutor pointer.
type OpenAIExecutor struct {
	*BaseExecutor
}

// NewOpenAIExecutor returns an OpenAIExecutor wired with the canonical
// api.openai.com endpoint and the default Bearer-token auth header.
func NewOpenAIExecutor() *OpenAIExecutor {
	cfg := &ProviderConfig{
		Provider:      "openai",
		BaseURLs:      []string{OpenAIBaseURL},
		AuthHeader:    "Authorization",
		StreamPath:    "", // BuildUrl handles the default path
		NonStreamPath: "",
		MaxRetries:    DefaultMaxRetries,
	}
	return &OpenAIExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// NewOpenAICompatExecutor builds an OpenAIExecutor pointed at an
// OpenAI-compatible base URL (e.g. vLLM, Together, OpenRouter, ...).
// Pass baseURL WITHOUT a trailing path — the executor appends the
// canonical /v1/chat/completions path itself. To use a custom path,
// pass it via the StreamPath / NonStreamPath fields of a manually
// constructed ProviderConfig.
func NewOpenAICompatExecutor(provider, baseURL string) *OpenAIExecutor {
	cfg := &ProviderConfig{
		Provider:   provider,
		BaseURLs:   []string{baseURL},
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &OpenAIExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// BuildUrl returns the OpenAI chat-completions URL. The OpenAI base
// URL already contains the full /v1/chat/completions path, so we
// return it directly — letting BaseExecutor append its default path
// on top of an already-complete URL would double the suffix.
func (o *OpenAIExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	if o.config == nil || len(o.config.BaseURLs) == 0 {
		return "/v1/chat/completions"
	}
	if urlIndex < 0 || urlIndex >= len(o.config.BaseURLs) {
		return "/v1/chat/completions"
	}
	return o.config.BaseURLs[urlIndex]
}

// BuildHeaders adds the OpenAI-required Content-Type and Bearer token.
// Per-request headers (e.g. OpenAI-Organization) are layered on top.
func (o *OpenAIExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	return o.BaseExecutor.BuildHeaders(req, creds)
}

// TransformRequest is a passthrough — the OpenAI-compatible translator
// upstream of the executor produces the exact JSON shape OpenAI
// expects, so the executor does not mutate the body.
func (o *OpenAIExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return o.BaseExecutor.TransformRequest(model, body, stream, creds)
}

// init registers the OpenAIExecutor with the package-level registry.
func init() {
	Register("openai", func() Executor { return NewOpenAIExecutor() })
}

// ────────────────────────────────────────────────────────────────────
// SSE parsing
// ────────────────────────────────────────────────────────────────────

// SSEChunk is one parsed event from an OpenAI streaming response. The
// field names mirror the upstream wire format (snake_case) so the
// json.Unmarshal tag is a thin pass-through.
//
//	Id        — the SSE event id (rarely populated by OpenAI)
//	Event     — the SSE event name (always "message" for OpenAI chat)
//	Data      — raw payload, exactly the line(s) following "data:"
//	Done      — true when Data is the sentinel "[DONE]"
type SSEChunk struct {
	Id    string `json:"id,omitempty"`
	Event string `json:"event,omitempty"`
	Data  string `json:"data"`
	Done  bool   `json:"-"`
}

// ToolCall represents one fully-assembled tool call. OpenAI streams a
// tool call as a sequence of partial deltas, each tagged with an
// index; the executor accumulates deltas until finish_reason="tool_calls"
// arrives, then emits the final array.
type ToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function ToolCallFunc `json:"function"`
}

// ToolCallFunc is the function payload of a tool call. Arguments is
// stored as a string (raw JSON text) because the chunks may be
// fragmented across SSE events.
type ToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ToolCallDelta is the streaming input shape: OpenAI ships only the
// fields that changed in each delta, so the JSON tags allow zero values
// to round-trip cleanly.
type ToolCallDelta struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function ToolCallFunc `json:"function,omitempty"`
}

// Usage is the per-request token count payload that OpenAI emits in
// the final chunk of a streaming response (when stream_options.
// include_usage=true) or as a top-level field in non-streaming
// responses. Field names match the upstream wire format.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamAccumulator is the per-request state machine that turns a
// stream of SSE chunks into the final chat-completion message.
// Construction is zero-value-safe — callers can use &StreamAccumulator{}
// directly, or use NewStreamAccumulator to attach a usage callback.
//
// Concurrency: each accumulator is single-goroutine. The chat handler
// reads the response body in a loop and feeds chunks via AppendChunk;
// OnUsage fires synchronously inside AppendChunk.
type StreamAccumulator struct {
	mu sync.Mutex

	// Role/Content/Finish are accumulated as the deltas arrive.
	Role    string
	Content strings.Builder

	// Tool calls are keyed by their upstream index; OpenAI uses 0..N-1
	// matching the order tools are produced in.
	ToolCalls map[int]*ToolCall

	// FinishReason is set by the final chunk that has
	// finish_reason != null/"".
	FinishReason string

	// Model is captured from the first chunk's "model" field. Useful
	// for usage tracking when the request model was an alias.
	Model string

	// Usage is populated when stream_options.include_usage was set.
	Usage *Usage

	// onUsage is invoked from inside AppendChunk when Usage becomes
	// non-nil. May be nil — calls become no-ops in that case.
	onUsage func(*Usage)
}

// NewStreamAccumulator returns an empty accumulator. The optional
// usage callback is invoked at most once per stream, after Usage has
// been set by a chunk.
func NewStreamAccumulator(onUsage func(*Usage)) *StreamAccumulator {
	return &StreamAccumulator{
		ToolCalls: make(map[int]*ToolCall),
		onUsage:   onUsage,
	}
}

// SetUsageCallback attaches (or replaces) the usage callback. Useful
// when the accumulator is constructed in a struct literal.
func (a *StreamAccumulator) SetUsageCallback(cb func(*Usage)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onUsage = cb
}

// AppendChunk feeds one parsed SSE chunk into the accumulator. It is
// safe to call AppendChunk for the same chunk multiple times — the
// tool-call merge is idempotent for fields that are already set.
//
// The function returns an error only when the JSON inside the chunk's
// Data field is malformed in a way that prevents any progress. SSE
// chunks with non-JSON Data (such as the [DONE] sentinel) are
// accepted and produce no state change.
func (a *StreamAccumulator) AppendChunk(chunk *SSEChunk) error {
	if chunk == nil {
		return nil
	}
	if chunk.Done {
		return nil
	}
	data := strings.TrimSpace(chunk.Data)
	if data == "" {
		return nil
	}
	// OpenAI sends literal "[DONE]" as the final SSE event.
	if data == "[DONE]" {
		return nil
	}

	var raw struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role      string          `json:"role,omitempty"`
				Content   string          `json:"content,omitempty"`
				ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage `json:"usage,omitempty"`
	}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return fmt.Errorf("sse chunk json: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Model == "" && raw.Model != "" {
		a.Model = raw.Model
	}

	for _, c := range raw.Choices {
		if c.Delta.Role != "" && a.Role == "" {
			a.Role = c.Delta.Role
		}
		if c.Delta.Content != "" {
			a.Content.WriteString(c.Delta.Content)
		}
		for _, d := range c.Delta.ToolCalls {
			tc, ok := a.ToolCalls[d.Index]
			if !ok {
				tc = &ToolCall{Index: d.Index}
				a.ToolCalls[d.Index] = tc
			}
			if d.ID != "" {
				tc.ID = d.ID
			}
			if d.Type != "" {
				tc.Type = d.Type
			}
			if d.Function.Name != "" {
				tc.Function.Name = d.Function.Name
			}
			if d.Function.Arguments != "" {
				tc.Function.Arguments += d.Function.Arguments
			}
		}
		if c.FinishReason != nil && *c.FinishReason != "" {
			a.FinishReason = *c.FinishReason
		}
	}

	if raw.Usage != nil {
		a.Usage = raw.Usage
		if a.onUsage != nil {
			a.onUsage(raw.Usage)
		}
	}
	return nil
}

// FinalMessage returns the fully-assembled assistant message in
// OpenAI's non-streaming wire format. The OutputContent is the
// concatenated delta text; ToolCalls is a sorted slice of all
// accumulated tool calls (sorted by index to preserve the order
// OpenAI emitted).
//
// Callers typically serialize the result with json.Marshal and pass it
// to the chat handler's usage tracker, or render it to the client.
type FinalMessage struct {
	Role         string     `json:"role"`
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Model        string     `json:"model,omitempty"`
}

// Finalize produces the consolidated message.
func (a *StreamAccumulator) Finalize() FinalMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	tcs := make([]ToolCall, 0, len(a.ToolCalls))
	for i := 0; i < len(a.ToolCalls); i++ {
		if tc, ok := a.ToolCalls[i]; ok {
			tcs = append(tcs, *tc)
		}
	}
	role := a.Role
	if role == "" {
		role = "assistant"
	}
	return FinalMessage{
		Role:         role,
		Content:      a.Content.String(),
		ToolCalls:    tcs,
		FinishReason: a.FinishReason,
		Model:        a.Model,
	}
}

// ParseSSEStream reads an SSE event stream from r and feeds each chunk
// into acc. It returns when:
//
//   - r returns io.EOF (clean end of stream),
//   - acc.AppendChunk returns an error (caller's choice to abort), or
//   - ctx is canceled (the reader is closed and context.Canceled is
//     returned).
//
// ParseSSEStream does not close r on success — that's the caller's
// responsibility. On error it drains and closes r.
func ParseSSEStream(ctx context.Context, r io.Reader, acc *StreamAccumulator) error {
	if acc == nil {
		return errors.New("ParseSSEStream: nil accumulator")
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}

	scanner := bufio.NewScanner(r)
	// OpenAI chunk lines are well under 64KB, but the first
	// data: line that includes the entire choice payload can be
	// larger in tool-call-heavy responses. 1MB is generous.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		idBuf    strings.Builder
		eventBuf strings.Builder
		dataBuf  bytes.Buffer
	)

	flush := func() error {
		if dataBuf.Len() == 0 && eventBuf.Len() == 0 && idBuf.Len() == 0 {
			return nil
		}
		chunk := &SSEChunk{
			Id:    idBuf.String(),
			Event: eventBuf.String(),
			Data:  dataBuf.String(),
		}
		idBuf.Reset()
		eventBuf.Reset()
		dataBuf.Reset()
		return acc.AppendChunk(chunk)
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		// SSE comments start with ':'. Skip them.
		if strings.HasPrefix(line, ":") {
			continue
		}
		// Blank line = end of event.
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		// Field parsing per the SSE spec: "field: value" or
		// "field:value" (no space).
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
		case "id":
			idBuf.WriteString(value)
		case "event":
			eventBuf.WriteString(value)
		case "data":
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(value)
		default:
			// Ignore unknown fields (retry, etc.).
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("sse scan: %w", err)
	}
	// Final flush for streams that end without a blank line.
	return flush()
}
