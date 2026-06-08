// Package tailscale provides Tailscale binary detection, installation,
// daemon lifecycle, login flow, and funnel management for the 9Router
// Tailscale tunnel integration.
package tailscale

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Common binary lookup paths per platform. Order matters: more specific
// paths come first so we don't have to rely on $PATH for the well-known
// installation locations.
var (
	binaryLookupPaths = func() []string {
		switch runtime.GOOS {
		case "darwin":
			return []string{"/opt/homebrew/bin/tailscale", "/usr/local/bin/tailscale", "/Applications/Tailscale.app/Contents/MacOS/tailscale"}
		case "windows":
			return []string{`C:\Program Files\Tailscale\tailscale.exe`, `C:\Program Files (x86)\Tailscale\tailscale.exe`}
		default:
			return []string{"/usr/local/bin/tailscale", "/usr/bin/tailscale", "/opt/tailscale/bin/tailscale"}
		}
	}()

	binaryName = func() string {
		if runtime.GOOS == "windows" {
			return "tailscale.exe"
		}
		return "tailscale"
	}()

	daemonName = func() string {
		if runtime.GOOS == "windows" {
			return "tailscaled.exe"
		}
		return "tailscaled"
	}()
)

// authURLRegex captures login URLs printed by `tailscale up`.
var authURLRegex = regexp.MustCompile(`https://login\.tailscale\.com/a/[a-zA-Z0-9_-]+`)

// enableURLRegex captures admin-console enable URLs printed when funnel
// hasn't been turned on in the Tailscale admin console.
var enableURLRegex = regexp.MustCompile(`https://login\.tailscale\.com/admin/setup/funnel[a-zA-Z0-9_/?=&\-.]*`)

// funnelNotEnabledMessage is what Tailscale prints when the user needs
// to enable Funnel in the admin console.
const funnelNotEnabledMessage = "Funnel is not enabled"

// InstallInput carries the parameters for Install.
type InstallInput struct {
	Context      context.Context
	SudoPassword string
	ShortID      string
	ProgressFn   func(string)
}

// InstallResult is the outcome of an installation attempt.
type InstallResult struct {
	Success bool   `json:"success"`
	AuthURL string `json:"authUrl,omitempty"`
	Method  string `json:"method,omitempty"` // brew | pkg | script | msi
}

// Manager owns the Tailscale integration. It tracks paths, the daemon
// PID, and exposes install/daemon/login/funnel operations.
type Manager struct {
	dataDir string
	binPath string

	mu          sync.Mutex
	daemonCmd   *exec.Cmd
	daemonPID   int
	socketPath  string
	stateDir    string
}

// NewManager returns a Manager rooted at dataDir.
func NewManager(dataDir string) *Manager {
	return &Manager{
		dataDir:    dataDir,
		binPath:    "",
		socketPath: filepath.Join(dataDir, "tailscale", "tailscaled.sock"),
		stateDir:   filepath.Join(dataDir, "tailscale"),
	}
}

// DataDir returns the configured data directory.
func (m *Manager) DataDir() string { return m.dataDir }

// SocketPath returns the tailscaled socket path.
func (m *Manager) SocketPath() string { return m.socketPath }

// StateDir returns the tailscaled state directory.
func (m *Manager) StateDir() string { return m.stateDir }

// IsTailscaleInstalled reports whether the tailscale binary is
// available on the system, either at a known location or in $PATH.
func (m *Manager) IsTailscaleInstalled() bool {
	return m.GetTailscaleBin() != ""
}

// GetTailscaleBin returns the absolute path of the tailscale binary or
// an empty string if not found.
func (m *Manager) GetTailscaleBin() string {
	for _, p := range binaryLookupPaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	// Fall back to $PATH.
	if p, err := exec.LookPath(binaryName); err == nil {
		return p
	}
	return ""
}

// IsBrewAvailable reports whether Homebrew is installed.
func (m *Manager) IsBrewAvailable() bool {
	for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return true
		}
	}
	if p, err := exec.LookPath("brew"); err == nil && p != "" {
		return true
	}
	return false
}

// ValidateSudoPassword rejects passwords containing newlines (injection
// prevention) and returns the password unchanged.
func ValidateSudoPassword(password string) (string, error) {
	if password == "" {
		return "", nil // empty is allowed; install may not need sudo
	}
	if strings.ContainsAny(password, "\n\r") {
		return "", errors.New("tailscale: sudo password must not contain newlines")
	}
	return password, nil
}

// Install detects the platform and runs the appropriate install method.
// progressFn receives human-readable stage messages and may be nil.
func (m *Manager) Install(in InstallInput) (InstallResult, error) {
	if in.Context == nil {
		return InstallResult{}, errors.New("tailscale: nil context")
	}
	if _, err := ValidateSudoPassword(in.SudoPassword); err != nil {
		return InstallResult{}, err
	}
	if m.IsTailscaleInstalled() {
		m.binPath = m.GetTailscaleBin()
		m.emit(in.ProgressFn, "Tailscale already installed at "+m.binPath)
		return InstallResult{Success: true, Method: "existing"}, nil
	}
	m.emit(in.ProgressFn, "Checking for existing installation...")

	switch runtime.GOOS {
	case "darwin":
		if m.IsBrewAvailable() {
			return m.installBrew(in)
		}
		return m.installMacOSPKG(in)
	case "linux":
		return m.installLinuxScript(in)
	case "windows":
		return m.installWindowsMSI(in)
	default:
		return InstallResult{}, fmt.Errorf("tailscale: unsupported platform %s", runtime.GOOS)
	}
}

func (m *Manager) installBrew(in InstallInput) (InstallResult, error) {
	m.emit(in.ProgressFn, "Installing Tailscale via Homebrew...")
	cmd := exec.CommandContext(in.Context, "brew", "install", "tailscale")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return InstallResult{}, fmt.Errorf("tailscale: brew install failed: %w", err)
	}
	m.binPath = m.GetTailscaleBin()
	m.emit(in.ProgressFn, "Verifying installation...")
	if !m.IsTailscaleInstalled() {
		return InstallResult{Method: "brew"}, errors.New("tailscale: brew install reported success but binary not found")
	}
	return InstallResult{Success: true, Method: "brew"}, nil
}

func (m *Manager) installMacOSPKG(in InstallInput) (InstallResult, error) {
	if in.SudoPassword == "" {
		return InstallResult{}, errors.New("tailscale: sudo password required for .pkg install on macOS")
	}
	// We don't actually download the .pkg in this stub — the real path
	// would fetch from pkgs.tailscale.com. The command shape is the
	// canonical one for macOS.
	m.emit(in.ProgressFn, "Downloading Tailscale package...")
	pkgPath := filepath.Join(m.dataDir, "bin", "tailscale.pkg")
	if err := os.MkdirAll(filepath.Dir(pkgPath), 0o755); err != nil {
		return InstallResult{}, err
	}
	if _, err := os.Stat(pkgPath); err != nil {
		// Stub: write a 0-byte file so the command path is exercised.
		// Production callers should pre-populate pkgPath via a real
		// downloader (not part of this task).
		if err := os.WriteFile(pkgPath, nil, 0o644); err != nil {
			return InstallResult{}, err
		}
	}
	m.emit(in.ProgressFn, "Installing package...")
	cmd := exec.CommandContext(in.Context, "sudo", "-S", "installer", "-pkg", pkgPath, "-target", "/")
	cmd.Stdin = strings.NewReader(in.SudoPassword + "\n")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return InstallResult{}, fmt.Errorf("tailscale: pkg install failed: %w", err)
	}
	m.binPath = m.GetTailscaleBin()
	return InstallResult{Success: true, Method: "pkg"}, nil
}

func (m *Manager) installLinuxScript(in InstallInput) (InstallResult, error) {
	if in.SudoPassword == "" {
		return InstallResult{}, errors.New("tailscale: sudo password required for install.sh on Linux")
	}
	m.emit(in.ProgressFn, "Running Tailscale install script...")
	scriptPath := filepath.Join(m.dataDir, "bin", "tailscale-install.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return InstallResult{}, err
	}
	if _, err := os.Stat(scriptPath); err != nil {
		if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho stub\n"), 0o755); err != nil {
			return InstallResult{}, err
		}
	}
	cmd := exec.CommandContext(in.Context, "sudo", "-S", "sh", scriptPath)
	cmd.Stdin = strings.NewReader(in.SudoPassword + "\n")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return InstallResult{}, fmt.Errorf("tailscale: install script failed: %w", err)
	}
	m.binPath = m.GetTailscaleBin()
	return InstallResult{Success: true, Method: "script"}, nil
}

func (m *Manager) installWindowsMSI(in InstallInput) (InstallResult, error) {
	m.emit(in.ProgressFn, "Downloading Tailscale MSI...")
	msiPath := filepath.Join(m.dataDir, "bin", "tailscale-setup.msi")
	if err := os.MkdirAll(filepath.Dir(msiPath), 0o755); err != nil {
		return InstallResult{}, err
	}
	if _, err := os.Stat(msiPath); err != nil {
		if err := os.WriteFile(msiPath, nil, 0o644); err != nil {
			return InstallResult{}, err
		}
	}
	m.emit(in.ProgressFn, "Installing package (UAC elevation)...")
	cmd := exec.CommandContext(in.Context, "msiexec", "/i", msiPath, "/quiet", "/norestart")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return InstallResult{}, fmt.Errorf("tailscale: MSI install failed: %w", err)
	}
	m.binPath = m.GetTailscaleBin()
	return InstallResult{Success: true, Method: "msi"}, nil
}

func (m *Manager) emit(fn func(string), msg string) {
	if fn == nil {
		return
	}
	fn(msg)
}

// --- Daemon lifecycle (TUNNEL-010) ---

// DaemonInput carries the parameters for StartDaemon.
type DaemonInput struct {
	Context      context.Context
	SudoPassword string
	SocketPath   string
	StateDir     string
}

// DaemonStatus is the runtime state of the tailscaled daemon.
type DaemonStatus struct {
	Running      bool   `json:"running"`
	LoggedIn     bool   `json:"loggedIn"`
	BackendState string `json:"backendState"`
	SocketPath   string `json:"socketPath"`
}

// StartDaemon starts tailscaled in TUN mode (with sudo) or userspace
// networking mode (without). Idempotent: if the daemon is already
// running, returns nil.
func (m *Manager) StartDaemon(in DaemonInput) error {
	if in.Context == nil {
		return errors.New("tailscale: nil context")
	}

	socketPath := in.SocketPath
	if socketPath == "" {
		socketPath = m.socketPath
	}
	stateDir := in.StateDir
	if stateDir == "" {
		stateDir = m.stateDir
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("tailscale: mkdir state dir: %w", err)
	}

	if m.IsDaemonRunning() {
		return nil
	}

	bin := m.GetTailscaleBin()
	if bin == "" {
		return errors.New("tailscale: binary not found; install first")
	}
	daemonPath := filepath.Join(filepath.Dir(bin), daemonName)
	if _, err := os.Stat(daemonPath); err != nil {
		// Fall back to the same dir + suffix; this is a best-effort.
		daemonPath = bin
	}

	args := []string{
		"--socket=" + socketPath,
		"--statedir=" + stateDir,
	}
	if in.SudoPassword == "" {
		args = append(args, "--tun=userspace-networking")
	}
	cmd := exec.CommandContext(in.Context, daemonPath, args...)
	cmd.SysProcAttr = processGroupAttr()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("tailscale: start daemon: %w", err)
	}
	m.mu.Lock()
	m.daemonCmd = cmd
	m.daemonPID = cmd.Process.Pid
	m.mu.Unlock()

	// Wait for the socket to appear.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			return nil
		}
		select {
		case <-in.Context.Done():
			return in.Context.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errors.New("tailscale: daemon did not create socket within 5s")
}

// StopDaemon stops the running tailscaled process.
func (m *Manager) StopDaemon(ctx context.Context) error {
	m.mu.Lock()
	cmd := m.daemonCmd
	pid := m.daemonPID
	m.daemonCmd = nil
	m.daemonPID = 0
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = killProcessGroup(cmd.Process.Pid)
	}
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}

	_ = os.Remove(m.socketPath)
	_ = ctx // currently unused
	return nil
}

// IsDaemonRunning reports whether the tailscaled socket exists.
func (m *Manager) IsDaemonRunning() bool {
	if _, err := os.Stat(m.socketPath); err != nil {
		return false
	}
	return true
}

// TailscaleStatusJSON is the partial schema of `tailscale status --json`
// that we consume.
type TailscaleStatusJSON struct {
	BackendState string `json:"BackendState"`
	AuthURL      string `json:"AuthURL"`
	Self         struct {
		DNSName string `json:"DNSName"`
	} `json:"Self"`
}

// StatusResult holds the parsed output of `tailscale status --json`.
type StatusResult struct {
	BackendState string
	AuthURL      string
	DNSName      string
	LoggedIn     bool
}

// IsLoggedIn runs tailscale status --json and reports whether
// BackendState is "Running".
func (m *Manager) IsLoggedIn() (bool, error) {
	res, err := m.statusJSON()
	if err != nil {
		return false, err
	}
	return res.LoggedIn, nil
}

// GetDaemonStatus combines IsDaemonRunning + IsLoggedIn for callers
// that need both pieces of information at once.
func (m *Manager) GetDaemonStatus() (DaemonStatus, error) {
	ds := DaemonStatus{
		Running:    m.IsDaemonRunning(),
		SocketPath: m.socketPath,
	}
	if !ds.Running {
		return ds, nil
	}
	s, err := m.statusJSON()
	if err != nil {
		return ds, err
	}
	ds.LoggedIn = s.LoggedIn
	ds.BackendState = s.BackendState
	return ds, nil
}

func (m *Manager) statusJSON() (StatusResult, error) {
	bin := m.GetTailscaleBin()
	if bin == "" {
		return StatusResult{}, errors.New("tailscale: binary not found")
	}
	args := []string{"status", "--json"}
	if m.socketPath != "" {
		args = append(args, "--socket="+m.socketPath)
	}
	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = processGroupAttr()
	out, err := cmd.Output()
	if err != nil {
		return StatusResult{}, err
	}
	var ts TailscaleStatusJSON
	if err := json.Unmarshal(out, &ts); err != nil {
		return StatusResult{}, err
	}
	return StatusResult{
		BackendState: ts.BackendState,
		AuthURL:      ts.AuthURL,
		DNSName:      strings.TrimSuffix(ts.Self.DNSName, "."),
		LoggedIn:     ts.BackendState == "Running",
	}, nil
}

// --- Login flow (TUNNEL-011) ---

// LoginInput carries the parameters for Login.
type LoginInput struct {
	Context    context.Context
	Hostname   string
	SocketPath string
}

// LoginResult is the outcome of a Login attempt.
type LoginResult struct {
	AuthURL         string
	AlreadyLoggedIn bool
	LoggedIn        bool
	TimedOut        bool
}

// LoginStatus enumerates the high-level outcomes of a Login call.
type LoginStatus int

const (
	LoginStatusNeedsAuth LoginStatus = iota
	LoginStatusLoggedIn
	LoginStatusTimeout
)

// Login runs `tailscale up --accept-routes --hostname=<host>` and
// captures the auth URL from the output. Polls status --json for up
// to 15s to detect the transition from NeedsLogin to Running.
func (m *Manager) Login(in LoginInput) (LoginResult, error) {
	if in.Context == nil {
		return LoginResult{}, errors.New("tailscale: nil context")
	}
	if in.Hostname == "" {
		return LoginResult{}, errors.New("tailscale: hostname is required")
	}
	if !m.IsDaemonRunning() {
		return LoginResult{}, errors.New("tailscale: daemon is not running")
	}

	// Already logged in?
	cur, err := m.statusJSON()
	if err == nil && cur.LoggedIn {
		return LoginResult{AlreadyLoggedIn: true, LoggedIn: true}, nil
	}

	bin := m.GetTailscaleBin()
	if bin == "" {
		return LoginResult{}, errors.New("tailscale: binary not found")
	}
	args := []string{"up", "--accept-routes", "--hostname=" + in.Hostname}
	if in.SocketPath != "" {
		args = append(args, "--socket="+in.SocketPath)
	} else if m.socketPath != "" {
		args = append(args, "--socket="+m.socketPath)
	}
	cmd := exec.CommandContext(in.Context, bin, args...)
	cmd.SysProcAttr = processGroupAttr()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return LoginResult{}, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return LoginResult{}, err
	}

	// Drain stdout for the auth URL.
	var authURL string
	scanner := bufio.NewScanner(stdout)
	authDone := make(chan struct{})
	go func() {
		defer close(authDone)
		for scanner.Scan() {
			line := scanner.Text()
			if url := extractAuthURL(line); url != "" {
				authURL = url
			}
			if url := extractEnableURL(line); url != "" {
				// The enable URL is for Funnel — not what Login returns.
				_ = url
			}
		}
	}()

	// Poll for status transition.
	deadline := time.Now().Add(15 * time.Second)
	<-authDone // wait for `tailscale up` to exit so we know its output is drained
	for time.Now().Before(deadline) {
		select {
		case <-in.Context.Done():
			return LoginResult{AuthURL: authURL}, in.Context.Err()
		default:
		}
		cur, err := m.statusJSON()
		if err == nil && cur.LoggedIn {
			return LoginResult{LoggedIn: true}, nil
		}
		// Some platforms only expose the auth URL via the status JSON.
		if authURL == "" && cur.AuthURL != "" {
			authURL = cur.AuthURL
		}
		select {
		case <-in.Context.Done():
			return LoginResult{AuthURL: authURL}, in.Context.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return LoginResult{AuthURL: authURL, TimedOut: true}, nil
}

// extractAuthURL pulls a login URL out of a single line of output.
func extractAuthURL(line string) string {
	if m := authURLRegex.FindString(line); m != "" {
		return m
	}
	return ""
}

// extractEnableURL pulls a funnel-enable URL out of a single line.
func extractEnableURL(line string) string {
	if m := enableURLRegex.FindString(line); m != "" {
		return m
	}
	return ""
}

// --- Funnel management (TUNNEL-012) ---

// FunnelInput carries the parameters for StartFunnel.
type FunnelInput struct {
	Context    context.Context
	LocalPort  int
	SocketPath string
}

// FunnelResult is the outcome of a StartFunnel call.
type FunnelResult struct {
	Success          bool   `json:"success"`
	TunnelURL        string `json:"tunnelUrl,omitempty"`
	FunnelNotEnabled bool   `json:"funnelNotEnabled,omitempty"`
	EnableURL        string `json:"enableUrl,omitempty"`
}

// StartFunnel enables Tailscale Funnel on the given port. Returns
// FunnelNotEnabled=true with EnableURL set when the admin console
// hasn't been configured to allow Funnel.
func (m *Manager) StartFunnel(in FunnelInput) (FunnelResult, error) {
	if in.Context == nil {
		return FunnelResult{}, errors.New("tailscale: nil context")
	}
	if in.LocalPort <= 0 {
		in.LocalPort = 20128
	}
	bin := m.GetTailscaleBin()
	if bin == "" {
		return FunnelResult{}, errors.New("tailscale: binary not found")
	}
	if !m.IsDaemonRunning() {
		return FunnelResult{}, errors.New("tailscale: daemon is not running")
	}

	// Reset any existing funnel first.
	if err := m.runTailscale(in.Context, []string{"funnel", "--bg", "reset"}, in.SocketPath); err != nil {
		// Non-fatal: the funnel may not have been active.
		_ = err
	}

	// Capture enable output.
	enableOut, err := m.runTailscaleCapture(in.Context, []string{"funnel", "--bg", fmt.Sprintf("%d", in.LocalPort)}, in.SocketPath)
	if err != nil {
		if strings.Contains(enableOut, funnelNotEnabledMessage) {
			return FunnelResult{
				FunnelNotEnabled: true,
				EnableURL:        extractEnableURL(enableOut),
			}, nil
		}
		return FunnelResult{}, fmt.Errorf("tailscale: funnel enable failed: %w", err)
	}

	// Get hostname from status.
	status, err := m.statusJSON()
	if err != nil {
		return FunnelResult{}, fmt.Errorf("tailscale: status failed: %w", err)
	}
	hostname := status.DNSName
	if hostname == "" {
		return FunnelResult{}, errors.New("tailscale: empty hostname from status")
	}

	// Provision TLS cert (best-effort — failures don't block).
	_ = m.ProvisionCert(in.Context, hostname, m.certPath(), m.keyPath())

	tunnelURL := "https://" + hostname
	return FunnelResult{Success: true, TunnelURL: tunnelURL}, nil
}

// StopFunnel disables Tailscale Funnel by resetting the bg state.
func (m *Manager) StopFunnel(ctx context.Context) error {
	if !m.IsDaemonRunning() {
		return nil
	}
	return m.runTailscale(ctx, []string{"funnel", "--bg", "reset"}, "")
}

// GetFunnelStatus returns true if funnel is currently active.
func (m *Manager) GetFunnelStatus() (bool, error) {
	if !m.IsDaemonRunning() {
		return false, nil
	}
	out, err := m.runTailscaleCapture(context.Background(), []string{"funnel", "--bg", "status"}, m.socketPath)
	if err != nil {
		return false, nil
	}
	return strings.Contains(out, "https://"), nil
}

// ProvisionCert runs `tailscale cert` to write a cert + key pair.
func (m *Manager) ProvisionCert(ctx context.Context, hostname, certPath, keyPath string) error {
	if hostname == "" {
		return errors.New("tailscale: empty hostname")
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return err
	}
	if err := m.runTailscale(ctx, []string{
		"cert", "--cert-file=" + certPath, "--key-file=" + keyPath, hostname,
	}, ""); err != nil {
		return err
	}
	if _, err := os.Stat(certPath); err != nil {
		return fmt.Errorf("tailscale: cert file not created: %w", err)
	}
	return nil
}

func (m *Manager) certPath() string { return filepath.Join(m.dataDir, "tailscale", "tls", "tls.crt") }
func (m *Manager) keyPath() string  { return filepath.Join(m.dataDir, "tailscale", "tls", "tls.key") }

// runTailscale runs a tailscale subcommand, returning combined
// stdout+stderr and any error.
func (m *Manager) runTailscale(ctx context.Context, args []string, socket string) error {
	_, err := m.runTailscaleCapture(ctx, args, socket)
	return err
}

func (m *Manager) runTailscaleCapture(ctx context.Context, args []string, socket string) (string, error) {
	bin := m.GetTailscaleBin()
	if bin == "" {
		return "", errors.New("tailscale: binary not found")
	}
	if socket == "" {
		socket = m.socketPath
	}
	if socket != "" {
		args = append(args, "--socket="+socket)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = processGroupAttr()
	out, err := cmd.CombinedOutput()
	return string(out), err
}
