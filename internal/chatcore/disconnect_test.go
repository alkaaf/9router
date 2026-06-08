package chatcore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestDisconnect_FiresOnCancel — AC-001: cancelling the context
// fires the onDisconnect callback and cancels upstream.
func TestDisconnect_FiresOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upCtx, upCancel := context.WithCancel(context.Background())
	var fired int32
	w := NewDisconnectWatcher(ctx, upCancel, nil, func() {
		atomic.StoreInt32(&fired, 1)
	})
	t.Cleanup(w.MarkComplete)

	// Initially no disconnect.
	if err := w.Wait(50 * time.Millisecond); err != nil {
		t.Errorf("unexpected early disconnect: %v", err)
	}

	// Now cancel the client ctx.
	cancel()
	// Wait long enough for the goroutine to fire.
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&fired) != 1 {
		t.Errorf("onDisconnect did not fire")
	}
	// Upstream should be cancelled.
	select {
	case <-upCtx.Done():
	case <-time.After(50 * time.Millisecond):
		t.Errorf("upstream not cancelled")
	}
	// Wait should now return ErrClientDisconnect.
	if err := w.Wait(50 * time.Millisecond); !errors.Is(err, ErrClientDisconnect) {
		t.Errorf("Wait after cancel = %v, want ErrClientDisconnect", err)
	}
}

// TestDisconnect_ServerCloseDoesNotFire — AC-003: when the server
// signals done via doneCh, the callback does NOT fire even if the
// context happens to be cancelled later.
func TestDisconnect_ServerCloseDoesNotFire(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upCtx, upCancel := context.WithCancel(context.Background())
	var fired int32
	done := make(chan struct{})
	_ = NewDisconnectWatcher(ctx, upCancel, done, func() {
		atomic.StoreInt32(&fired, 1)
	})

	close(done) // server signals completion first
	cancel()   // then client disconnects (or whatever)

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&fired) != 0 {
		t.Errorf("onDisconnect should not fire after server done")
	}
	if upCtx.Err() != nil {
		t.Errorf("upstream should NOT be cancelled, got %v", upCtx.Err())
	}
}

// TestDisconnect_NormalFinish — AC-004: a stream that completes
// without the client disconnecting does not fire the callback.
func TestDisconnect_NormalFinish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fired int32
	w := NewDisconnectWatcher(ctx, nil, nil, func() {
		atomic.StoreInt32(&fired, 1)
	})
	w.MarkComplete()
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&fired) != 0 {
		t.Errorf("onDisconnect fired on normal completion")
	}
}

// TestDisconnect_OnlyOnce — defensive: callback is invoked at most
// once even if the context is cancelled multiple times.
func TestDisconnect_OnlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	_ = NewDisconnectWatcher(ctx, nil, nil, func() {
		atomic.AddInt32(&calls, 1)
	})
	cancel()
	cancel()
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("callback called %d times, want 1", got)
	}
}

// TestIsDisconnectErr — the helper recognises the relevant error
// types.
func TestIsDisconnectErr(t *testing.T) {
	if IsDisconnectErr(nil) {
		t.Error("nil should not be a disconnect")
	}
	if !IsDisconnectErr(ErrClientDisconnect) {
		t.Error("ErrClientDisconnect should be recognised")
	}
	if !IsDisconnectErr(context.Canceled) {
		t.Error("context.Canceled should be recognised")
	}
	if IsDisconnectErr(errors.New("other")) {
		t.Error("other errors should not be recognised")
	}
}
