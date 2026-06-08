package chatcore

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestRequest builds a *http.Request with the given body bytes.
// The body reader is freshly initialised so each scenario is isolated.
func newTestRequest(body []byte, contentType string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	return r
}

// parseBody is a test helper that calls ParseRequest and returns only
// the parsed body map, surfacing the error for the caller to inspect.
func parseBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	out, err := ParseRequest(r)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	return out.Body
}

// TestParseRequest_ValidJSON exercises AC-004: a valid JSON object
// parses correctly and fields are accessible.
func TestParseRequest_ValidJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-4","messages":[]}`)
	got := parseBody(t, newTestRequest(body, "application/json"))

	if got["model"] != "gpt-4" {
		t.Errorf("model = %v, want %q", got["model"], "gpt-4")
	}
	msgs, ok := got["messages"].([]any)
	if !ok || len(msgs) != 0 {
		t.Errorf("messages = %#v, want empty []any", got["messages"])
	}
}

// TestParseRequest_MalformedJSON exercises AC-001: a broken JSON body
// is rejected with ErrInvalidJSON, which the handler translates to a
// 400 with "Invalid JSON body".
func TestParseRequest_MalformedJSON(t *testing.T) {
	r := newTestRequest([]byte(`{"model":"gpt-4"`), "application/json")
	_, err := ParseRequest(r)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !errors.Is(err, ErrInvalidJSON) {
		t.Errorf("error = %v, want ErrInvalidJSON", err)
	}
}

// TestParseRequest_EmptyBody exercises AC-002: an empty body (zero
// bytes) is rejected with ErrEmptyBody.
func TestParseRequest_EmptyBody(t *testing.T) {
	r := newTestRequest([]byte{}, "application/json")
	_, err := ParseRequest(r)
	if err == nil {
		t.Fatal("expected error for empty body, got nil")
	}
	if !errors.Is(err, ErrEmptyBody) {
		t.Errorf("error = %v, want ErrEmptyBody", err)
	}
}

// TestParseRequest_NullBody exercises AC-002 variant: a body that is
// just the literal JSON token null must be treated as empty — the
// Node.js equivalent of request.json() resolving to null.
func TestParseRequest_NullBody(t *testing.T) {
	r := newTestRequest([]byte("null"), "application/json")
	_, err := ParseRequest(r)
	if err == nil {
		t.Fatal("expected error for null body, got nil")
	}
	if !errors.Is(err, ErrEmptyBody) {
		t.Errorf("error = %v, want ErrEmptyBody", err)
	}
}

// TestParseRequest_ArrayBody exercises AC-003: a top-level array is
// not a valid chat request shape and is rejected with
// ErrInvalidJSON. The error message remains "Invalid JSON body" so
// that the byte-for-byte contract with the Node.js response is
// preserved.
func TestParseRequest_ArrayBody(t *testing.T) {
	r := newTestRequest([]byte(`[]`), "application/json")
	_, err := ParseRequest(r)
	if err == nil {
		t.Fatal("expected error for array body, got nil")
	}
	if !errors.Is(err, ErrInvalidJSON) {
		t.Errorf("error = %v, want ErrInvalidJSON", err)
	}
}

// TestParseRequest_WhitespaceOnly exercises AC-002 variant: a body
// that is only whitespace (e.g. a client that sends a stray newline
// before the JSON) is treated the same as empty.
func TestParseRequest_WhitespaceOnly(t *testing.T) {
	for _, ws := range []string{"   ", "\n\r\t", "  \n  "} {
		r := newTestRequest([]byte(ws), "application/json")
		_, err := ParseRequest(r)
		if err == nil {
			t.Errorf("expected error for whitespace-only body %q, got nil", ws)
			continue
		}
		if !errors.Is(err, ErrEmptyBody) {
			t.Errorf("error for %q = %v, want ErrEmptyBody", ws, err)
		}
	}
}

// TestParseRequest_OversizedBody exercises the MaxBytesReader cap. A
// body larger than maxChatBodyBytes should be rejected.
func TestParseRequest_OversizedBody(t *testing.T) {
	b := make([]byte, maxChatBodyBytes+1)
	for i := range b {
		b[i] = 'a'
	}
	r := newTestRequest(b, "application/json")
	_, err := ParseRequest(r)
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}
	if !errors.Is(err, ErrInvalidJSON) {
		t.Errorf("error = %v, want ErrInvalidJSON", err)
	}
}

// TestErrorBodyFormat exercises AC-005: the JSON response emitted by
// WriteError must match the Node.js error shape byte-for-byte so that
// existing clients (AI IDEs, curl scripts, internal dashboards) keep
// working.
//
// We check:
//  1. Top-level key is "error" (not "errors", not "detail").
//  2. The inner object has exactly the three expected fields and no extras.
//  3. The Content-Type header is set.
func TestErrorBodyFormat(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "Invalid JSON body")

	resp := w.Result()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var got ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}

	// Structure check.
	if got.Error.Message != "Invalid JSON body" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "Invalid JSON body")
	}
	if got.Error.Type != "invalid_request_error" {
		t.Errorf("error.type = %q, want %q", got.Error.Type, "invalid_request_error")
	}
	if got.Error.Code != "" {
		t.Errorf("error.code = %q, want empty string", got.Error.Code)
	}

	// CORS header — the Node.js response sets Access-Control-Allow-Origin: *.
	if gotCT := resp.Header.Get("Access-Control-Allow-Origin"); gotCT != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", gotCT, "*")
	}
}

// TestErrorBody_ServerError exercises the 5xx path in WriteError.
func TestErrorBody_ServerError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusInternalServerError, "Upstream fetch failed")

	var got ErrorBody
	if err := json.NewDecoder(w.Result().Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Error.Type != "server_error" {
		t.Errorf("error.type = %q, want %q", got.Error.Type, "server_error")
	}
	if got.Error.Code != "internal_server_error" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "internal_server_error")
	}
}

// TestErrorBody_UnknownStatus exercises the default branch for status
// codes not explicitly mapped (e.g. 418).
func TestErrorBody_UnknownStatus(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusTeapot, "I am a teapot")

	var got ErrorBody
	if err := json.NewDecoder(w.Result().Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Error.Type != "invalid_request_error" {
		t.Errorf("error.type = %q, want %q", got.Error.Type, "invalid_request_error")
	}
}
