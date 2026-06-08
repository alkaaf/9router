package chatcore

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Flusher is the subset of http.Flusher and bufio.Writer that
// SSEWriter needs to push bytes downstream as they are written.
type Flusher interface {
	Flush() error
}

// SSEWriter emits Server-Sent Events to an underlying writer. The
// wire format is exactly:
//
//	data: <json>\n\n
//	data: [DONE]\n\n
//
// Each Write* call writes a single SSE event and flushes.
//
// The writer is single-goroutine — concurrent calls would interleave
// bytes and break the protocol. The HTTP handler that owns the
// stream must serialise writes.
type SSEWriter struct {
	w       io.Writer
	flusher Flusher
	enc     *json.Encoder
}

// NewSSEWriter wraps w. If w also implements Flusher (http.ResponseWriter
// or *bufio.Writer) the writer is flushed after every event.
func NewSSEWriter(w io.Writer) *SSEWriter {
	f, _ := w.(Flusher)
	return &SSEWriter{w: w, flusher: f}
}

// SSEEvent is the JSON payload written for each chat completion
// chunk. It mirrors the OpenAI streaming response shape (model +
// choices[].delta.content). Additional fields (usage, finish_reason)
// are populated when the upstream signals completion.
type SSEEvent struct {
	Model    string         `json:"model"`
	Choices  []SSEChoice    `json:"choices"`
	Usage    *SSEUsage      `json:"usage,omitempty"`
	Created  int64          `json:"created,omitempty"`
	Extras   map[string]any `json:"-"` // dropped; reserved for future use
	Object   string         `json:"object,omitempty"`
	ID       string         `json:"id,omitempty"`
}

// SSEChoice is a single choice inside an SSE event.
type SSEChoice struct {
	Index        int    `json:"index"`
	Delta        Delta  `json:"delta"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// Delta is the streamed token payload.
type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// SSEUsage is the (optional) token-usage block in the final event.
type SSEUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// WriteChunk emits one streaming chunk. model, content, and
// finishReason are mapped onto the OpenAI-style SSE payload.
func (s *SSEWriter) WriteChunk(model, content, finishReason string) error {
	ev := SSEEvent{
		Object:  "chat.completion.chunk",
		Model:   model,
		Choices: []SSEChoice{{Index: 0, Delta: Delta{Role: "assistant", Content: content}, FinishReason: finishReason}},
	}
	return s.writeJSON(ev)
}

// WriteRaw emits a fully-formed SSE event whose payload is supplied
// by the caller. The payload is serialised as JSON without further
// transformation. Use this when the upstream executor hands you a
// pre-shaped OpenAI chunk.
func (s *SSEWriter) WriteRaw(payload any) error {
	return s.writeJSON(payload)
}

// WriteDone emits the terminating "data: [DONE]\n\n" event.
func (s *SSEWriter) WriteDone() error {
	if _, err := io.WriteString(s.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	s.maybeFlush()
	return nil
}

// WriteError emits an SSE error event in OpenAI's error envelope.
// The status code is included so consumers can log it.
//
// Wire format:
//
//	event: error\ndata: {"error":{"code":<status>,"message":"<msg>"}}\n\n
func (s *SSEWriter) WriteError(statusCode int, message string) error {
	payload := map[string]any{
		"error": map[string]any{
			"code":    statusCode,
			"message": message,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(s.w, "event: error\ndata: "); err != nil {
		return err
	}
	if _, err := s.w.Write(body); err != nil {
		return err
	}
	if _, err := io.WriteString(s.w, "\n\n"); err != nil {
		return err
	}
	s.maybeFlush()
	return nil
}

// writeJSON serialises v as JSON and writes the SSE data: line.
func (s *SSEWriter) writeJSON(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("sse marshal: %w", err)
	}
	var b strings.Builder
	b.WriteString("data: ")
	b.Write(body)
	b.WriteString("\n\n")
	if _, err := io.WriteString(s.w, b.String()); err != nil {
		return err
	}
	s.maybeFlush()
	return nil
}

func (s *SSEWriter) maybeFlush() {
	if s.flusher != nil {
		_ = s.flusher.Flush()
	}
}
