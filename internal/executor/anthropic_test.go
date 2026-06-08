package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAnthropicExecutor_Defaults(t *testing.T) {
	e := NewAnthropicExecutor()
	if e.GetProvider() != "anthropic" {
		t.Errorf("GetProvider = %q, want anthropic", e.GetProvider())
	}
	if got := e.BuildUrl("claude-3", false, 0); got != "https://api.anthropic.com/v1/messages" {
		t.Errorf("BuildUrl = %q, want canonical", got)
	}
}

func TestAnthropicExecutor_Headers(t *testing.T) {
	e := NewAnthropicExecutor()
	h := e.BuildHeaders(&Request{Model: "claude-3"}, &Credentials{AccessToken: "sk-ant-abc"})
	if got := h.Get("x-api-key"); got != "sk-ant-abc" {
		t.Errorf("x-api-key = %q, want sk-ant-abc", got)
	}
	if got := h.Get("anthropic-version"); got != AnthropicAPIVersion {
		t.Errorf("anthropic-version = %q, want %q", got, AnthropicAPIVersion)
	}
	if got := h.Get(AnthropicDirectAccessHeader); got != "true" {
		t.Errorf("%s = %q, want true", AnthropicDirectAccessHeader, got)
	}
	if h.Get("Authorization") != "" {
		t.Errorf("Authorization should not be set for Anthropic")
	}
}

func TestAnthropicExecutor_TransformRequest_AddsMaxTokens(t *testing.T) {
	e := NewAnthropicExecutor()
	in := []byte(`{"model":"claude-3","messages":[]}`)
	out, err := e.TransformRequest("claude-3", in, false, &Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := msg["max_tokens"]; !ok {
		t.Errorf("max_tokens should be set")
	}
}

func TestAnthropicExecutor_TransformRequest_PreservesExplicit(t *testing.T) {
	e := NewAnthropicExecutor()
	in := []byte(`{"model":"claude-3","max_tokens":1000,"messages":[]}`)
	out, _ := e.TransformRequest("claude-3", in, false, &Credentials{AccessToken: "t"})
	var msg map[string]any
	_ = json.Unmarshal(out, &msg)
	if got, _ := msg["max_tokens"].(float64); got != 1000 {
		t.Errorf("max_tokens = %v, want 1000", msg["max_tokens"])
	}
}

func TestAnthropicExecutor_TransformRequest_PassthroughOnNonJSON(t *testing.T) {
	e := NewAnthropicExecutor()
	in := []byte("not-json")
	out, _ := e.TransformRequest("m", in, false, &Credentials{AccessToken: "t"})
	if string(out) != "not-json" {
		t.Errorf("non-JSON should pass through unchanged")
	}
}

func TestAnthropicExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("anthropic") {
		t.Errorf("anthropic should be registered")
	}
	e := GetExecutor("anthropic")
	if _, ok := e.(*AnthropicExecutor); !ok {
		t.Errorf("GetExecutor(anthropic) = %T, want *AnthropicExecutor", e)
	}
}

// ────────────────────────────────────────────────────────────────────
// SSE adapter
// ────────────────────────────────────────────────────────────────────

func TestAnthropicSSEAdapter_MessageStopBecomesDone(t *testing.T) {
	var buf bytes.Buffer
	a := NewAnthropicSSEAdapter(&buf)

	input := "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	if err := a.Run(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "data: [DONE]") {
		t.Errorf("output should contain [DONE], got %q", buf.String())
	}
}

func TestAnthropicSSEAdapter_PassesThroughEvents(t *testing.T) {
	var buf bytes.Buffer
	a := NewAnthropicSSEAdapter(&buf)

	input := "" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\"}\n" +
		"\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n" +
		"\n"
	if err := a.Run(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "data: {\"type\":\"message_start\"}\n\n") {
		t.Errorf("message_start not forwarded: %q", out)
	}
	if !strings.Contains(out, "\"text\":\"hi\"") {
		t.Errorf("content_block_delta not forwarded: %q", out)
	}
}

func TestAnthropicSSEAdapter_CommentLines(t *testing.T) {
	var buf bytes.Buffer
	a := NewAnthropicSSEAdapter(&buf)
	input := ":ping\n\nevent: message_stop\ndata: x\n\n"
	if err := a.Run(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "[DONE]") {
		t.Errorf("expected [DONE] in output: %q", buf.String())
	}
}

func TestAnthropicSSEAdapter_ContextCanceled(t *testing.T) {
	var buf bytes.Buffer
	a := NewAnthropicSSEAdapter(&buf)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.Run(ctx, strings.NewReader("event: message_start\ndata: x\n\n"))
	if err == nil {
		t.Errorf("expected error on canceled context")
	}
}

func TestAnthropicExecutor_Execute_AnthropicError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad input"}}`))
	}))
	defer srv.Close()

	e := NewAnthropicCompatExecutor("anthropic", srv.URL)
	resp, err := e.Execute(context.Background(),
		&Request{Method: "POST", Model: "claude-3", Body: []byte(`{"messages":[]}`)},
		&Credentials{AccessToken: "x"})
	if err != nil {
		t.Fatalf("Execute should return response (handler will call ParseError): %v", err)
	}
	if resp.Status != 400 {
		t.Errorf("Status = %d, want 400", resp.Status)
	}
	// The handler is expected to call ParseError on the response.
	body := readAllString(t, resp.Body)
	ee := e.ParseError(resp.Status, body)
	if ee.Code != CodeBadRequest {
		t.Errorf("ParseError.Code = %q, want BadRequest", ee.Code)
	}
	if !strings.Contains(ee.Message, "bad input") {
		t.Errorf("Message should contain upstream error, got %q", ee.Message)
	}
}

func readAllString(t *testing.T, r interface{ Read([]byte) (int, error) }) string {
	t.Helper()
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}
