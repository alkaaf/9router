package executor

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewPerplexityWebExecutor_Defaults(t *testing.T) {
	e := NewPerplexityWebExecutor()
	if e.GetProvider() != "perplexity-web" {
		t.Errorf("GetProvider = %q, want perplexity-web", e.GetProvider())
	}
}

func TestPerplexityWebExecutor_BuildHeaders_Cookie(t *testing.T) {
	e := NewPerplexityWebExecutor()
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "session-tok-123"})
	if got := h.Get("Cookie"); got != "__Secure-next-auth.session-token=session-tok-123" {
		t.Errorf("Cookie = %q", got)
	}
}

func TestPerplexityWebExecutor_TransformRequest_Passthrough(t *testing.T) {
	e := NewPerplexityWebExecutor()
	in := []byte(`{"query_str":"hi"}`)
	out, _ := e.TransformRequest("pplx-sonar", in, true, &Credentials{AccessToken: "t"})
	if string(out) != string(in) {
		t.Errorf("transform should be a passthrough")
	}
}

func TestPerplexityWebExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("perplexity-web") {
		t.Errorf("perplexity-web should be registered")
	}
}

func TestCleanPplxResponse(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain text", "plain text"},
		{"<?xml version='1.0'?>rest", "rest"},
		{"before<sup>1</sup>after", "beforeafter"},
		{"[[a]]", "[a]"},
	}
	for _, c := range cases {
		if got := cleanPplxResponse(c.in); got != c.want {
			t.Errorf("cleanPplxResponse(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReadPplxSseEvents(t *testing.T) {
	payload := "" +
		"data: {\"answer\":\"hi\"}\n\n" +
		"data: {\"answer\":\" there\"}\n\n"
	ch := ReadPplxSseEvents(context.Background(), strings.NewReader(payload))
	var out []string
	for ev := range ch {
		out = append(out, ev)
	}
	if len(out) != 2 {
		t.Errorf("got %d events, want 2", len(out))
	}
}

func TestFormatPerplexityAnswer(t *testing.T) {
	ans, err := FormatPerplexityAnswer(`{"answer":"hi"}`)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if ans != "hi" {
		t.Errorf("Answer = %q, want hi", ans)
	}
	if _, err := FormatPerplexityAnswer("not-json"); err == nil {
		t.Errorf("bad JSON should error")
	}
}

func TestPerplexitySessionCache(t *testing.T) {
	c := newPerplexitySessionCache()
	c.Put("k1", "tok1")
	if got := c.Get("k1"); got != "tok1" {
		t.Errorf("Get k1 = %q, want tok1", got)
	}
	// Manually expire.
	c.mu.Lock()
	c.sessions["k2"] = perplexitySession{sessionToken: "tok2", createdAt: time.Now().Add(-2 * time.Hour)}
	c.mu.Unlock()
	if got := c.Get("k2"); got != "" {
		t.Errorf("expired k2 = %q, want empty", got)
	}
}

func TestPerplexitySessionCache_Eviction(t *testing.T) {
	c := &perplexitySessionCache{sessions: make(map[string]perplexitySession), max: 2}
	c.Put("a", "1")
	time.Sleep(2 * time.Millisecond)
	c.Put("b", "2")
	time.Sleep(2 * time.Millisecond)
	c.Put("c", "3") // should evict "a"
	if c.Get("a") != "" {
		t.Errorf("a should have been evicted")
	}
	if c.Get("b") != "2" {
		t.Errorf("b should still be present")
	}
	if c.Get("c") != "3" {
		t.Errorf("c should be present")
	}
}

func TestPerplexityModelMap(t *testing.T) {
	cases := map[string]string{
		"pplx-sonar":   "experimental",
		"pplx-online":  "turbo",
		"pplx-pro":     "pplx_pro",
		"pplx-sonar-r": "sonar",
	}
	for in, want := range cases {
		if got, ok := PerplexityModelMap[in]; !ok || got != want {
			t.Errorf("PerplexityModelMap[%q] = (%q, %v), want (%q, true)", in, got, ok, want)
		}
	}
}
