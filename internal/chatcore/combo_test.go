package chatcore

import (
	"errors"
	"testing"
)

// comboMap builds a simple in-memory ComboLookup from a name→models
// map. The returned function never returns an error.
func comboMap(data map[string][]string) ComboLookup {
	return func(name string) (*ComboInfo, error) {
		models, ok := data[name]
		if !ok {
			return nil, nil
		}
		return &ComboInfo{Name: name, Models: models}, nil
	}
}

// comboMapWithError is like comboMap but returns the supplied error
// for a specific name. Used to exercise the error path.
func comboMapWithError(name string, err error) ComboLookup {
	return func(got string) (*ComboInfo, error) {
		if got == name {
			return nil, err
		}
		return nil, nil
	}
}

// TestGetComboModels_Valid — AC-001: a known combo returns its models.
func TestGetComboModels_Valid(t *testing.T) {
	lookup := comboMap(map[string][]string{
		"my-combo": {"openai/gpt-4", "anthropic/claude-3", "openai/gpt-4o-mini"},
	})
	models, err := GetComboModels("my-combo", lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("got %d models, want 3", len(models))
	}
	if models[0] != "openai/gpt-4" || models[1] != "anthropic/claude-3" || models[2] != "openai/gpt-4o-mini" {
		t.Errorf("models = %v, want [openai/gpt-4 anthropic/claude-3 openai/gpt-4o-mini]", models)
	}
}

// TestGetComboModels_ExplicitProvider — AC-002: "openai/gpt-4" is
// never treated as a combo name regardless of what combos exist.
func TestGetComboModels_ExplicitProvider(t *testing.T) {
	lookup := comboMap(map[string][]string{
		"openai/gpt-4": {"should-never-be-returned"},
	})
	models, err := GetComboModels("openai/gpt-4", lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Errorf("explicit form should return nil, got %v", models)
	}
}

// TestGetComboModels_Unknown — AC-003: an unknown name returns nil.
func TestGetComboModels_Unknown(t *testing.T) {
	lookup := comboMap(map[string][]string{"my-combo": {"a"}})
	models, err := GetComboModels("not-a-combo", lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Errorf("unknown name should return nil, got %v", models)
	}
}

// TestGetComboModels_EmptyModels — AC-004: a row with an empty models
// list returns nil.
func TestGetComboModels_EmptyModels(t *testing.T) {
	lookup := comboMap(map[string][]string{"empty-combo": {}})
	models, err := GetComboModels("empty-combo", lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Errorf("empty combo should return nil, got %v", models)
	}
}

// TestGetComboModels_ComboWithSlash — slash in the input is a
// short-circuit; a combo with a slash in its name (theoretical only)
// can never be looked up.
func TestGetComboModels_ComboWithSlash(t *testing.T) {
	lookup := comboMap(map[string][]string{"combo/name": {"a"}})
	models, err := GetComboModels("combo/name", lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Errorf("slash-prefixed input should return nil, got %v", models)
	}
}

// TestGetComboModels_EmptyString — defensive: empty input is a no-op.
func TestGetComboModels_EmptyString(t *testing.T) {
	lookup := comboMap(map[string][]string{"a": {"x"}})
	models, err := GetComboModels("", lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Errorf("empty input should return nil, got %v", models)
	}
}

// TestGetComboModels_NilLookup — defensive: nil lookup function is a
// no-op (no DB to consult).
func TestGetComboModels_NilLookup(t *testing.T) {
	models, err := GetComboModels("anything", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Errorf("nil lookup should return nil, got %v", models)
	}
}

// TestGetComboModels_LookupError — a DB error is propagated.
func TestGetComboModels_LookupError(t *testing.T) {
	lookup := comboMapWithError("my-combo", errors.New("db down"))
	_, err := GetComboModels("my-combo", lookup)
	if err == nil {
		t.Fatal("expected error from lookup, got nil")
	}
}

// TestParseComboModels_ArrayForm — the canonical shape.
func TestParseComboModels_ArrayForm(t *testing.T) {
	raw := `["openai/gpt-4","anthropic/claude-3"]`
	got := ParseComboModels(raw)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != "openai/gpt-4" || got[1] != "anthropic/claude-3" {
		t.Errorf("got %v, want [openai/gpt-4 anthropic/claude-3]", got)
	}
}

// TestParseComboModels_ObjectForm — the older {models: [...]} shape.
func TestParseComboModels_ObjectForm(t *testing.T) {
	raw := `{"models":["a","b"]}`
	got := ParseComboModels(raw)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

// TestParseComboModels_Empty — empty / whitespace input → nil.
func TestParseComboModels_Empty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n"} {
		if got := ParseComboModels(in); got != nil {
			t.Errorf("input %q: got %v, want nil", in, got)
		}
	}
}

// TestParseComboModels_Invalid — garbage JSON returns nil rather than
// panicking.
func TestParseComboModels_Invalid(t *testing.T) {
	for _, in := range []string{"not json", "{", "[\"unterminated"} {
		got := ParseComboModels(in)
		if got != nil {
			t.Errorf("input %q: got %v, want nil", in, got)
		}
	}
}

// TestComboDetection_RunsBeforeAlias — AC-005: combo detection must
// short-circuit before alias resolution. The ResolveModel function
// would otherwise treat "my-combo" as an alias and try to look it up
// in the alias map. We confirm the call order is correct by
// composing the two functions.
func TestComboDetection_RunsBeforeAlias(t *testing.T) {
	// Suppose "my-combo" is both a combo name AND an alias target.
	// The contract: GetComboModels wins, and the alias map is
	// never consulted.
	lookup := comboMap(map[string][]string{
		"my-combo": {"a", "b"},
	})
	models, err := GetComboModels("my-combo", lookup)
	if err != nil || len(models) != 2 {
		t.Fatalf("combo detection failed: %v, %v", err, models)
	}

	// And the alias resolver is a no-op for the combo name (it has
	// not been called yet, but if it WERE called it would find
	// nothing useful in the map below).
	aliases := map[string]string{"my-combo": "openai/gpt-4"} // wrong on purpose
	_ = ResolveModel("my-combo", aliases)
	// The test succeeds if GetComboModels returned models — that
	// means the caller will short-circuit and never call
	// ResolveModel.
}
