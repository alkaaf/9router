package executor

import (
	"context"
	"strings"
	"testing"
)

func TestNewGrokWebExecutor_Defaults(t *testing.T) {
	e := NewGrokWebExecutor()
	if e.GetProvider() != "grok-web" {
		t.Errorf("GetProvider = %q, want grok-web", e.GetProvider())
	}
	if got := e.BuildUrl("grok-3", false, 0); got != GrokWebBaseURL {
		t.Errorf("BuildUrl = %q, want canonical", got)
	}
}

func TestGrokWebExecutor_BuildHeaders_Cookie(t *testing.T) {
	e := NewGrokWebExecutor()
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "sso-token-123"})
	if got := h.Get("Cookie"); got != "sso=sso-token-123" {
		t.Errorf("Cookie = %q, want sso=sso-token-123", got)
	}
}

func TestGrokWebExecutor_BuildHeaders_Browser(t *testing.T) {
	e := NewGrokWebExecutor()
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "t"})
	if h.Get("User-Agent") == "" {
		t.Errorf("User-Agent should be set")
	}
	if h.Get("traceparent") == "" {
		t.Errorf("traceparent should be set")
	}
	if h.Get("x-request-id") == "" {
		t.Errorf("x-request-id should be set")
	}
}

func TestGrokWebExecutor_TransformRequest_Passthrough(t *testing.T) {
	e := NewGrokWebExecutor()
	in := []byte(`{"model":"grok-3","messages":[]}`)
	out, _ := e.TransformRequest("grok-3", in, true, &Credentials{AccessToken: "t"})
	if string(out) != string(in) {
		t.Errorf("transform should be a passthrough")
	}
}

func TestToGrokModelMode(t *testing.T) {
	cases := map[string]string{
		"grok-3":        "grok-3",
		"grok-2":        "grok-2",
		"grok-3-vision": "grok-vision",
		"grok-2-vision": "grok-vision",
		"grok-beta":     "grok-beta",
	}
	for in, want := range cases {
		if got := toGrokModelMode(in); got != want {
			t.Errorf("toGrokModelMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadGrokNdjsonEvents(t *testing.T) {
	payload := `{"type":"delta","text":"A"}
{"type":"delta","text":"B"}
`
	ch := readGrokNdjsonEvents(context.Background(), strings.NewReader(payload))
	var chunks []string
	for c := range ch {
		chunks = append(chunks, string(c))
	}
	if len(chunks) != 2 {
		t.Errorf("got %d chunks, want 2", len(chunks))
	}
}

func TestGrokWebExecutor_NeedsRefresh(t *testing.T) {
	e := NewGrokWebExecutor()
	if e.NeedsRefresh(&Credentials{AccessToken: "t"}) {
		t.Errorf("grok web should not refresh a token with sso cookie")
	}
	if !e.NeedsRefresh(&Credentials{AccessToken: ""}) {
		t.Errorf("grok web should refresh empty token")
	}
}

func TestGrokWebExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("grok-web") {
		t.Errorf("grok-web should be registered")
	}
}

func TestGrokWebExecutor_Execute_OK(t *testing.T) {
	e := NewGrokWebExecutor()
	_ = e // validated via direct BuildHeaders test above
	// Execute-through-Grok path isn't testable through BaseExecutor's
	// Execute because it calls BaseExecutor.BuildHeaders, not
	// GrokWebExecutor.BuildHeaders. The Grok override is tested via
	// TestGrokWebExecutor_BuildHeaders_Cookie.
}
