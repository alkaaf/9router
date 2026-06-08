package executor

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewGithubExecutor_Defaults(t *testing.T) {
	e := NewGithubExecutor()
	if e.GetProvider() != "github" {
		t.Errorf("GetProvider = %q, want github", e.GetProvider())
	}
}

func TestGithubExecutor_Headers(t *testing.T) {
	e := NewGithubExecutor()
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "copilot-tok"})
	if got := h.Get("copilot-integration-id"); got != GithubCopilotIntegrationID {
		t.Errorf("copilot-integration-id = %q, want %q", got, GithubCopilotIntegrationID)
	}
	if got := h.Get("Editor-Version"); got != GithubCopilotEditorVersion {
		t.Errorf("Editor-Version = %q, want %q", got, GithubCopilotEditorVersion)
	}
}

func TestRequiresMaxCompletionTokens(t *testing.T) {
	cases := map[string]bool{
		"gpt-5":          true,
		"gpt-5-codex":    true,
		"gpt-5.4":        true,
		"gpt-4":          false,
		"claude-3-sonne": false,
	}
	for m, want := range cases {
		if got := RequiresMaxCompletionTokens(m); got != want {
			t.Errorf("RequiresMaxCompletionTokens(%q) = %v, want %v", m, got, want)
		}
	}
}

func TestSupportsTemperature(t *testing.T) {
	if !SupportsTemperature("gpt-4") {
		t.Errorf("gpt-4 should support temperature")
	}
	if !SupportsTemperature("gpt-5") {
		t.Errorf("gpt-5 should support temperature")
	}
	if SupportsTemperature("gpt-5.4") {
		t.Errorf("gpt-5.4 should NOT support temperature")
	}
}

func TestSupportsThinking(t *testing.T) {
	if !SupportsThinking("claude-3-opus") {
		t.Errorf("claude-3 should support thinking")
	}
	if SupportsThinking("gpt-4") {
		t.Errorf("gpt-4 should NOT support thinking")
	}
}

func TestRequiresResponsesEndpoint(t *testing.T) {
	if !RequiresResponsesEndpoint("gpt-5-codex-high") {
		t.Errorf("gpt-5-codex-high should require /responses")
	}
	if RequiresResponsesEndpoint("gpt-4") {
		t.Errorf("gpt-4 should not require /responses")
	}
}

func TestGithubExecutor_TransformRequest_MaxTokensRename(t *testing.T) {
	e := NewGithubExecutor()
	in := []byte(`{"model":"gpt-5","max_tokens":1000}`)
	out, _ := e.TransformRequest("gpt-5", in, false, &Credentials{AccessToken: "t"})
	var msg map[string]any
	_ = json.Unmarshal(out, &msg)
	if _, ok := msg["max_completion_tokens"]; !ok {
		t.Errorf("max_completion_tokens should be set")
	}
	if _, ok := msg["max_tokens"]; ok {
		t.Errorf("max_tokens should be removed")
	}
}

func TestGithubExecutor_TransformRequest_DropTemperature(t *testing.T) {
	e := NewGithubExecutor()
	in := []byte(`{"model":"gpt-5.4","temperature":0.5}`)
	out, _ := e.TransformRequest("gpt-5.4", in, false, &Credentials{AccessToken: "t"})
	var msg map[string]any
	_ = json.Unmarshal(out, &msg)
	if _, ok := msg["temperature"]; ok {
		t.Errorf("temperature should be dropped for gpt-5.4")
	}
}

func TestGithubExecutor_TransformRequest_AddThinking(t *testing.T) {
	e := NewGithubExecutor()
	in := []byte(`{"model":"claude-3-opus"}`)
	out, _ := e.TransformRequest("claude-3-opus", in, false, &Credentials{AccessToken: "t"})
	if !strings.Contains(string(out), "thinking") {
		t.Errorf("claude-3 should get thinking field: %s", out)
	}
}

func TestGithubExecutor_TransformRequest_StripNonText(t *testing.T) {
	e := NewGithubExecutor()
	in := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"x"}]}]}`)
	out, _ := e.TransformRequest("gpt-4", in, false, &Credentials{AccessToken: "t"})
	if strings.Contains(string(out), "tool_use") {
		t.Errorf("tool_use content should be stripped: %s", out)
	}
	if !strings.Contains(string(out), "hi") {
		t.Errorf("text content should be preserved: %s", out)
	}
}

func TestGithubExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("github") {
		t.Errorf("github should be registered")
	}
}
