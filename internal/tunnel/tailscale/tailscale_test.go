package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateSudoPassword(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"", false},
		{"hunter2", false},
		{"pass\nword", true},
		{"with\rcr", true},
		{"normal password with spaces", false},
	}
	for _, tc := range cases {
		_, err := ValidateSudoPassword(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateSudoPassword(%q) err = %v, wantErr = %v", tc.in, err, tc.wantErr)
		}
	}
}

func TestExtractAuthURL(t *testing.T) {
	cases := []struct {
		line, want string
	}{
		{"https://login.tailscale.com/a/abc123def", "https://login.tailscale.com/a/abc123def"},
		{"To authenticate, visit: https://login.tailscale.com/a/zzz", "https://login.tailscale.com/a/zzz"},
		{"no url here", ""},
		{"https://example.com/a/abc", ""},
	}
	for _, tc := range cases {
		got := extractAuthURL(tc.line)
		if got != tc.want {
			t.Errorf("extractAuthURL(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

func TestExtractEnableURL(t *testing.T) {
	cases := []struct {
		line, want string
	}{
		{"https://login.tailscale.com/admin/setup/funnel", "https://login.tailscale.com/admin/setup/funnel"},
		{"", ""},
		{"https://login.tailscale.com/admin/setup/something-else", ""},
	}
	for _, tc := range cases {
		got := extractEnableURL(tc.line)
		if got != tc.want {
			t.Errorf("extractEnableURL(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

func TestNewManager_DefaultPaths(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if m.DataDir() != dir {
		t.Fatalf("DataDir = %q, want %q", m.DataDir(), dir)
	}
	wantSocket := filepath.Join(dir, "tailscale", "tailscaled.sock")
	if m.SocketPath() != wantSocket {
		t.Fatalf("SocketPath = %q, want %q", m.SocketPath(), wantSocket)
	}
	if m.StateDir() == "" {
		t.Fatalf("StateDir should not be empty")
	}
}

func TestManager_Install_NilContext(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Install(InstallInput{Context: nil})
	if err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestManager_Install_BadPassword(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Install(InstallInput{Context: context.Background(), SudoPassword: "bad\npass"})
	if err == nil {
		t.Fatalf("expected error for newline password")
	}
}

func TestManager_Install_ProgressEventsEmitted(t *testing.T) {
	// Set up a fake tailscale binary in /tmp and $PATH so IsTailscaleInstalled
	// reports true. We can't actually run brew/curl/msi/etc, so this is the
	// only path we can fully exercise in a unit test.
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test (uses /tmp)")
	}
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "tailscale")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	m := NewManager(t.TempDir())
	if !m.IsTailscaleInstalled() {
		t.Fatalf("fake tailscale should be detected")
	}
	var progress []string
	res, err := m.Install(InstallInput{
		Context:    context.Background(),
		ProgressFn: func(s string) { progress = append(progress, s) },
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success=true")
	}
	if res.Method != "existing" {
		t.Fatalf("expected method=existing, got %q", res.Method)
	}
	if len(progress) == 0 {
		t.Fatalf("expected at least one progress event")
	}
}

func TestManager_Install_PasswordWithNewlineRejected(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Install(InstallInput{Context: context.Background(), SudoPassword: "x\nrm -rf /"})
	if err == nil {
		t.Fatalf("expected rejection of newline password")
	}
}

func TestManager_StartDaemon_NilContext(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.StartDaemon(DaemonInput{Context: nil}); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestManager_StartDaemon_CreatesStateDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if m.IsTailscaleInstalled() {
		// The real tailscaled needs privileges we don't have in tests;
		// skip this test when tailscale is on the system.
		t.Skipf("tailscale is installed; StartDaemon would hang waiting for socket")
	}
	err := m.StartDaemon(DaemonInput{
		Context:    context.Background(),
		SocketPath: filepath.Join(dir, "tailscale", "tailscaled.sock"),
		StateDir:   filepath.Join(dir, "tailscale"),
	})
	if err == nil {
		t.Fatalf("expected error (no binary)")
	}
	if !strings.Contains(err.Error(), "binary") {
		t.Fatalf("expected binary-not-found error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "tailscale")); statErr != nil {
		t.Fatalf("state dir should be created: %v", statErr)
	}
}

func TestManager_StopDaemon_NoOp(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.StopDaemon(context.Background()); err != nil {
		t.Fatalf("StopDaemon: %v", err)
	}
}

func TestManager_IsDaemonRunning_NoSocket(t *testing.T) {
	m := NewManager(t.TempDir())
	if m.IsDaemonRunning() {
		t.Fatalf("expected false with no socket")
	}
}

func TestManager_IsDaemonRunning_WithSocket(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := os.MkdirAll(filepath.Dir(m.SocketPath()), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(m.SocketPath(), nil, 0o644); err != nil {
		t.Fatalf("write socket: %v", err)
	}
	if !m.IsDaemonRunning() {
		t.Fatalf("expected true with socket present")
	}
}

func TestManager_GetDaemonStatus_NoSocket(t *testing.T) {
	m := NewManager(t.TempDir())
	ds, err := m.GetDaemonStatus()
	if err != nil {
		t.Fatalf("GetDaemonStatus: %v", err)
	}
	if ds.Running {
		t.Fatalf("expected running=false with no socket")
	}
}

func TestManager_Login_NilContext(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Login(LoginInput{Context: nil, Hostname: "x"}); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestManager_Login_NoHostname(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Login(LoginInput{Context: context.Background()}); err == nil {
		t.Fatalf("expected error for missing hostname")
	}
}

func TestManager_Login_NoDaemon(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Login(LoginInput{Context: context.Background(), Hostname: "x"}); err == nil {
		t.Fatalf("expected error when daemon not running")
	}
}

func TestManager_StartFunnel_NilContext(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.StartFunnel(FunnelInput{Context: nil}); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestManager_StartFunnel_NoDaemon(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.StartFunnel(FunnelInput{Context: context.Background(), LocalPort: 20128}); err == nil {
		t.Fatalf("expected error when daemon not running")
	}
}

func TestManager_StopFunnel_NoDaemon(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.StopFunnel(context.Background()); err != nil {
		t.Fatalf("StopFunnel on no daemon should be no-op, got %v", err)
	}
}

func TestManager_GetFunnelStatus_NoDaemon(t *testing.T) {
	m := NewManager(t.TempDir())
	active, err := m.GetFunnelStatus()
	if err != nil {
		t.Fatalf("GetFunnelStatus: %v", err)
	}
	if active {
		t.Fatalf("expected inactive with no daemon")
	}
}

func TestManager_ProvisionCert_EmptyHostname(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.ProvisionCert(context.Background(), "", "/tmp/cert", "/tmp/key"); err == nil {
		t.Fatalf("expected error for empty hostname")
	}
}

func TestManager_ProvisionCert_NoBinary(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.ProvisionCert(context.Background(), "host.ts.net", "/tmp/cert", "/tmp/key"); err == nil {
		t.Fatalf("expected error when tailscale not installed")
	}
}

func TestTailscaleStatusJSON_Parse(t *testing.T) {
	raw := []byte(`{
		"BackendState": "Running",
		"AuthURL": "https://login.tailscale.com/a/x",
		"Self": {"DNSName": "host.tail123.ts.net."}
	}`)
	var ts TailscaleStatusJSON
	if err := json.Unmarshal(raw, &ts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ts.BackendState != "Running" {
		t.Fatalf("BackendState = %q", ts.BackendState)
	}
	if ts.Self.DNSName != "host.tail123.ts.net." {
		t.Fatalf("DNSName = %q", ts.Self.DNSName)
	}
}

func TestTailscaleStatusJSON_LoggedIn(t *testing.T) {
	raw := []byte(`{"BackendState": "NeedsLogin"}`)
	var ts TailscaleStatusJSON
	if err := json.Unmarshal(raw, &ts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ts.BackendState == "Running" {
		t.Fatalf("expected NeedsLogin, not Running")
	}
}

// Verify that errors from the package can be detected via errors.Is
// when wrapped (sanity check).
func TestErrorWrapping(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := errors.Join(sentinel, errors.New("extra"))
	if !errors.Is(wrapped, sentinel) {
		t.Fatalf("errors.Is should match wrapped sentinel")
	}
}
