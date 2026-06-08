package chatcore

import (
	"sync"
	"sync/atomic"
)

// ProviderStats holds the per-provider/model counters surfaced on
// the real-time dashboard.
type ProviderStats struct {
	Pending int64 // currently in-flight requests
	Total   int64 // total requests seen (started)
}

// UsageTracker is a thread-safe in-memory store of ProviderStats
// keyed by "provider:model".
//
// All increments/decrements are atomic; the underlying ProviderStats
// value is returned by value so callers cannot race the tracker.
type UsageTracker struct {
	pending sync.Map // map[string]*int64
	total   sync.Map // map[string]*int64
}

// NewUsageTracker returns an empty tracker.
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{}
}

// Track increments or decrements the per-key counters.
//
//   - model, provider: identify the request.
//   - connectionID: kept for future audit logging; the current
//     implementation only uses it as a sanity argument to make the
//     call-site obvious.
//   - increment: true = request started, false = request completed.
//
// Pending never goes below zero (defensive floor). Total is
// monotonically increasing.
func (t *UsageTracker) Track(model, provider, connectionID string, increment bool) {
	if t == nil {
		return
	}
	_ = connectionID // reserved for future audit
	key := provider + ":" + model

	if increment {
		pending := t.getOrCreate(&t.pending, key)
		atomic.AddInt64(pending, 1)
		total := t.getOrCreate(&t.total, key)
		atomic.AddInt64(total, 1)
		return
	}
	pending := t.getOrCreate(&t.pending, key)
	// Floor at zero — over-decrements (e.g. double complete) are
	// silently clamped.
	for {
		cur := atomic.LoadInt64(pending)
		if cur <= 0 {
			atomic.StoreInt64(pending, 0)
			return
		}
		if atomic.CompareAndSwapInt64(pending, cur, cur-1) {
			return
		}
	}
}

func (t *UsageTracker) getOrCreate(m *sync.Map, key string) *int64 {
	if v, ok := m.Load(key); ok {
		return v.(*int64)
	}
	nv := new(int64)
	actual, _ := m.LoadOrStore(key, nv)
	return actual.(*int64)
}

// Stats returns a snapshot for the supplied key. The bool is false
// when no requests have been tracked yet.
func (t *UsageTracker) Stats(model, provider string) (ProviderStats, bool) {
	if t == nil {
		return ProviderStats{}, false
	}
	key := provider + ":" + model
	var out ProviderStats
	seen := false
	if v, ok := t.pending.Load(key); ok {
		out.Pending = atomic.LoadInt64(v.(*int64))
		seen = true
	}
	if v, ok := t.total.Load(key); ok {
		out.Total = atomic.LoadInt64(v.(*int64))
		seen = true
	}
	return out, seen
}

// Snapshot returns a copy of every tracked key. Intended for the
// dashboard endpoint.
func (t *UsageTracker) Snapshot() map[string]ProviderStats {
	if t == nil {
		return nil
	}
	out := make(map[string]ProviderStats)
	t.pending.Range(func(k, v any) bool {
		key := k.(string)
		pending := atomic.LoadInt64(v.(*int64))
		stat := ProviderStats{Pending: pending}
		if tv, ok := t.total.Load(key); ok {
			stat.Total = atomic.LoadInt64(tv.(*int64))
		}
		out[key] = stat
		return true
	})
	return out
}
