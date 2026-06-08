package chatcore

import (
	"bytes"
	"strings"
	"testing"
)

// flushBuf is a bytes.Buffer that implements Flusher for tests.
type flushBuf struct {
	bytes.Buffer
	flushes int
}

func (f *flushBuf) Flush() error {
	f.flushes++
	return nil
}

// TestSSEWriter_WriteChunk — AC-001: a single chunk produces a
// well-formed SSE event line with the expected fields.
func TestSSEWriter_WriteChunk(t *testing.T) {
	var buf flushBuf
	s := NewSSEWriter(&buf)
	if err := s.WriteChunk("gpt-4o", "hello", ""); err != nil {
		t.Fatalf("WriteChunk error: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "data: ") {
		t.Errorf("missing data: prefix: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("missing terminator: %q", got)
	}
	// decode the JSON payload to verify fields.
	payload := strings.TrimPrefix(got, "data: ")
	payload = strings.TrimSpace(payload)
	if !strings.Contains(payload, `"model":"gpt-4o"`) {
		t.Errorf("missing model in %q", payload)
	}
	if !strings.Contains(payload, `"content":"hello"`) {
		t.Errorf("missing content in %q", payload)
	}
	if !strings.Contains(payload, `"object":"chat.completion.chunk"`) {
		t.Errorf("missing object in %q", payload)
	}
	if buf.flushes != 1 {
		t.Errorf("expected 1 flush, got %d", buf.flushes)
	}
}

// TestSSEWriter_WriteDone — AC-002: WriteDone emits the
// canonical "data: [DONE]\n\n".
func TestSSEWriter_WriteDone(t *testing.T) {
	var buf flushBuf
	s := NewSSEWriter(&buf)
	if err := s.WriteDone(); err != nil {
		t.Fatalf("WriteDone error: %v", err)
	}
	if got := buf.String(); got != "data: [DONE]\n\n" {
		t.Errorf("got %q", got)
	}
}

// TestSSEWriter_WriteError — AC-003: WriteError emits a SSE error
// event with the OpenAI envelope.
func TestSSEWriter_WriteError(t *testing.T) {
	var buf flushBuf
	s := NewSSEWriter(&buf)
	if err := s.WriteError(429, "rate limit"); err != nil {
		t.Fatalf("WriteError error: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "event: error\ndata: ") {
		t.Errorf("missing event prefix: %q", got)
	}
	if !strings.Contains(got, `"code":429`) || !strings.Contains(got, `"message":"rate limit"`) {
		t.Errorf("missing error fields: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("missing terminator: %q", got)
	}
}

// TestSSEWriter_MultipleWrites — AC-004: multiple writes concatenate.
func TestSSEWriter_MultipleWrites(t *testing.T) {
	var buf flushBuf
	s := NewSSEWriter(&buf)
	for i := 0; i < 3; i++ {
		_ = s.WriteChunk("m", "x", "")
	}
	_ = s.WriteDone()
	got := buf.String()
	if c := strings.Count(got, "data: "); c != 4 {
		t.Errorf("expected 4 data: lines, got %d in %q", c, got)
	}
	if !strings.HasSuffix(got, "data: [DONE]\n\n") {
		t.Errorf("missing trailing DONE: %q", got)
	}
	if buf.flushes != 4 {
		t.Errorf("expected 4 flushes, got %d", buf.flushes)
	}
}

// TestSSEWriter_NoFlush — writes still succeed when the underlying
// writer is not a Flusher.
func TestSSEWriter_NoFlush(t *testing.T) {
	var buf bytes.Buffer
	s := NewSSEWriter(&buf)
	if err := s.WriteChunk("m", "x", ""); err != nil {
		t.Fatalf("WriteChunk: %v", err)
	}
	if !strings.Contains(buf.String(), "data: ") {
		t.Errorf("no data: line written")
	}
}

// TestSSEWriter_EmptyContent — an empty content string still
// produces a well-formed event.
func TestSSEWriter_EmptyContent(t *testing.T) {
	var buf flushBuf
	s := NewSSEWriter(&buf)
	if err := s.WriteChunk("m", "", "stop"); err != nil {
		t.Fatalf("WriteChunk: %v", err)
	}
	if !strings.Contains(buf.String(), `"finish_reason":"stop"`) {
		t.Errorf("expected finish_reason in payload, got %q", buf.String())
	}
}

// TestSSEWriter_WriteRaw — pre-shaped payload is emitted verbatim.
func TestSSEWriter_WriteRaw(t *testing.T) {
	var buf flushBuf
	s := NewSSEWriter(&buf)
	raw := map[string]any{
		"id":      "chatcmpl-1",
		"object":  "chat.completion.chunk",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "raw"}}},
	}
	if err := s.WriteRaw(raw); err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "data: ") || !strings.HasSuffix(got, "\n\n") {
		t.Errorf("malformed SSE frame: %q", got)
	}
	if !strings.Contains(got, `"id":"chatcmpl-1"`) {
		t.Errorf("missing id in payload: %q", got)
	}
}
