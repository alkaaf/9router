package combo

import (
	"sync"
)

// RoundRobinSelector rotates the model list per call. With stickyLimit
// N, it serves the same ordering for N consecutive calls before
// advancing. State is keyed by comboName.
type RoundRobinSelector struct {
	mu          sync.Mutex
	stickyLimit int
	state       map[string]*rrState
}

type rrState struct {
	index                int
	consecutiveUseCount  int
}

// NewRoundRobinSelector returns a new RoundRobinSelector. stickyLimit
// is coerced to >= 1.
func NewRoundRobinSelector(stickyLimit int) *RoundRobinSelector {
	if stickyLimit < 1 {
		stickyLimit = 1
	}
	return &RoundRobinSelector{stickyLimit: stickyLimit, state: make(map[string]*rrState)}
}

// NextOrder returns the models rotated by the current index.
func (r *RoundRobinSelector) NextOrder(comboName string, models []string) []string {
	if len(models) <= 1 {
		return append([]string{}, models...)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.state[comboName]
	if !ok {
		st = &rrState{}
		r.state[comboName] = st
	}
	rotated := rotateSlice(models, st.index)
	st.consecutiveUseCount++
	if st.consecutiveUseCount >= r.stickyLimit {
		st.index = (st.index + 1) % len(models)
		st.consecutiveUseCount = 0
	}
	return rotated
}

// Reset clears the state for one combo.
func (r *RoundRobinSelector) Reset(comboName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.state, comboName)
}

// ResetAll clears all state.
func (r *RoundRobinSelector) ResetAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = make(map[string]*rrState)
}

func rotateSlice(s []string, start int) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[(start+i)%len(s)]
	}
	return out
}
