package chatcore

import (
	"reflect"
	"sync"
	"testing"
)

// TestRoundRobin_Limit1AlwaysRotates — AC-001: stickyLimit=1 means
// every call advances the starting index.
func TestRoundRobin_Limit1AlwaysRotates(t *testing.T) {
	r := NewComboRotator()
	models := []string{"a", "b", "c"}
	calls := [][]string{
		r.Next("c1", models, 1),
		r.Next("c1", models, 1),
		r.Next("c1", models, 1),
	}
	want := [][]string{
		{"a", "b", "c"},
		{"b", "c", "a"},
		{"c", "a", "b"},
	}
	for i, w := range want {
		if !reflect.DeepEqual(calls[i], w) {
			t.Errorf("call %d: got %v, want %v", i, calls[i], w)
		}
	}
}

// TestRoundRobin_Limit3StaysThenRotates — AC-002: stickyLimit=3
// keeps the starting index for 3 calls, then rotates.
func TestRoundRobin_Limit3StaysThenRotates(t *testing.T) {
	r := NewComboRotator()
	models := []string{"a", "b", "c"}
	got := []string{}
	for i := 0; i < 6; i++ {
		out := r.Next("c1", models, 3)
		got = append(got, out[0])
	}
	// 3 calls at "a", then 3 calls at "b".
	want := []string{"a", "a", "a", "b", "b", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("starting indices = %v, want %v", got, want)
	}
}

// TestRoundRobin_MultipleCombosIndependent — AC-003: state is
// tracked separately per combo name.
func TestRoundRobin_MultipleCombosIndependent(t *testing.T) {
	r := NewComboRotator()
	models := []string{"a", "b", "c"}
	_ = r.Next("c1", models, 1)
	_ = r.Next("c1", models, 1)
	_ = r.Next("c2", models, 1)
	// c1: rotated twice → starts at "c". c2: rotated once → "b".
	if r.Next("c1", models, 1)[0] != "c" {
		t.Errorf("c1 should be at index 2 after 2 rotations")
	}
	if r.Next("c2", models, 1)[0] != "b" {
		t.Errorf("c2 should be at index 1 after 1 rotation")
	}
}

// TestRoundRobin_ResetClears — AC-004: Reset clears a single
// combo's state; others are untouched.
func TestRoundRobin_ResetClears(t *testing.T) {
	r := NewComboRotator()
	models := []string{"a", "b", "c"}
	_ = r.Next("c1", models, 1)
	_ = r.Next("c1", models, 1)
	_ = r.Next("c2", models, 1)
	r.Reset("c1")
	// c1 should restart at "a"; c2 should still be at "b".
	if r.Next("c1", models, 1)[0] != "a" {
		t.Errorf("after reset, c1 should start at a")
	}
	if r.Next("c2", models, 1)[0] != "b" {
		t.Errorf("c2 should be unaffected by reset on c1")
	}
}

// TestRoundRobin_ThreadSafe — AC-005: concurrent calls are
// race-free (run with -race).
func TestRoundRobin_ThreadSafe(t *testing.T) {
	r := NewComboRotator()
	models := []string{"a", "b", "c", "d"}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = r.Next("concurrent", models, 3)
			}
		}()
	}
	wg.Wait()
	// We don't assert the exact index — just that nothing panicked.
	if _, ok := r.State("concurrent"); !ok {
		t.Error("expected state to exist after concurrent calls")
	}
}

// TestRoundRobin_EmptyModels — AC-006: empty input is returned
// unchanged and does not poison the state.
func TestRoundRobin_EmptyModels(t *testing.T) {
	r := NewComboRotator()
	if got := r.Next("c1", nil, 1); got != nil {
		t.Errorf("nil in → nil out, got %v", got)
	}
	if got := r.Next("c1", []string{}, 1); len(got) != 0 {
		t.Errorf("[] in → [] out, got %v", got)
	}
}

// TestRoundRobin_NilRotator — defensive: a nil rotator returns
// its input unchanged.
func TestRoundRobin_NilRotator(t *testing.T) {
	var r *ComboRotator
	models := []string{"a", "b"}
	got := r.Next("c1", models, 1)
	if !reflect.DeepEqual(got, models) {
		t.Errorf("nil rotator should return input, got %v", got)
	}
}

// TestRoundRobin_ResetAll — sanity check.
func TestRoundRobin_ResetAll(t *testing.T) {
	r := NewComboRotator()
	models := []string{"a", "b"}
	_ = r.Next("c1", models, 1)
	_ = r.Next("c2", models, 1)
	r.ResetAll()
	if _, ok := r.State("c1"); ok {
		t.Error("c1 should be cleared after ResetAll")
	}
	if _, ok := r.State("c2"); ok {
		t.Error("c2 should be cleared after ResetAll")
	}
}
