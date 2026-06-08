package executor

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewOllamaExecutor_Defaults(t *testing.T) {
	e := NewOllamaExecutor()
	if e.GetProvider() != "ollama" {
		t.Errorf("GetProvider = %q, want ollama", e.GetProvider())
	}
	if got := e.BuildUrl("llama3", false, 0); !strings.Contains(got, "localhost:11434/api/chat") {
		t.Errorf("BuildUrl = %q, should contain localhost:11434/api/chat", got)
	}
}

func TestNewOllamaExecutor_CustomHost(t *testing.T) {
	e := NewOllamaExecutorWithHost("192.168.1.100:11434")
	if !strings.Contains(e.BuildUrl("m", false, 0), "192.168.1.100:11434") {
		t.Errorf("custom host should appear in URL")
	}
}

func TestOllamaExecutor_BuildUrl(t *testing.T) {
	e := NewOllamaExecutor()
	want := "http://localhost:11434/api/chat"
	if got := e.BuildUrl("llama3", false, 0); got != want {
		t.Errorf("BuildUrl = %q, want %q", got, want)
	}
}

func TestOllamaExecutor_BuildUrl_Stream(t *testing.T) {
	e := NewOllamaExecutor()
	got := e.BuildUrl("llama3", true, 0)
	if !strings.Contains(got, "stream=true") {
		t.Errorf("stream URL should contain stream=true: %q", got)
	}
}

func TestOllamaExecutor_BuildHeaders_NoAuth(t *testing.T) {
	e := NewOllamaExecutor()
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "ignored"})
	if h.Get("Authorization") != "" {
		t.Errorf("Ollama should not set Authorization")
	}
	if h.Get("api-key") != "" {
		t.Errorf("Ollama should not set api-key")
	}
}

func TestOllamaExecutor_TransformRequest_Passthrough(t *testing.T) {
	e := NewOllamaExecutor()
	in := []byte(`{"model":"llama3","messages":[]}`)
	out, _ := e.TransformRequest("llama3", in, true, &Credentials{AccessToken: ""})
	if string(out) != string(in) {
		t.Errorf("Ollama transform should be a passthrough")
	}
}

func TestOllamaExecutor_Execute_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"model":"llama3","done":true}`))
	}))
	defer srv.Close()

	e := NewOllamaExecutorWithHost(srv.Listener.Addr().String())
	// Mutate config to point at the test server.
	e.config.BaseURLs[0] = "http://" + srv.Listener.Addr().String() + "/api/chat"
	resp, err := e.Execute(context.Background(),
		&Request{Method: "POST", Model: "llama3", Body: []byte(`{"messages":[]}`)},
		&Credentials{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
}

func TestParseOllamaResponse(t *testing.T) {
	msg, err := ParseOllamaResponse([]byte(`{"model":"l","message":{"role":"assistant","content":"hi"},"done":true}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := msg["done"]; !ok {
		t.Errorf("done key missing")
	}
	if _, err := ParseOllamaResponse([]byte("not-json")); err == nil {
		t.Errorf("bad JSON should error")
	}
}

func TestOllamaModelHash(t *testing.T) {
	if got := OllamaModelHash("llama3"); len(got) != 8 {
		t.Errorf("hash len = %d, want 8", len(got))
	}
}

func TestOllamaExecutor_NeedsRefresh(t *testing.T) {
	e := NewOllamaExecutor()
	if e.NeedsRefresh(&Credentials{AccessToken: "t"}) {
		t.Errorf("Ollama should never need refresh")
	}
}

func TestOllamaExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("ollama") {
		t.Errorf("ollama should be registered")
	}
}

// Compiled-in check for io.Reader use in NewOllamaExecutor.
var _ = bytes.NewReader
