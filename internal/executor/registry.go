package executor

import (
	"sync"
)

// DefaultExecutor is the OpenAI-compatible fallback returned by
// getExecutor for any provider name the registry does not have a
// specialized executor for. The wire format, URL paths, and auth header
// all match OpenAI's public API — the most common denominator across
// "compatible" providers in the open-sse registry.
type DefaultExecutor struct {
	*BaseExecutor
}

// NewDefaultExecutor builds a DefaultExecutor for the given provider
// name. The provider name appears in logs and is also used as the
// Bearer-token scope (callers can override AuthHeader by passing a
// non-default ProviderConfig in the future, but the current registry
// always uses the same OpenAI-shaped config).
func NewDefaultExecutor(provider string) *DefaultExecutor {
	cfg := &ProviderConfig{
		Provider:      provider,
		BaseURLs:      []string{""},
		AuthHeader:    "Authorization",
		StreamPath:    "/v1/chat/completions",
		NonStreamPath: "/v1/chat/completions",
	}
	return &DefaultExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// GetProvider returns the underlying provider name.
func (d *DefaultExecutor) GetProvider() string {
	if d == nil || d.BaseExecutor == nil {
		return ""
	}
	return d.BaseExecutor.GetProvider()
}

// Registry holds the provider → executor mapping. Concrete executors
// register themselves on package init; callers retrieve the right
// executor via GetExecutor. The registry is safe for concurrent use.
//
// Two layers of locking:
//
//   - mu (RWMutex) protects the entries map. Reads are common (every
//     chat request calls GetExecutor), writes are rare (only on
//     RegisterExecutor).
//   - per-entry sync.Once guarantees each provider's executor is
//     constructed exactly once even under concurrent first access.
//
// The split is deliberate: the map is the volatile part, the per-entry
// constructor is the expensive part. Taking the read-lock for every
// GetExecutor call would be wasteful.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*registryEntry
	factories map[string]ExecutorFactory

	// defaultProvider is the name attached to DefaultExecutor instances
	// created for unknown providers. It is set once at construction
	// and never changes.
	defaultProvider string
}

// registryEntry pairs a constructor with a once-only memoization slot.
type registryEntry struct {
	once     sync.Once
	executor Executor
	factory  ExecutorFactory
	err      error
}

// ExecutorFactory builds an Executor. It is the constructor registered
// for a provider name; the registry invokes it lazily on first
// GetExecutor call.
type ExecutorFactory func() Executor

// NewRegistry returns an empty registry. Callers (typically package
// init() in each executor file) populate it via Register before any
// GetExecutor calls.
func NewRegistry() *Registry {
	return &Registry{
		entries:         make(map[string]*registryEntry),
		factories:       make(map[string]ExecutorFactory),
		defaultProvider: "default",
	}
}

// Default returns the package-level registry. The variable is
// initialized once via package init ordering — concrete executors that
// need to be discoverable at runtime should call Register on Default
// from their own init() blocks.
var Default = NewRegistry()

// Register binds a provider name to its executor factory. Subsequent
// GetExecutor calls for the same name will return the singleton
// constructed from this factory. Registering the same name twice is
// a programming error and panics — the registry is meant to be wired
// up at package init, not mutated at runtime.
//
// The factory is invoked at most once, even under concurrent first
// access. If the factory returns nil, that nil is cached and every
// subsequent call yields a nil Executor (callers should treat that as
// "no executor available, use DefaultExecutor via GetExecutor after
// registering a working factory").
func (r *Registry) Register(provider string, factory ExecutorFactory) {
	if provider == "" {
		panic("executor: Register called with empty provider name")
	}
	if factory == nil {
		panic("executor: Register called with nil factory for " + provider)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[provider]; exists {
		panic("executor: provider " + provider + " already registered")
	}
	r.entries[provider] = &registryEntry{factory: factory}
	r.factories[provider] = factory
}

// GetExecutor returns the singleton executor for the given provider
// name. If the provider has no registered factory, a fresh
// DefaultExecutor scoped to the provider name is returned. The
// returned Executor is shared between all callers — concrete
// executors must be safe for concurrent use.
//
// The second return value reports whether a specialized executor was
// used (true) or the OpenAI-compatible default (false).
func (r *Registry) GetExecutor(provider string) (Executor, bool) {
	r.mu.RLock()
	entry, ok := r.entries[provider]
	r.mu.RUnlock()
	if !ok {
		return NewDefaultExecutor(provider), false
	}
	entry.once.Do(func() {
		entry.executor = entry.factory()
	})
	return entry.executor, true
}

// HasSpecializedExecutor reports whether the given provider has a
// registered factory. It does not invoke the factory; this is the
// cheap query used by the chat handler's combo-routing logic to
// decide whether to fall back to a generic OpenAI-shaped request.
func (r *Registry) HasSpecializedExecutor(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.entries[provider]
	return ok
}

// Providers returns a snapshot of the registered provider names. The
// slice is freshly allocated and safe to mutate. The order is
// unspecified.
func (r *Registry) Providers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.entries))
	for k := range r.entries {
		out = append(out, k)
	}
	return out
}

// Unregister removes a provider from the registry. Intended for tests
// that need to verify the "unknown provider → default" branch. It is
// a programming error to unregister a provider whose executor is in
// active use — concurrent callers may already hold a reference to
// the singleton and the sync.Once prevents re-construction.
func (r *Registry) Unregister(provider string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, provider)
	delete(r.factories, provider)
}

// Reset clears every entry. The package-level Default registry
// intentionally does NOT expose Reset; tests should use their own
// Registry instance via NewRegistry to avoid global state collisions.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make(map[string]*registryEntry)
	r.factories = make(map[string]ExecutorFactory)
}

// ────────────────────────────────────────────────────────────────────
// Package-level convenience functions. They delegate to Default so
// existing executors (and the chat handler) can use a 1-line API.
// ────────────────────────────────────────────────────────────────────

// GetExecutor returns the executor for the given provider, or an
// OpenAI-compatible default when the provider is unknown.
func GetExecutor(provider string) Executor {
	exec, _ := Default.GetExecutor(provider)
	return exec
}

// HasSpecializedExecutor reports whether the provider has a registered
// specialized executor.
func HasSpecializedExecutor(provider string) bool {
	return Default.HasSpecializedExecutor(provider)
}

// Register binds a provider name to a factory on the package-level
// Default registry. Convenience wrapper for the common case.
func Register(provider string, factory ExecutorFactory) {
	Default.Register(provider, factory)
}
