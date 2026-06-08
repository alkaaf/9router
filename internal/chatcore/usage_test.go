package chatcore

import (
	"sync"
	"testing"
)

// TestUsage_StartEnd — AC-001: incrementing pending + total, then
// decrementing pending, balances correctly.
func TestUsage_StartEnd(t *testing.T) {
	tr := NewUsageTracker()
	tr.Track("gpt-4", "openai", "c1", true)
	tr.Track("gpt-4", "openai", "c1", false)
	s, ok := tr.Stats("gpt-4", "openai")
	if !ok {
		t.Fatal("expected stats to exist")
	}
	if s.Pending != 0 {
		t.Errorf("Pending = %d, want 0", s.Pending)
	}
	if s.Total != 1 {
		t.Errorf("Total = %d, want 1", s.Total)
	}
}

// TestUsage_Concurrent — AC-002: 100 concurrent starts produce
// 100 pending; race-free.
func TestUsage_Concurrent(t *testing.T) {
	tr := NewUsageTracker()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Track("gpt-4", "openai", "c1", true)
		}()
	}
	wg.Wait()
	s, _ := tr.Stats("gpt-4", "openai")
	if s.Pending != 100 {
		t.Errorf("Pending = %d, want 100", s.Pending)
	}
	if s.Total != 100 {
		t.Errorf("Total = %d, want 100", s.Total)
	}
}

// TestUsage_OverDecrement — AC-004: pending never goes below zero.
func TestUsage_OverDecrement(t *testing.T) {
	tr := NewUsageTracker()
	tr.Track("m", "p", "c", false) // nothing to decrement
	tr.Track("m", "p", "c", false) // nothing
	s, _ := tr.Stats("m", "p")
	if s.Pending != 0 {
		t.Errorf("Pending = %d, want 0", s.Pending)
	}
	if s.Total != 0 {
		t.Errorf("Total = %d, want 0", s.Total)
	}
}

// TestUsage_MultipleKeys — AC-005: different provider/model pairs
// are tracked separately.
func TestUsage_MultipleKeys(t *testing.T) {
	tr := NewUsageTracker()
	tr.Track("gpt-4", "openai", "c1", true)
	tr.Track("claude-3", "anthropic", "c1", true)
	tr.Track("gpt-4", "openai", "c1", true)

	s1, _ := tr.Stats("gpt-4", "openai")
	if s1.Pending != 2 {
		t.Errorf("openai/gpt-4 pending = %d, want 2", s1.Pending)
	}
	s2, _ := tr.Stats("claude-3", "anthropic")
	if s2.Pending != 1 {
		t.Errorf("anthropic/claude-3 pending = %d, want 1", s2.Pending)
	}
}

// TestUsage_Snapshot — the snapshot method returns all keys.
func TestUsage_Snapshot(t *testing.T) {
	tr := NewUsageTracker()
	tr.Track("a", "p1", "c", true)
	tr.Track("b", "p2", "c", true)
	snap := tr.Snapshot()
	if len(snap) != 2 {
		t.Errorf("snapshot len = %d, want 2", len(snap))
	}
	if snap["p1:a"].Pending != 1 {
		t.Errorf("p1:a pending = %d", snap["p1:a"].Pending)
	}
}

// TestUsage_NilTracker — defensive: nil tracker does not panic.
func TestUsage_NilTracker(t *testing.T) {
	var tr *UsageTracker
	tr.Track("m", "p", "c", true) // should not panic
	if _, ok := tr.Stats("m", "p"); ok {
		t.Errorf("nil tracker should report no stats")
	}
}
