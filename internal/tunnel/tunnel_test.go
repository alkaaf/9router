package tunnel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const shortIDAlphabet = "abcdefghijklmnpqrstuvwxyz23456789"

func TestGenerateShortID_LengthAndAlphabet(t *testing.T) {
	id := generateShortID()
	if len(id) != ShortIDLength {
		t.Fatalf("short id length = %d, want %d", len(id), ShortIDLength)
	}
	for i, r := range id {
		if !strings.ContainsRune(shortIDAlphabet, r) {
			t.Fatalf("short id char %d (%q) not in alphabet %q", i, r, shortIDAlphabet)
		}
	}
}

func TestGenerateShortID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := generateShortID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate short id %q after %d iterations", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestStateManager_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(TunnelConfig{DataDir: dir})

	want := TunnelState{ShortID: "abc123", TunnelURL: "https://abc123.trycloudflare.com"}
	if err := mgr.SaveState(want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got != want {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", got, want)
	}
}

func TestStateManager_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(TunnelConfig{DataDir: dir})

	got, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState on fresh dir: %v", err)
	}
	if got != (TunnelState{}) {
		t.Fatalf("expected empty state, got %+v", got)
	}
}

func TestStateManager_CorruptFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(TunnelConfig{DataDir: dir})

	// Ensure parent dir exists and write garbage.
	if err := os.MkdirAll(filepath.Dir(mgr.Path()), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(mgr.Path(), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	got, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState on corrupt file: %v", err)
	}
	if got != (TunnelState{}) {
		t.Fatalf("expected empty state for corrupt file, got %+v", got)
	}
}

func TestStateManager_ClearState(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(TunnelConfig{DataDir: dir})

	if err := mgr.SaveState(TunnelState{ShortID: "x", TunnelURL: "y"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if _, err := os.Stat(mgr.Path()); err != nil {
		t.Fatalf("state file should exist: %v", err)
	}

	if err := mgr.ClearState(); err != nil {
		t.Fatalf("ClearState: %v", err)
	}
	if _, err := os.Stat(mgr.Path()); !os.IsNotExist(err) {
		t.Fatalf("state file should be gone, stat err = %v", err)
	}

	// Clearing again is a no-op.
	if err := mgr.ClearState(); err != nil {
		t.Fatalf("ClearState (second call): %v", err)
	}
}

func TestStateManager_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	// Note: no mkdir of {dir}/tunnel beforehand.
	mgr := NewStateManager(TunnelConfig{DataDir: dir})

	if err := mgr.SaveState(TunnelState{ShortID: "abc", TunnelURL: "url"}); err != nil {
		t.Fatalf("SaveState should create parent dir: %v", err)
	}
	if _, err := os.Stat(mgr.Path()); err != nil {
		t.Fatalf("state file should exist after SaveState: %v", err)
	}
}

func TestStateManager_AtomicWriteProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(TunnelConfig{DataDir: dir})

	want := TunnelState{ShortID: "z9z9z9", TunnelURL: "https://z9z9z9.trycloudflare.com"}
	if err := mgr.SaveState(want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	raw, err := os.ReadFile(mgr.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var parsed TunnelState
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("file should be valid JSON: %v\nraw=%s", err, raw)
	}
	if parsed != want {
		t.Fatalf("parsed = %+v, want %+v", parsed, want)
	}

	// No leftover .tmp file.
	entries, err := os.ReadDir(filepath.Dir(mgr.Path()))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestStateManager_ConcurrentWritesNoCorruption(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(TunnelConfig{DataDir: dir})

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			state := TunnelState{
				ShortID:   "id",
				TunnelURL: "https://example.test/" + strings.Repeat("a", i),
			}
			if err := mgr.SaveState(state); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent SaveState error: %v", err)
	}

	// Final state must be parseable and one of the values we wrote.
	got, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState after concurrent writes: %v", err)
	}
	if got.ShortID != "id" {
		t.Fatalf("ShortID corrupted: %q", got.ShortID)
	}
	if !strings.HasPrefix(got.TunnelURL, "https://example.test/") {
		t.Fatalf("TunnelURL corrupted: %q", got.TunnelURL)
	}
}

func TestStateManager_Path(t *testing.T) {
	mgr := NewStateManager(TunnelConfig{DataDir: "/var/data"})
	want := filepath.Join("/var/data", "tunnel", "state.json")
	if got := mgr.Path(); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestContextCancellationPropagates(t *testing.T) {
	// Sanity check: a function that respects ctx.Done() should return
	// ctx.Err() when cancelled. We exercise this against a tiny helper
	// that mirrors what service methods will do.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runWithCtx(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := runWithCtx(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

// runWithCtx is a tiny helper that demonstrates context-aware behaviour.
// It is used only by the cancellation tests above.
func runWithCtx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestEnsureTunnelDir(t *testing.T) {
	base := t.TempDir()
	// No {base}/tunnel yet.
	if err := EnsureTunnelDir(base); err != nil {
		t.Fatalf("EnsureTunnelDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(base, "tunnel"))
	if err != nil {
		t.Fatalf("tunnel dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("tunnel path is not a directory")
	}
	// Idempotent.
	if err := EnsureTunnelDir(base); err != nil {
		t.Fatalf("EnsureTunnelDir (idempotent): %v", err)
	}
}
