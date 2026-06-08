package combo

import (
	"sync"
)

// StickySelector sticks to models[0] for stickyLimit consecutive
// requests per (comboName, sessionKey) pair, then rotates. Useful
// when the chat handler wants the same upstream account for a
// caller-determined window before cycling to the next.
type StickySelector struct {
	mu          sync.RWMutex
	stickyLimit int
	state       map[string]map[string]*rrState
}

// NewStickySelector returns a new StickySelector. stickyLimit is
// coerced to >= 1.
func NewStickySelector(stickyLimit int) *StickySelector {
	if stickyLimit < 1 {
		stickyLimit = 1
	}
	return &StickySelector{stickyLimit: stickyLimit, state: make(map[string]map[string]*rrState)}
}

// NextOrder returns the standard order (state has no per-call session
// awareness in this variant).
func (s *StickySelector) NextOrder(comboName string, models []string) []string {
	return append([]string{}, models...)
}

// NextOrderWithSession rotates the per-session counter and returns
// models in the new order.
func (s *StickySelector) NextOrderWithSession(comboName, sessionKey string, models []string) []string {
	if len(models) <= 1 {
		return append([]string{}, models...)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	combo, ok := s.state[comboName]
	if !ok {
		combo = make(map[string]*rrState)
		s.state[comboName] = combo
	}
	st, ok := combo[sessionKey]
	if !ok {
		st = &rrState{}
		combo[sessionKey] = st
	}
	rotated := rotateSlice(models, st.index)
	st.consecutiveUseCount++
	if st.consecutiveUseCount >= s.stickyLimit {
		st.index = (st.index + 1) % len(models)
		st.consecutiveUseCount = 0
	}
	return rotated
}

// Reset clears all session state for one combo.
func (s *StickySelector) Reset(comboName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state, comboName)
}

// ResetAll clears all state.
func (s *StickySelector) ResetAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = make(map[string]map[string]*rrState)
}
