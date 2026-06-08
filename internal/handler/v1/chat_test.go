package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/9router/9router/internal/chatcore"
)

// handlerRequest builds a request to the chat handler. Centralising
// this keeps the handler tests focused on assertion logic.
func handlerRequest(t *testing.T, method, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, "/v1/chat/completions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// TestHandler_ValidJSON exercises AC-004 through the full HTTP
// stack. A valid JSON object should reach the placeholder success
// path and produce a 200 with application/json content type.
func TestHandler_ValidJSON(t *testing.T) {
	r := handlerRequest(t, http.MethodPost, `{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}]}`)
	w := httptest.NewRecorder()

	ChatCompletionsHandler().ServeHTTP(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// TestHandler_MalformedJSON exercises AC-001 through the HTTP
// handler: a malformed body should surface as a 400 with the OpenAI
// error envelope.
func TestHandler_MalformedJSON(t *testing.T) {
	r := handlerRequest(t, http.MethodPost, `{"model": "gpt-4"`)
	w := httptest.NewRecorder()

	ChatCompletionsHandler().ServeHTTP(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var got chatcore.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if got.Error.Message != "Invalid JSON body" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "Invalid JSON body")
	}
	if got.Error.Type != "invalid_request_error" {
		t.Errorf("error.type = %q, want %q", got.Error.Type, "invalid_request_error")
	}
}

// TestHandler_EmptyBody exercises AC-002 through the HTTP handler.
// An empty body must surface as a 400 with the canonical error body.
func TestHandler_EmptyBody(t *testing.T) {
	r := handlerRequest(t, http.MethodPost, "")
	w := httptest.NewRecorder()

	ChatCompletionsHandler().ServeHTTP(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var got chatcore.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if got.Error.Message != "Invalid JSON body" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "Invalid JSON body")
	}
}

// TestHandler_ArrayBody exercises AC-003 through the HTTP handler.
// A top-level array must be rejected as 400 (not 200 with weird
// behaviour). The Node.js backend also rejects this case via
// `body.model` returning undefined; we keep the user-facing message
// identical.
func TestHandler_ArrayBody(t *testing.T) {
	r := handlerRequest(t, http.MethodPost, `[]`)
	w := httptest.NewRecorder()

	ChatCompletionsHandler().ServeHTTP(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var got chatcore.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if got.Error.Message != "Invalid JSON body" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "Invalid JSON body")
	}
}

// TestHandler_NullBody exercises the null-body variant — it must
// surface as 400 with the same error envelope as an empty body.
func TestHandler_NullBody(t *testing.T) {
	r := handlerRequest(t, http.MethodPost, "null")
	w := httptest.NewRecorder()

	ChatCompletionsHandler().ServeHTTP(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var got chatcore.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if got.Error.Message != "Invalid JSON body" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "Invalid JSON body")
	}
}

// TestHandler_OptionsPreflight exercises the CORS preflight path.
// The Node.js route handles OPTIONS and returns 204; we match that.
func TestHandler_OptionsPreflight(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	ChatCompletionsHandler().ServeHTTP(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if or := resp.Header.Get("Access-Control-Allow-Origin"); or != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", or, "*")
	}
	if m := resp.Header.Get("Access-Control-Allow-Methods"); m != "GET, POST, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q", m)
	}
}
