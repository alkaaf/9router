// Package combo implements the strategy pattern for routing chat
// requests through a named list of models. The strategies are pure
// (no DB, no I/O) and live in process memory.
package combo

import (
	"errors"
	"strings"
)

// Kind enumerates the supported combo strategies. Strings match the
// open-sse/services/combo.js vocabulary.
const (
	KindFallback    = "fallback"
	KindRoundRobin  = "round-robin"
	KindSticky      = "sticky"
)

// Selector returns the model order for the next request against a
// given combo. Implementations are stateful (round-robin, sticky)
// and must be safe for concurrent use.
type Selector interface {
	// NextOrder returns the models in the order they should be tried.
	NextOrder(comboName string, models []string) []string
	// Reset clears any per-combo state for comboName.
	Reset(comboName string)
	// ResetAll clears all state.
	ResetAll()
}

// NewSelector returns a Selector for the given kind. stickyLimit <= 0
// is normalised to 1, matching normalizeStickyLimit in combo.js.
func NewSelector(kind string, stickyLimit int) (Selector, error) {
	if stickyLimit < 1 {
		stickyLimit = 1
	}
	switch strings.ToLower(kind) {
	case "", KindFallback:
		return &FallbackSelector{}, nil
	case KindRoundRobin:
		return NewRoundRobinSelector(stickyLimit), nil
	case KindSticky:
		return NewStickySelector(stickyLimit), nil
	}
	return nil, errors.New("unknown combo kind: " + kind)
}
