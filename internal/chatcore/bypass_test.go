package chatcore

import (
	"testing"
)

func msg(role, content string) map[string]any {
	return map[string]any{"role": role, "content": content}
}

func msgs(v ...map[string]any) []any {
	out := make([]any, len(v))
	for i, m := range v {
		out[i] = m
	}
	return out
}

// TestBypass_EachPattern — AC-001: each of the 5 patterns triggers
// bypass.
func TestBypass_EachPattern(t *testing.T) {
	tests := []struct {
		name         string
		messages     []any
		userAgent    string
		ccFilter     bool
		wantBypass   bool
		wantNaming   bool
		wantModel    string
	}{
		{"title-extraction", msgs(msg("user", "hello"), msg("assistant", `{"title":"x"}`)), "claude-cli/1.0", false, true, false, "bypass/title"},
		{"warmup", msgs(msg("user", "Warmup")), "claude-cli/1.0", false, true, false, "bypass/warmup"},
		{"count", msgs(msg("user", "count")), "claude-cli/1.0", false, true, false, "bypass/count"},
		{"skip", msgs(msg("user", "please skip this")), "claude-cli/1.0", false, true, false, "bypass/skip"},
		{"naming-on", msgs(msg("system", "isNewTopic=true"), msg("user", "hi")), "claude-cli/1.0", true, true, true, "bypass/naming"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]any{"messages": tt.messages}
			got := CheckBypass(body, tt.userAgent, tt.ccFilter)
			if got == nil && tt.wantBypass {
				t.Fatal("expected bypass, got nil")
			}
			if got != nil {
				if got.Bypass != tt.wantBypass {
					t.Errorf("Bypass = %v, want %v", got.Bypass, tt.wantBypass)
				}
				if got.NamingBypass != tt.wantNaming {
					t.Errorf("NamingBypass = %v, want %v", got.NamingBypass, tt.wantNaming)
				}
				if got.Response != nil && got.Response.Model != tt.wantModel {
					t.Errorf("Model = %q, want %q", got.Response.Model, tt.wantModel)
				}
			}
		})
	}
}

// TestBypass_NonClaude — AC-002: non-claude-cli user-agent → no
// bypass.
func TestBypass_NonClaude(t *testing.T) {
	body := map[string]any{"messages": msgs(msg("user", "Warmup"))}
	if got := CheckBypass(body, "curl/7.68", false); got != nil {
		t.Errorf("expected nil bypass for non-claude-cli, got %+v", got)
	}
}

// TestBypass_MultiplePatterns — AC-003: first match wins.
func TestBypass_MultiplePatterns(t *testing.T) {
	// Two messages: first is "skip this", second is "count".
	// skip runs first in source order; count only matches when
	// there is exactly one user message. With two user messages
	// only the skip pattern can fire.
	body := map[string]any{"messages": msgs(msg("user", "please skip this"), msg("user", "count"))}
	got := CheckBypass(body, "claude-cli/1.0", false)
	if got == nil || got.Response == nil {
		t.Fatal("expected bypass")
	}
	if got.Response.Model != "bypass/skip" {
		t.Errorf("expected skip to win over count, got %q", got.Response.Model)
	}
}

// TestBypass_EmptyMessages — AC-004: empty messages array → no
// bypass.
func TestBypass_EmptyMessages(t *testing.T) {
	if got := CheckBypass(map[string]any{"messages": []any{}}, "claude-cli", false); got != nil {
		t.Errorf("nil expected for empty messages, got %+v", got)
	}
}

// TestBypass_CCFilterNamingOff — AC-005: ccFilterNaming=false
// suppresses the naming bypass.
func TestBypass_CCFilterNamingOff(t *testing.T) {
	body := map[string]any{"messages": msgs(msg("system", "isNewTopic"), msg("user", "hi"))}
	if got := CheckBypass(body, "claude-cli", false); got != nil {
		t.Errorf("expected no naming bypass when ccFilterNaming=false, got %+v", got)
	}
}

// TestBypass_TitleCaseAssistant — title extraction is
// case-insensitive on role.
func TestBypass_TitleCaseAssistant(t *testing.T) {
	body := map[string]any{"messages": msgs(msg("user", "hi"), msg("ASSISTANT", "{\"x\":1}"))}
	got := CheckBypass(body, "claude-cli/1.0", false)
	if got == nil || !got.Bypass {
		t.Fatal("expected bypass for title pattern")
	}
	if got.Response == nil || got.Response.Model != "bypass/title" {
		t.Errorf("title pattern not matched, got %+v", got)
	}
}

// TestBypass_NoMatch — no pattern matches when user-agent is
// claude-cli but messages don't match any rule.
func TestBypass_NoMatch(t *testing.T) {
	body := map[string]any{"messages": msgs(msg("user", "hello world"))}
	if got := CheckBypass(body, "claude-cli/1.0", false); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}
