package executor

import (
	"testing"
)

// ────────────────────────────────────────────────────────────────────
// DefaultExecutor
// ────────────────────────────────────────────────────────────────────

func TestNewDefaultExecutor_Defaults(t *testing.T) {
	d := NewDefaultExecutor("openai")
	if d.GetProvider() != "openai" {
		t.Errorf("GetProvider = %q, want openai", d.GetProvider())
	}
	if got := d.Config().FallbackCount(); got != 1 {
		t.Errorf("FallbackCount = %d, want 1", got)
	}
}

func TestDefaultExecutor_BuildUrl(t *testing.T) {
	d := NewDefaultExecutor("github")
	want := "/v1/chat/completions"
	if got := d.BuildUrl("gpt-4", false, 0); got != want {
		t.Errorf("BuildUrl = %q, want %q", got, want)
	}
}

// ────────────────────────────────────────────────────────────────────
// Registry
// ────────────────────────────────────────────────────────────────────

func TestRegistry_Register_And_GetExecutor(t *testing.T) {
	r := NewRegistry()

	openAI := NewDefaultExecutor("openai")
	r.Register("openai", func() Executor { return openAI })

	got, ok := r.GetExecutor("openai")
	if !ok {
		t.Errorf("ok = false for registered provider")
	}
	if got != openAI {
		t.Errorf("GetExecutor should return the registered instance")
	}
}

func TestRegistry_GetExecutor_Unknown(t *testing.T) {
	r := NewRegistry()
	got, ok := r.GetExecutor("unknown")
	if ok {
		t.Errorf("unknown provider should return ok=false")
	}
	if got == nil {
		t.Errorf("unknown provider should return DefaultExecutor, not nil")
	}
}

func TestRegistry_HasSpecializedExecutor(t *testing.T) {
	r := NewRegistry()
	r.Register("vertex", func() Executor { return NewDefaultExecutor("vertex") })

	if !r.HasSpecializedExecutor("vertex") {
		t.Errorf("registered provider should be reported as specialized")
	}
	if r.HasSpecializedExecutor("foobar") {
		t.Errorf("unregistered provider should not be reported as specialized")
	}
}

func TestRegistry_Singleton(t *testing.T) {
	r := NewRegistry()
	r.Register("openai", func() Executor { return NewDefaultExecutor("openai") })

	// Same pointer, multiple calls.
	a, _ := r.GetExecutor("openai")
	b, _ := r.GetExecutor("openai")
	if a != b {
		t.Errorf("GetExecutor should return the same instance on repeated calls")
	}
}

// A counter we advance via a closure to verify factory fires once.
func TestRegistry_FactoryFiresOnce(t *testing.T) {
	r := NewRegistry()
	count := 0
	r.Register("once", func() Executor {
		count++
		return NewDefaultExecutor("once")
	})

	for i := 0; i < 10; i++ {
		r.GetExecutor("once")
	}
	if count != 1 {
		t.Errorf("factory called %d times, want 1", count)
	}
}

// ────────────────────────────────────────────────────────────────────
// Default (package-level)
// ────────────────────────────────────────────────────────────────────

func TestDefaultRegistry_GetExecutor_Fallback(t *testing.T) {
	// Default may already have providers registered by other init() blocks
	// in this package; skip this test if "openai" is already there.
	if HasSpecializedExecutor("totally-unknown-provider-9e1b2") {
		return
	}
	got := GetExecutor("totally-unknown-provider-9e1b2")
	if got == nil {
		t.Errorf("GetExecutor for unknown provider should not be nil")
	}
	if got.GetProvider() != "totally-unknown-provider-9e1b2" {
		t.Errorf("DefaultExecutor should use the requested provider name as its identity")
	}
}

// ────────────────────────────────────────────────────────────────────
// Reset / Unregister
// ────────────────────────────────────────────────────────────────────

func TestRegistry_Reset(t *testing.T) {
	r := NewRegistry()
	r.Register("x", func() Executor { return NewDefaultExecutor("x") })
	if !r.HasSpecializedExecutor("x") {
		t.Errorf("precondition: x should be registered")
	}
	r.Reset()
	if r.HasSpecializedExecutor("x") {
		t.Errorf("after Reset, x should not be registered")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	r.Register("y", func() Executor { return NewDefaultExecutor("y") })
	r.Unregister("y")
	if r.HasSpecializedExecutor("y") {
		t.Errorf("after Unregister, y should not be registered")
	}
}

// ────────────────────────────────────────────────────────────────────
// Providers list
// ────────────────────────────────────────────────────────────────────

func TestRegistry_Providers(t *testing.T) {
	r := NewRegistry()
	r.Register("alpha", func() Executor { return NewDefaultExecutor("alpha") })
	r.Register("beta", func() Executor { return NewDefaultExecutor("beta") })

	list := r.Providers()
	if len(list) != 2 {
		t.Errorf("Providers() len = %d, want 2", len(list))
	}
	// Order unspecified, but both must be present.
	got := map[string]bool{}
	for _, p := range list {
		got[p] = true
	}
	if !got["alpha"] || !got["beta"] {
		t.Errorf("Providers() missing alpha/beta, got %v", list)
	}
}

// ────────────────────────────────────────────────────────────────────
// Concurrency
// ────────────────────────────────────────────────────────────────────

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	r.Register("concurrent", func() Executor { return NewDefaultExecutor("concurrent") })

	const goroutines = 100
	done := make(chan Executor, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			exec, _ := r.GetExecutor("concurrent")
			done <- exec
		}()
	}

	var first Executor
	for i := 0; i < goroutines; i++ {
		exec := <-done
		if i == 0 {
			first = exec
		} else if exec != first {
			t.Errorf("goroutine %d got a different executor pointer", i)
		}
	}
}

func TestRegistry_ConcurrentRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	// Pre-register distinct names so concurrent reads don't collide
	// with the "duplicate register" panic.
	r.Register("p-even", func() Executor { return NewDefaultExecutor("p-even") })
	r.Register("p-odd", func() Executor { return NewDefaultExecutor("p-odd") })

	const goroutines = 50
	done := make(chan struct{}, goroutines*2)
	for i := 0; i < goroutines; i++ {
		go func() {
			_, _ = r.GetExecutor("p-even")
			done <- struct{}{}
		}()
		go func() {
			_, _ = r.GetExecutor("p-odd")
			done <- struct{}{}
		}()
	}
	for i := 0; i < goroutines*2; i++ {
		<-done
	}
}

// ────────────────────────────────────────────────────────────────────
// Panics on contract violations
// ────────────────────────────────────────────────────────────────────

func TestRegistry_Register_PanicsOnEmptyProvider(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if rec := recover(); rec == nil {
			t.Errorf("expected panic for empty provider name")
		}
	}()
	r.Register("", func() Executor { return NewDefaultExecutor("") })
}

func TestRegistry_Register_PanicsOnNilFactory(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if rec := recover(); rec == nil {
			t.Errorf("expected panic for nil factory")
		}
	}()
	r.Register("p", nil)
}

func TestRegistry_Register_PanicsOnDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register("p", func() Executor { return NewDefaultExecutor("p") })
	defer func() {
		if rec := recover(); rec == nil {
			t.Errorf("expected panic on duplicate register")
		}
	}()
	r.Register("p", func() Executor { return NewDefaultExecutor("p") })
}
