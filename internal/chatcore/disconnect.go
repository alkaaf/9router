package chatcore

import (
	"context"
	"errors"
	"sync"
	"time"
)

// DisconnectWatcher observes a request's context and a
// server-side done channel. When the client disconnects (ctx fires
// before the server signals completion), the watcher invokes
// onDisconnect exactly once and returns ErrClientDisconnect.
//
// When the server completes the stream normally, the caller must
// call MarkComplete to prevent a spurious "disconnect" from firing
// later if the context happens to be cancelled at a later time
// (which happens with keep-alive timeouts after a successful
// response).
type DisconnectWatcher struct {
	ctx           context.Context
	cancelUp      context.CancelFunc
	doneCh        <-chan struct{}
	completeCh    chan struct{}
	completeOnce  sync.Once
	disconnectCh  chan error
	onDisconnect  func()
	onceFired     sync.Once
	waitOnce      sync.Once
	disconnectErr error
}

// NewDisconnectWatcher returns a watcher. cancelUp cancels the
// upstream request; doneCh signals server-side completion (the
// handler closes it before returning). onDisconnect, if non-nil, is
// invoked on disconnect before cancelUp is called.
func NewDisconnectWatcher(ctx context.Context, cancelUp context.CancelFunc, doneCh <-chan struct{}, onDisconnect func()) *DisconnectWatcher {
	w := &DisconnectWatcher{
		ctx:          ctx,
		cancelUp:     cancelUp,
		doneCh:       doneCh,
		completeCh:   make(chan struct{}),
		disconnectCh: make(chan error, 1),
		onDisconnect: onDisconnect,
	}
	if w.doneCh == nil {
		// No done channel → server is implicitly never done. The
		// caller must call MarkComplete.
		w.doneCh = make(chan struct{})
	}
	go w.run()
	return w
}

func (w *DisconnectWatcher) run() {
	defer close(w.disconnectCh)
	for {
		// Priority: completion beats disconnect. We check
		// completeCh and doneCh non-blockingly first so that a
		// server that signals "done" before the client cancels
		// always wins regardless of Go's random select tiebreak.
		select {
		case <-w.completeCh:
			return
		default:
		}
		select {
		case <-w.completeCh:
			return
		case <-w.doneCh:
			w.completeOnce.Do(func() { close(w.completeCh) })
			return
		default:
		}
		select {
		case <-w.completeCh:
			return
		case <-w.doneCh:
			w.completeOnce.Do(func() { close(w.completeCh) })
			return
		case <-w.ctx.Done():
			w.onceFired.Do(func() {
				w.disconnectErr = ErrClientDisconnect
				if w.onDisconnect != nil {
					w.onDisconnect()
				}
				if w.cancelUp != nil {
					w.cancelUp()
				}
				w.disconnectCh <- ErrClientDisconnect
			})
			return
		}
	}
}

// MarkComplete signals that the stream finished normally. Safe to
// call multiple times. After MarkComplete returns, the watcher will
// not fire the disconnect callback.
func (w *DisconnectWatcher) MarkComplete() {
	if w == nil {
		return
	}
	w.completeOnce.Do(func() { close(w.completeCh) })
}

// Wait blocks until either the client disconnects, the server
// signals completion, or the optional timeout elapses. It returns
// ErrClientDisconnect on disconnect, nil on normal completion.
func (w *DisconnectWatcher) Wait(timeout time.Duration) error {
	if w == nil {
		return nil
	}
	// If already disconnected, the buffered channel will deliver
	// the error immediately.
	select {
	case err := <-w.disconnectCh:
		return err
	default:
	}
	if timeout <= 0 {
		return <-w.disconnectCh
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case err := <-w.disconnectCh:
		return err
	case <-t.C:
		return nil
	}
}

// IsDisconnectErr reports whether the supplied error indicates a
// client disconnect. It recognises ErrClientDisconnect and
// context.Canceled (the upstream cancel we trigger).
func IsDisconnectErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrClientDisconnect) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	return false
}
