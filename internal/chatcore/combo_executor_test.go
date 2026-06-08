package chatcore

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// TestComboFallback_FirstSucceeds — AC-001: a successful first
// attempt returns immediately without consulting later models.
func TestComboFallback_FirstSucceeds(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context, model string) (*Response, error) {
		calls++
		return &Response{Status: http.StatusOK, Body: []byte("ok-" + model), Model: model}, nil
	}
	cf := NewComboFallback(exec)
	resp, err := cf.Execute(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
	if resp.Model != "a" {
		t.Errorf("resp.Model = %q, want a", resp.Model)
	}
}

// TestComboFallback_SecondSucceeds — AC-002: first model fails
// fallbackable, second succeeds.
func TestComboFallback_SecondSucceeds(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context, model string) (*Response, error) {
		calls++
		if model == "a" {
			return nil, NewUpstreamError(http.StatusInternalServerError, "boom", nil)
		}
		return &Response{Status: http.StatusOK, Model: model}, nil
	}
	cf := NewComboFallback(exec, WithCooldownOn503(0))
	resp, err := cf.Execute(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
	if resp.Model != "b" {
		t.Errorf("resp.Model = %q, want b", resp.Model)
	}
}

// TestComboFallback_AllFail — AC-003: all models failing returns
// the last error wrapped in a *ComboError with Retryable=true.
func TestComboFallback_AllFail(t *testing.T) {
	exec := func(ctx context.Context, model string) (*Response, error) {
		return nil, NewUpstreamError(http.StatusTooManyRequests, "rate limited", nil)
	}
	cf := NewComboFallback(exec, WithCooldownOn503(0))
	_, err := cf.Execute(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *ComboError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ComboError, got %T", err)
	}
	if !ce.Retryable {
		t.Errorf("Retryable = false, want true")
	}
	if ce.Model != "b" {
		t.Errorf("Model = %q, want b (last attempted)", ce.Model)
	}
	if ce.Attempt != 2 || ce.Total != 2 {
		t.Errorf("Attempt/Total = %d/%d, want 2/2", ce.Attempt, ce.Total)
	}
}

// TestComboFallback_NonFallbackableStops — AC-004: 400/422 errors
// short-circuit and the function returns the error without trying
// later models.
func TestComboFallback_NonFallbackableStops(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context, model string) (*Response, error) {
		calls++
		return nil, NewUpstreamError(http.StatusBadRequest, "bad", nil)
	}
	cf := NewComboFallback(exec, WithCooldownOn503(0))
	_, err := cf.Execute(context.Background(), []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *ComboError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ComboError, got %T", err)
	}
	if ce.Retryable {
		t.Errorf("Retryable = true, want false for 400")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no fallback on 400), got %d", calls)
	}
}

// TestComboFallback_Transient503Waits — AC-005: 503 with a small
// cooldown triggers the wait; the next call only fires after the
// wait elapses.
func TestComboFallback_Transient503Waits(t *testing.T) {
	calls := []time.Time{}
	exec := func(ctx context.Context, model string) (*Response, error) {
		calls = append(calls, time.Now())
		if model == "a" {
			return nil, NewUpstreamError(http.StatusServiceUnavailable, "down", nil)
		}
		return &Response{Status: http.StatusOK, Model: model}, nil
	}
	cf := NewComboFallback(exec, WithCooldownOn503(5000))
	start := time.Now()
	_, err := cf.Execute(context.Background(), []string{"a", "b"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	// 503 cooldown = 30s which is > 5000ms ceiling, so the
	// configured limit should suppress the wait. Verify that.
	if elapsed > 1*time.Second {
		t.Errorf("cooldown wait should have been suppressed (>5s), took %v", elapsed)
	}
}

// TestComboFallback_Transient429Waits — AC-005: a 429 with a
// short cooldown actually waits.
func TestComboFallback_Transient429Waits(t *testing.T) {
	// Force a short cooldown by lowering DefaultBackoff.Base
	// briefly for this test.
	orig := DefaultBackoff
	DefaultBackoff = BackoffConfig{Base: 50 * time.Millisecond, Max: 1 * time.Second, MaxLevel: 3}
	t.Cleanup(func() { DefaultBackoff = orig })

	calls := []time.Time{}
	exec := func(ctx context.Context, model string) (*Response, error) {
		calls = append(calls, time.Now())
		if model == "a" {
			return nil, NewUpstreamError(http.StatusTooManyRequests, "rate", nil)
		}
		return &Response{Status: http.StatusOK, Model: model}, nil
	}
	cf := NewComboFallback(exec, WithCooldownOn503(5000))
	_, err := cf.Execute(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	gap := calls[1].Sub(calls[0])
	if gap < 20*time.Millisecond {
		t.Errorf("expected ~50ms cooldown, got %v", gap)
	}
}

// TestComboFallback_EarliestRetryAfter — AC-006: across all
// attempts, the earliest retry-after is tracked.
func TestComboFallback_EarliestRetryAfter(t *testing.T) {
	// Force a small base so we can assert ordering.
	orig := DefaultBackoff
	DefaultBackoff = BackoffConfig{Base: 100 * time.Millisecond, Max: 1 * time.Second, MaxLevel: 5}
	t.Cleanup(func() { DefaultBackoff = orig })

	exec := func(ctx context.Context, model string) (*Response, error) {
		// First model: level-0 429 → cooldown 100ms.
		// Second model: level-1 429 → cooldown 200ms.
		// Third model: level-2 429 → cooldown 400ms.
		// Earliest is 100ms from the first attempt.
		if model == "a" {
			return nil, NewUpstreamError(http.StatusTooManyRequests, "r", nil)
		}
		return nil, NewUpstreamError(http.StatusTooManyRequests, "r", nil)
	}
	cf := NewComboFallback(exec, WithCooldownOn503(0))
	_, _ = cf.Execute(context.Background(), []string{"a", "b"})
	got := cf.EarliestRetryAfter()
	if got.IsZero() {
		t.Fatal("expected non-zero EarliestRetryAfter, got zero")
	}
	delta := time.Until(got)
	if delta <= 0 || delta > 150*time.Millisecond {
		t.Errorf("EarliestRetryAfter delta = %v, want ~100ms", delta)
	}
}

// TestComboFallback_ContextCancelled — when the caller cancels the
// context the executor stops immediately and returns ctx.Err().
func TestComboFallback_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exec := func(ctx context.Context, model string) (*Response, error) {
		return &Response{Status: http.StatusOK}, nil
	}
	cf := NewComboFallback(exec)
	_, err := cf.Execute(ctx, []string{"a", "b"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// TestComboFallback_NilExec — defensive: a nil exec returns a
// configuration error rather than panicking.
func TestComboFallback_NilExec(t *testing.T) {
	cf := NewComboFallback(nil)
	_, err := cf.Execute(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected error from nil exec")
	}
}

// TestComboFallback_EmptyModels — defensive: empty models list is
// an error.
func TestComboFallback_EmptyModels(t *testing.T) {
	exec := func(ctx context.Context, model string) (*Response, error) {
		return &Response{Status: http.StatusOK}, nil
	}
	cf := NewComboFallback(exec)
	_, err := cf.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error from empty models")
	}
}

// TestNewUpstreamError_Error — the Error() formatting includes the
// status and text.
func TestNewUpstreamError_Error(t *testing.T) {
	e := NewUpstreamError(429, "rate limit", nil)
	if e.Error() != "upstream 429: rate limit" {
		t.Errorf("Error() = %q", e.Error())
	}
	inner := errors.New("network")
	e2 := NewUpstreamError(0, "", inner)
	if e2.Unwrap() != inner {
		t.Errorf("Unwrap() did not return inner error")
	}
}
