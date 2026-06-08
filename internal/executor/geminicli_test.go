package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGeminiCLIExecutor_Defaults(t *testing.T) {
	e := NewGeminiCLIExecutor()
	if e.GetProvider() != "gemini-cli" {
		t.Errorf("GetProvider = %q, want gemini-cli", e.GetProvider())
	}
}

func TestGeminiCLIExecutor_BuildUrl_Stream(t *testing.T) {
	e := NewGeminiCLIExecutor()
	got := e.BuildUrl("gemini-1.5-pro", true, 0)
	if !strings.Contains(got, "streamGenerateContent?alt=sse") {
		t.Errorf("stream URL should contain streamGenerateContent: %q", got)
	}
	if !strings.Contains(got, "gemini-1.5-pro") {
		t.Errorf("URL should contain model: %q", got)
	}
}

func TestGeminiCLIExecutor_BuildUrl_NonStream(t *testing.T) {
	e := NewGeminiCLIExecutor()
	got := e.BuildUrl("gemini-1.5-pro", false, 0)
	if !strings.Contains(got, ":generateContent") {
		t.Errorf("non-stream URL should contain :generateContent: %q", got)
	}
}

func TestGeminiCLIExecutor_BuildHeaders_UserAgent(t *testing.T) {
	e := NewGeminiCLIExecutor()
	e.TransformRequest("gemini-1.5-pro", []byte(`{}`), true, &Credentials{AccessToken: "t"})
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "t"})
	if !strings.Contains(h.Get("User-Agent"), "gemini-1.5-pro") {
		t.Errorf("User-Agent should contain model name: %q", h.Get("User-Agent"))
	}
}

func TestGeminiCLIExecutor_TransformRequest_Envelope(t *testing.T) {
	e := NewGeminiCLIExecutor()
	in := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	out, err := e.TransformRequest("gemini-1.5-pro", in, true, &Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env["model"] != "gemini-1.5-pro" {
		t.Errorf("envelope.model = %v, want gemini-1.5-pro", env["model"])
	}
	if _, ok := env["request"]; !ok {
		t.Errorf("envelope.request missing")
	}
}

func TestGeminiCLIExecutor_ParseError_RetryInfo(t *testing.T) {
	e := NewGeminiCLIExecutor()
	body := `{"error":{"code":429,"message":"quota","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"2s"}]}}`
	ee := e.ParseError(429, body)
	if ee.Code != CodeRateLimited {
		t.Errorf("Code = %q, want RateLimited", ee.Code)
	}
	if !strings.Contains(ee.Message, "2s") {
		t.Errorf("Message should contain retry delay, got %q", ee.Message)
	}
}

func TestParseRetryDelaySeconds(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"2s", 2},
		{"1.5s", 1},
		{"500ms", 0},
		{"1500ms", 1},
		{"", 0},
		{"x", 0},
	}
	for _, c := range cases {
		if got := ParseRetryDelaySeconds(c.in); got != c.want {
			t.Errorf("ParseRetryDelaySeconds(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestGeminiCLIExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("gemini-cli") {
		t.Errorf("gemini-cli should be registered")
	}
	if !HasSpecializedExecutor("antigravity") {
		t.Errorf("antigravity should be registered")
	}
}

func TestGeminiCLIExecutor_Execute_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	e := NewGeminiCLIExecutor()
	// Replace the base URL with the test server by mutating config
	// in place (acceptable for tests).
	e.config.BaseURLs[0] = srv.URL + "/v1internal"
	resp, err := e.Execute(context.Background(),
		&Request{Method: "POST", Model: "gemini-1.5-pro", Stream: true, Body: []byte(`{"contents":[]}`)},
		&Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
}
