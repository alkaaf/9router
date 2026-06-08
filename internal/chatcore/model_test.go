package chatcore

import "testing"

// TestParseModel_ProviderModel — AC-002: explicit "openai/gpt-4" form.
func TestParseModel_ProviderModel(t *testing.T) {
	got := parseModel("openai/gpt-4")
	if got.Provider != "openai" {
		t.Errorf("provider = %q, want %q", got.Provider, "openai")
	}
	if got.Model != "gpt-4" {
		t.Errorf("model = %q, want %q", got.Model, "gpt-4")
	}
	if got.IsAlias {
		t.Errorf("isAlias = true, want false")
	}
	if got.ProviderAlias != "openai" {
		t.Errorf("providerAlias = %q, want %q", got.ProviderAlias, "openai")
	}
}

// TestParseModel_ProviderAlias — "cc/claude-3" expands to "claude"
// (the alias map's canonical target). The Claude model itself is then
// routed by a downstream provider-strategy lookup.
func TestParseModel_ProviderAlias(t *testing.T) {
	got := parseModel("cc/claude-3")
	if got.Provider != "claude" {
		t.Errorf("provider = %q, want %q", got.Provider, "claude")
	}
	if got.Model != "claude-3" {
		t.Errorf("model = %q, want %q", got.Model, "claude-3")
	}
	if got.ProviderAlias != "cc" {
		t.Errorf("providerAlias = %q, want %q", got.ProviderAlias, "cc")
	}
}

// TestParseModel_Alias — AC-001: "gpt-4" alone is treated as an alias.
func TestParseModel_Alias(t *testing.T) {
	got := parseModel("gpt-4")
	if got.IsAlias != true {
		t.Errorf("isAlias = false, want true")
	}
	if got.Model != "gpt-4" {
		t.Errorf("model = %q, want %q", got.Model, "gpt-4")
	}
	if got.Provider != "" {
		t.Errorf("provider = %q, want empty (no resolution yet)", got.Provider)
	}
}

// TestParseModel_Empty — edge case: empty input.
func TestParseModel_Empty(t *testing.T) {
	got := parseModel("")
	if got.Provider != "" || got.Model != "" || got.IsAlias {
		t.Errorf("empty input should give zero ModelInfo, got %+v", got)
	}
}

// TestParseModel_MultipleSlashes — only the FIRST slash is the
// provider separator. "a/b/c" → provider="a", model="b/c".
func TestParseModel_MultipleSlashes(t *testing.T) {
	got := parseModel("openrouter/owner/repo")
	if got.Provider != "openrouter" {
		t.Errorf("provider = %q, want %q", got.Provider, "openrouter")
	}
	if got.Model != "owner/repo" {
		t.Errorf("model = %q, want %q", got.Model, "owner/repo")
	}
}

// TestParseModel_TrailingSlash — "openai/" → provider=openai, model="".
// We mirror the Node.js behaviour (no error) and let downstream code
// reject an empty model.
func TestParseModel_TrailingSlash(t *testing.T) {
	got := parseModel("openai/")
	if got.Provider != "openai" {
		t.Errorf("provider = %q, want %q", got.Provider, "openai")
	}
	if got.Model != "" {
		t.Errorf("model = %q, want empty", got.Model)
	}
	if got.IsAlias {
		t.Errorf("isAlias = true, want false")
	}
}

// TestResolveProviderAlias_Known — known alias resolves.
func TestResolveProviderAlias_Known(t *testing.T) {
	if got := resolveProviderAlias("cc"); got != "claude" {
		t.Errorf("cc = %q, want %q", got, "claude")
	}
	if got := resolveProviderAlias("openai"); got != "openai" {
		t.Errorf("openai = %q, want %q", got, "openai")
	}
}

// TestResolveProviderAlias_Unknown — unknown input passes through.
func TestResolveProviderAlias_Unknown(t *testing.T) {
	if got := resolveProviderAlias("unknown-provider"); got != "unknown-provider" {
		t.Errorf("unknown = %q, want pass-through", got)
	}
}

// TestResolveModelAlias_Present — the alias map is consulted.
func TestResolveModelAlias_Present(t *testing.T) {
	aliases := map[string]string{
		"my-gpt4": "openai/gpt-4",
		"my-cc":   "cc/claude-3",
	}
	if p, m, ok := resolveModelAlias("my-gpt4", aliases); !ok || p != "openai" || m != "gpt-4" {
		t.Errorf("my-gpt4 = (%q, %q, %v), want (openai, gpt-4, true)", p, m, ok)
	}
	if p, m, ok := resolveModelAlias("my-cc", aliases); !ok || p != "claude" || m != "claude-3" {
		t.Errorf("my-cc = (%q, %q, %v), want (claude, claude-3, true)", p, m, ok)
	}
}

// TestResolveModelAlias_Absent — unknown alias returns ok=false.
func TestResolveModelAlias_Absent(t *testing.T) {
	if _, _, ok := resolveModelAlias("nope", map[string]string{}); ok {
		t.Error("expected ok=false for missing alias")
	}
}

// TestResolveModelAlias_NilMap — nil map does not panic.
func TestResolveModelAlias_NilMap(t *testing.T) {
	if _, _, ok := resolveModelAlias("anything", nil); ok {
		t.Error("expected ok=false with nil map")
	}
}

// TestInferProviderFromModelName — table-driven for the heuristic
// fallbacks. The function returns "openai" for anything it does not
// recognise, matching the Node.js default branch.
func TestInferProviderFromModelName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"claude-3-5-sonnet-20241022", "anthropic"},
		{"gemini-1.5-pro", "gemini"},
		{"gpt-4o", "openai"},
		{"o1-preview", "openai"},
		{"o3-mini", "openai"},
		{"o4-turbo", "openai"},
		{"deepseek-coder", "openrouter"},
		{"unknown-model", "openai"}, // default fallback
		{"", "openai"},              // empty input → default
	}
	for _, c := range cases {
		if got := inferProviderFromModelName(c.in); got != c.want {
			t.Errorf("infer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestResolveModel_Explicit — explicit "openai/gpt-4" bypasses aliases.
func TestResolveModel_Explicit(t *testing.T) {
	aliases := map[string]string{"gpt-4": "anthropic/gpt-4"} // wrong on purpose
	got := ResolveModel("openai/gpt-4", aliases)
	if got.Provider != "openai" || got.Model != "gpt-4" || got.IsAlias {
		t.Errorf("explicit form should bypass alias map, got %+v", got)
	}
}

// TestResolveModel_AliasHit — alias form that is in the map.
func TestResolveModel_AliasHit(t *testing.T) {
	aliases := map[string]string{"my-best": "anthropic/claude-3-5-sonnet"}
	got := ResolveModel("my-best", aliases)
	if got.Provider != "anthropic" || got.Model != "claude-3-5-sonnet" || !got.IsAlias {
		t.Errorf("alias hit: got %+v, want provider=anthropic, model=claude-3-5-sonnet, isAlias=true", got)
	}
}

// TestResolveModel_AliasMiss — alias form not in the map falls through
// to inference. "gpt-4-turbo" should infer to openai.
func TestResolveModel_AliasMiss(t *testing.T) {
	got := ResolveModel("gpt-4-turbo", nil)
	if got.Provider != "openai" || got.Model != "gpt-4-turbo" || !got.IsAlias {
		t.Errorf("alias miss: got %+v, want provider=openai, model=gpt-4-turbo, isAlias=true", got)
	}
}

// TestResolveModel_AliasMiss_UnknownModel — unknown alias form falls
// through to the default fallback ("openai") since the heuristic
// does not match. Mirrors the JS default branch.
func TestResolveModel_AliasMiss_UnknownModel(t *testing.T) {
	got := ResolveModel("mystery-7", nil)
	if got.Provider != "openai" {
		t.Errorf("unknown alias form: provider = %q, want openai (default)", got.Provider)
	}
}
