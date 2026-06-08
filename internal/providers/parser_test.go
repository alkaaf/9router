package providers

import "testing"

func TestParseModelString_SlashFormat(t *testing.T) {
	info, err := ParseModelString("anthropic/claude-sonnet-4-20250514", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "anthropic" {
		t.Errorf("provider = %q, want %q", info.Provider, "anthropic")
	}
	if info.Model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q, want %q", info.Model, "claude-sonnet-4-20250514")
	}
}

func TestParseModelString_ColonFormat(t *testing.T) {
	info, err := ParseModelString("openai:gpt-4o-mini", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "openai" {
		t.Errorf("provider = %q, want %q", info.Provider, "openai")
	}
	if info.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want %q", info.Model, "gpt-4o-mini")
	}
}

func TestParseModelString_BareWithDefault(t *testing.T) {
	info, err := ParseModelString("claude-3-haiku-20240307", "anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "anthropic" {
		t.Errorf("provider = %q, want %q", info.Provider, "anthropic")
	}
	if info.Model != "claude-3-haiku-20240307" {
		t.Errorf("model = %q, want %q", info.Model, "claude-3-haiku-20240307")
	}
	if !info.IsAlias {
		t.Error("IsAlias should be true for bare model")
	}
}

func TestParseModelString_BareWithAliasDefault(t *testing.T) {
	info, err := ParseModelString("claude-3-haiku-20240307", "cc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "claude" {
		t.Errorf("provider = %q, want %q (alias resolved)", info.Provider, "claude")
	}
}

func TestParseModelString_BareWithNoDefault(t *testing.T) {
	_, err := ParseModelString("some-model", "")
	if err == nil {
		t.Fatal("expected error for bare model with no default")
	}
}

func TestParseModelString_UnknownProvider(t *testing.T) {
	_, err := ParseModelString("unknown/model", "")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestParseModelString_Empty(t *testing.T) {
	_, err := ParseModelString("", "anthropic")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseModelString_CompatiblePassthrough(t *testing.T) {
	info, err := ParseModelString("openai-compatible-node-abc/model-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "openai-compatible-node-abc" {
		t.Errorf("provider = %q, want %q", info.Provider, "openai-compatible-node-abc")
	}
	if info.Model != "model-1" {
		t.Errorf("model = %q, want %q", info.Model, "model-1")
	}
}

func TestResolveProviderID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"ds", "deepseek"},
		{"anthropic", "anthropic"},
		{"openai-compatible-abc", "openai-compatible-abc"},
		{"custom-embedding-xyz", "custom-embedding-xyz"},
		{"unknown-foo", "unknown-foo"},
		{"", ""},
	}
	for _, tc := range tests {
		got := ResolveProviderID(tc.in)
		if got != tc.want {
			t.Errorf("ResolveProviderID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsKnownProvider(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"anthropic", true},
		{"openai", true},
		{"ds", true},
		{"openai-compatible-abc", true},
		{"custom-embedding-xyz", true},
		{"unknown-foo", false},
		{"", false},
	}
	for _, tc := range tests {
		got := IsKnownProvider(tc.in)
		if got != tc.want {
			t.Errorf("IsKnownProvider(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestIsCompatible(t *testing.T) {
	if !IsCompatible("openai-compatible-abc") {
		t.Error("expected true for openai-compatible-abc")
	}
	if !IsCompatible("anthropic-compatible-foo") {
		t.Error("expected true for anthropic-compatible-foo")
	}
	if !IsCompatible("custom-embedding-xyz") {
		t.Error("expected true for custom-embedding-xyz")
	}
	if IsCompatible("anthropic") {
		t.Error("expected false for anthropic")
	}
}
