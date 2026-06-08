package chatcore

import "sync"

// RotationState is the per-combo in-memory counter tracking the
// sticky consecutive-use count and the next starting index.
type RotationState struct {
	Index int
	Count int
}

// ComboRotator is a thread-safe per-combo rotation store. It is
// keyed by combo name and stores a *RotationState per combo.
//
// Use the package-level NewComboRotator function to construct one;
// tests can instantiate the type directly to bypass the package
// singleton.
type ComboRotator struct {
	mu    sync.Mutex
	store map[string]*RotationState
}

// NewComboRotator returns an empty rotator.
func NewComboRotator() *ComboRotator {
	return &ComboRotator{store: make(map[string]*RotationState)}
}

// Next computes the next starting index and updates the state for
// the supplied combo. stickyLimit is the number of consecutive uses
// before rotating; a value <= 0 rotates on every call (limit=1).
//
// models is rotated such that the returned slice starts at the
// computed index. Empty input is returned unchanged.
func (r *ComboRotator) Next(comboName string, models []string, stickyLimit int) []string {
	if r == nil || len(models) == 0 {
		return models
	}
	if stickyLimit < 1 {
		stickyLimit = 1
	}

	r.mu.Lock()
	state, ok := r.store[comboName]
	if !ok {
		state = &RotationState{Index: 0, Count: 0}
		r.store[comboName] = state
	}

	if state.Count >= stickyLimit {
		state.Index = (state.Index + 1) % len(models)
		state.Count = 0
	}
	state.Count++

	idx := state.Index
	r.mu.Unlock()

	if idx == 0 {
		// No rotation needed — return a copy to keep callers from
		// mutating the input slice.
		out := make([]string, len(models))
		copy(out, models)
		return out
	}
	out := make([]string, len(models))
	copy(out, models[idx:])
	copy(out[len(models)-idx:], models[:idx])
	return out
}

// Reset clears the rotation state for the supplied combo (and only
// that combo). Other combos are unaffected.
func (r *ComboRotator) Reset(comboName string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.store, comboName)
	r.mu.Unlock()
}

// ResetAll clears every combo's state. Used by tests and the admin
// "force refresh" handler.
func (r *ComboRotator) ResetAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.store = make(map[string]*RotationState)
	r.mu.Unlock()
}

// State returns a snapshot of the current rotation state for a
// combo, or (nil, false) when no state has been recorded yet.
func (r *ComboRotator) State(comboName string) (*RotationState, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.store[comboName]
	if !ok {
		return nil, false
	}
	cp := *s
	return &cp, true
}
