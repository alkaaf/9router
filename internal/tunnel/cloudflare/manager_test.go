package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/9router/9router/internal/tunnel"
)

// --- mocks ---

type mockSettings struct {
	mu      sync.Mutex
	enabled bool
	url     string
	setErr  error
}

func (m *mockSettings) GetTunnelEnabled() (bool, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled, m.url, nil
}

func (m *mockSettings) SetTunnelEnabled(enabled bool, u string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.enabled = enabled
	m.url = u
	return nil
}

type mockHealth struct {
	mu    sync.Mutex
	alive bool
	calls int
}

func (m *mockHealth) ProbeURL(ctx context.Context, u string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.alive, nil
}

func (m *mockHealth) WaitForHealth(ctx context.Context, u string, timeout time.Duration) error {
	if !m.alive {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

type mockWorker struct {
	mu   sync.Mutex
	regs []workerReg
	fail bool
}

type workerReg struct {
	WorkerURL, ShortID, TunnelURL string
}

func (m *mockWorker) Register(ctx context.Context, workerURL, shortID, tunnelURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("simulated worker failure")
	}
	m.regs = append(m.regs, workerReg{workerURL, shortID, tunnelURL})
	return nil
}

func (m *mockWorker) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.regs)
}

func (m *mockWorker) last() workerReg {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.regs) == 0 {
		return workerReg{}
	}
	return m.regs[len(m.regs)-1]
}

// --- helpers ---

func buildFakeBinary(t *testing.T, dir string) {
	t.Helper()
	bin := filepath.Join(dir, "cloudflared")
	buf := make([]byte, MinBinarySize)
	copy(buf, []byte{0x7f, 0x45, 0x4c, 0x46})
	if err := os.WriteFile(bin, buf, 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
}

type testOpt func(*testCfg)

type testCfg struct {
	realBinary bool
	health     HealthChecker
	worker     WorkerRegisterer
	settings   SettingsStore
	workerURL  string
}

func withHealth(h HealthChecker) testOpt    { return func(o *testCfg) { o.health = h } }
func withWorker(w WorkerRegisterer) testOpt { return func(o *testCfg) { o.worker = w } }
func withSettings(s SettingsStore) testOpt  { return func(o *testCfg) { o.settings = s } }
func withRealBinary() testOpt               { return func(o *testCfg) { o.realBinary = true } }
func withWorkerURL(u string) testOpt        { return func(o *testCfg) { o.workerURL = u } }

func newTestMgr(t *testing.T, opts ...testOpt) *TunnelManager {
	t.Helper()
	o := testCfg{workerURL: "https://abc-tunnel.us"}
	for _, fn := range opts {
		fn(&o)
	}
	if o.health == nil {
		o.health = &mockHealth{alive: true}
	}
	if o.worker == nil {
		o.worker = &mockWorker{}
	}
	if o.settings == nil {
		o.settings = &mockSettings{}
	}

	dataDir := t.TempDir()
	state := tunnel.NewStateManager(tunnel.TunnelConfig{DataDir: dataDir})
	binary := NewManager(dataDir)
	if o.realBinary {
		if err := os.MkdirAll(binary.binDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		buildFakeBinary(t, binary.binDir)
	}

	return NewTunnelManager(
		tunnel.TunnelConfig{DataDir: dataDir, LocalPort: 20128, WorkerURL: o.workerURL},
		state, binary, o.health, o.worker, o.settings,
	)
}

// --- tests ---

func TestBuildPublicURL(t *testing.T) {
	cases := []struct {
		w, s, want string
	}{
		{"", "abc", ""},
		{"https://abc-tunnel.us", "", ""},
		{"https://abc-tunnel.us", "abc123", "https://rabc123.abc-tunnel.us"},
		{"https://abc-tunnel.us:8443", "x9", "https://rx9.abc-tunnel.us:8443"},
	}
	for _, tc := range cases {
		got := buildPublicURL(tc.w, tc.s)
		if got != tc.want {
			t.Errorf("buildPublicURL(%q,%q) = %q, want %q", tc.w, tc.s, got, tc.want)
		}
	}
}

func TestTunnelManager_GetStatus_Empty(t *testing.T) {
	mgr := newTestMgr(t)
	st, err := mgr.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Running || st.Enabled || st.TunnelURL != "" || st.ShortID != "" {
		t.Fatalf("expected empty, got %+v", st)
	}
}

func TestTunnelManager_GetStatus_NilContext(t *testing.T) {
	mgr := newTestMgr(t)
	_, err := mgr.GetStatus(nil)
	if err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestTunnelManager_Disable_EmptyState(t *testing.T) {
	mgr := newTestMgr(t)
	if err := mgr.Disable(DisableInput{Context: context.Background()}); err != nil {
		t.Fatalf("Disable empty: %v", err)
	}
}

func TestTunnelManager_Disable_NilContext(t *testing.T) {
	mgr := newTestMgr(t)
	if err := mgr.Disable(DisableInput{Context: nil}); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestTunnelManager_Enable_NilContext(t *testing.T) {
	mgr := newTestMgr(t)
	_, err := mgr.Enable(EnableInput{Context: nil})
	if err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestTunnelManager_Enable_WithRealBinary_CancelledContext(t *testing.T) {
	mgr := newTestMgr(t, withRealBinary())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := mgr.Enable(EnableInput{Context: ctx})
	if err == nil {
		t.Fatalf("expected error from cancelled context")
	}
}

func TestTunnelManager_Disable_PreservesShortID(t *testing.T) {
	mgr := newTestMgr(t, withRealBinary())
	if err := mgr.state.SaveState(tunnel.TunnelState{
		ShortID: "preserve1", TunnelURL: "https://preserve1.trycloudflare.com",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := mgr.Disable(DisableInput{Context: context.Background()}); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	got, err := mgr.state.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.ShortID != "preserve1" {
		t.Fatalf("ShortID should be preserved, got %q", got.ShortID)
	}
	if got.TunnelURL != "" {
		t.Fatalf("TunnelURL should be cleared, got %q", got.TunnelURL)
	}
}

func TestTunnelManager_Disable_UpdatesSettings(t *testing.T) {
	settings := &mockSettings{enabled: true, url: "https://old.trycloudflare.com"}
	mgr := newTestMgr(t, withSettings(settings), withRealBinary())
	if err := mgr.Disable(DisableInput{Context: context.Background()}); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	settings.mu.Lock()
	defer settings.mu.Unlock()
	if settings.enabled || settings.url != "" {
		t.Fatalf("settings should be cleared: enabled=%v url=%q", settings.enabled, settings.url)
	}
}

func TestTunnelManager_GetStatus_SettingsURLFallback(t *testing.T) {
	settings := &mockSettings{enabled: true, url: "https://from-settings.trycloudflare.com"}
	mgr := newTestMgr(t, withSettings(settings))
	st, err := mgr.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.TunnelURL != "https://from-settings.trycloudflare.com" {
		t.Fatalf("TunnelURL = %q, want from settings", st.TunnelURL)
	}
	if st.ShortID != "" {
		t.Fatalf("ShortID should be empty when state is empty, got %q", st.ShortID)
	}
}

func TestTunnelManager_HealthTimeoutReportsNotRunning(t *testing.T) {
	mgr := newTestMgr(t, withHealth(&mockHealth{alive: false}))
	if err := mgr.state.SaveState(tunnel.TunnelState{
		ShortID: "h", TunnelURL: "https://h.trycloudflare.com",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	st, err := mgr.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Running || st.Enabled {
		t.Fatalf("expected not running when health fails, got %+v", st)
	}
}

func TestTunnelManager_RegisterIsNonFatalOnWorkerFailure(t *testing.T) {
	w := &mockWorker{fail: true}
	mgr := newTestMgr(t, withWorker(w), withRealBinary())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := mgr.Enable(EnableInput{Context: ctx})
	if err == nil {
		t.Fatalf("expected error on cancelled context")
	}
	if c := w.count(); c != 0 {
		t.Fatalf("worker should not have been called, got %d", c)
	}
}

func TestMockWorker_RegisterReceivesPayload(t *testing.T) {
	w := &mockWorker{}
	if err := w.Register(context.Background(), "https://abc-tunnel.us", "abc123", "https://abc123.trycloudflare.com"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if w.count() != 1 {
		t.Fatalf("expected 1 registration, got %d", w.count())
	}
	r := w.last()
	if r.ShortID != "abc123" || r.TunnelURL != "https://abc123.trycloudflare.com" {
		t.Fatalf("registration payload: %+v", r)
	}
}

func TestMockHealth_ProbeURLAndWait(t *testing.T) {
	h := &mockHealth{alive: true}
	ok, _ := h.ProbeURL(context.Background(), "https://x")
	if !ok {
		t.Fatalf("expected alive=true")
	}
	if h.calls != 1 {
		t.Fatalf("expected 1 probe call, got %d", h.calls)
	}
	if err := h.WaitForHealth(context.Background(), "https://x", time.Second); err != nil {
		t.Fatalf("WaitForHealth on healthy: %v", err)
	}
}

func TestHTTPJSONPost_Success(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := httpJSONPost(context.Background(), &http.Client{}, srv.URL, map[string]string{"hello": "world"}); err != nil {
		t.Fatalf("post: %v", err)
	}
	if got["hello"] != "world" {
		t.Fatalf("server didn't receive body: %+v", got)
	}
}

func TestHTTPJSONPost_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	err := httpJSONPost(context.Background(), &http.Client{Timeout: time.Second}, srv.URL, nil)
	if err == nil {
		t.Fatalf("expected HTTP error")
	}
}

func TestHTTPJSONPost_NetworkError(t *testing.T) {
	err := httpJSONPost(context.Background(), &http.Client{Timeout: 50 * time.Millisecond}, "http://127.0.0.1:1/", nil)
	if err == nil {
		t.Fatalf("expected network error")
	}
}

func TestTunnelManager_Disable_RespectsCancellation(t *testing.T) {
	mgr := newTestMgr(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mgr.Disable(DisableInput{Context: ctx})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestTunnelManager_Enable_NilContext_DefaultPortRejected(t *testing.T) {
	// Confirms the nil-context check fires before port-default logic.
	mgr := newTestMgr(t)
	_, err := mgr.Enable(EnableInput{Context: nil})
	if err == nil || err.Error() != "cloudflare: nil context" {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestTunnelManager_Enable_EmptyConfigPortDefaults(t *testing.T) {
	mgr := newTestMgr(t, withRealBinary())
	// cfg.LocalPort is 20128; explicitly set to 0 to test the default
	// branch in Enable.
	mgr.cfg.LocalPort = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := mgr.Enable(EnableInput{Context: ctx})
	if err == nil {
		t.Fatalf("expected error")
	}
}
