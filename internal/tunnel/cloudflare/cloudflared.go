// Package cloudflare provides cloudflared binary download, validation,
// and process management for Cloudflare quick-tunnel.
package cloudflare

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// GitHubBaseURL is the prefix for downloading official cloudflared
// release artifacts.
const GitHubBaseURL = "https://github.com/cloudflare/cloudflared/releases/latest/download"

// MinBinarySize is the minimum acceptable size for a cloudflared binary
// (~30MB+ in practice). Anything smaller is treated as corrupt/truncated.
const MinBinarySize = 1024 * 1024

// DefaultQuickTunnelTimeout is how long SpawnQuickTunnel waits for the
// trycloudflare.com URL to appear in cloudflared output.
const DefaultQuickTunnelTimeout = 90 * time.Second

// quickTunnelURLRegex extracts the public URL emitted by cloudflared once
// the quick tunnel is up. Anchored to trycloudflare.com to avoid matching
// api.trycloudflare.com (which cloudflared also logs).
var quickTunnelURLRegex = regexp.MustCompile(`https://([a-z0-9-]+)\.trycloudflare\.com`)

// DownloadStatus exposes the in-flight download state for the /api/tunnel
// status endpoint.
type DownloadStatus struct {
	Downloading bool `json:"downloading"`
	Progress    int  `json:"progress"`
}

// Manager owns the cloudflared binary lifecycle on disk and in memory.
type Manager struct {
	dataDir string
	binDir  string
	binPath string

	mu              sync.Mutex
	currentProcess  *exec.Cmd
	currentPIDFile  string
	downloadState   DownloadStatus
	downloadInFlight bool
}

// NewManager returns a Manager rooted at dataDir. The binary is stored at
// {dataDir}/bin/cloudflared[.exe].
func NewManager(dataDir string) *Manager {
	binDir := filepath.Join(dataDir, "bin")
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	return &Manager{
		dataDir:    dataDir,
		binDir:     binDir,
		binPath:    filepath.Join(binDir, binName),
		currentPIDFile: filepath.Join(dataDir, "tunnel", "cloudflared.pid"),
	}
}

// BinPath returns the on-disk path to the cloudflared binary.
func (m *Manager) BinPath() string { return m.binPath }

// GetDownloadStatus returns a copy of the current download state.
func (m *Manager) GetDownloadStatus() DownloadStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.downloadState
}

// DownloadURLFor returns the canonical GitHub release URL for the given
// OS/arch pair. Recognised inputs:
//
//	("darwin"|"linux"|"windows", "amd64"|"arm64"|"386")
//
// Unknown combinations fall back to a safe default per platform.
func DownloadURLFor(goos, arch string) string {
	platforms := platformMappings()
	mapping, ok := platforms[goos]
	if !ok {
		return ""
	}
	name, ok := mapping[arch]
	if !ok {
		if fb, ok := platformFallbacks()[goos]; ok {
			name = fb
		} else {
			return ""
		}
	}
	return GitHubBaseURL + "/" + name
}

func platformMappings() map[string]map[string]string {
	return map[string]map[string]string{
		"darwin": {
			"amd64": "cloudflared-darwin-amd64.tgz",
			"arm64": "cloudflared-darwin-arm64.tgz",
		},
		"linux": {
			"amd64": "cloudflared-linux-amd64",
			"arm64": "cloudflared-linux-arm64",
			"386":   "cloudflared-linux-386",
		},
		"windows": {
			"amd64":  "cloudflared-windows-amd64.exe",
			"arm64":  "cloudflared-windows-arm64.exe",
			"386":    "cloudflared-windows-386.exe",
		},
	}
}

func platformFallbacks() map[string]string {
	return map[string]string{
		"darwin":  "cloudflared-darwin-amd64.tgz",
		"linux":   "cloudflared-linux-amd64",
		"windows": "cloudflared-windows-amd64.exe",
	}
}

// IsValidBinary reports whether path looks like a real cloudflared
// executable: at least MinBinarySize bytes and a recognised magic header.
func IsValidBinary(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Size() < MinBinarySize {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	var head [4]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return false
	}
	return hasMatchingMagic(head[:], runtime.GOOS)
}

func hasMatchingMagic(head []byte, goos string) bool {
	elf := []byte{0x7f, 0x45, 0x4c, 0x46}
	pe := []byte{0x4d, 0x5a}
	machoLE := []byte{0xce, 0xfa, 0xed, 0xfe}
	machoBE := []byte{0xcf, 0xfa, 0xed, 0xfe}

	switch goos {
	case "windows":
		return len(head) >= 2 && head[0] == pe[0] && head[1] == pe[1]
	case "darwin":
		return len(head) >= 4 && (equalBytes(head[:4], machoLE) || equalBytes(head[:4], machoBE))
	default: // linux + others
		return len(head) >= 4 && equalBytes(head[:4], elf)
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// EnsureCloudflared guarantees a valid cloudflared binary exists on disk.
// It is concurrency-safe — parallel callers share a single in-flight
// download. progressFn is invoked with 0-100 during download (may be nil).
func (m *Manager) EnsureCloudflared(ctx context.Context, progressFn func(int)) (string, error) {
	// Fast path: existing binary is valid.
	if IsValidBinary(m.binPath) {
		_ = m.ensureExecutable()
		return m.binPath, nil
	}

	// Coalesce concurrent downloads behind a singleflight-style gate.
	m.mu.Lock()
	if m.downloadInFlight {
		m.mu.Unlock()
		// Poll until the in-flight download finishes; respect ctx.
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-ticker.C:
				m.mu.Lock()
				done := !m.downloadInFlight
				m.mu.Unlock()
				if done {
					if IsValidBinary(m.binPath) {
						_ = m.ensureExecutable()
						return m.binPath, nil
					}
					return "", errors.New("cloudflared: download did not produce a valid binary")
				}
			}
		}
	}
	m.downloadInFlight = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.downloadInFlight = false
		m.mu.Unlock()
	}()

	if err := os.MkdirAll(m.binDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir bin dir: %w", err)
	}
	// Clean up stale temp files from a previous crash.
	tmpPath := m.binPath + ".tmp"
	if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove stale tmp: %w", err)
	}

	goos, arch := runtime.GOOS, runtime.GOARCH
	url := DownloadURLFor(goos, arch)
	if url == "" {
		return "", fmt.Errorf("cloudflared: unsupported platform %s/%s", goos, arch)
	}

	isArchive := strings.HasSuffix(url, ".tgz")
	dest := tmpPath
	if isArchive {
		dest = filepath.Join(m.binDir, "cloudflared.tgz.tmp")
	}
	if err := m.download(ctx, url, dest, progressFn); err != nil {
		return "", err
	}

	if isArchive {
		if err := extractTGZ(dest, m.binDir); err != nil {
			_ = os.Remove(dest)
			return "", fmt.Errorf("extract tgz: %w", err)
		}
		_ = os.Remove(dest)
		// tarball extracts to ./cloudflared; move into place.
		extracted := filepath.Join(m.binDir, "cloudflared")
		if runtime.GOOS == "windows" {
			extracted += ".exe"
		}
		if extracted != m.binPath {
			if err := os.Rename(extracted, m.binPath); err != nil {
				return "", fmt.Errorf("place binary: %w", err)
			}
		}
	} else {
		if err := os.Rename(dest, m.binPath); err != nil {
			return "", fmt.Errorf("place binary: %w", err)
		}
	}

	if !IsValidBinary(m.binPath) {
		_ = os.Remove(m.binPath)
		return "", errors.New("cloudflared: downloaded file failed validation")
	}
	if err := m.ensureExecutable(); err != nil {
		return "", err
	}
	return m.binPath, nil
}

func (m *Manager) ensureExecutable() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Chmod(m.binPath, 0o755)
}

func (m *Manager) download(ctx context.Context, url, dest string, progressFn func(int)) error {
	m.setDownload(true, 0)
	defer m.setDownload(false, 0)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer out.Close()

	var reader io.Reader = resp.Body
	if progressFn != nil {
		total := parseContentLength(resp.Header.Get("Content-Length"))
		reader = &progressReader{
			inner: resp.Body,
			total: total,
			cb:    progressFn,
		}
	}

	if _, err := io.Copy(out, reader); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("write body: %w", ctxOrErr(ctx, err))
	}
	if err := out.Sync(); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("sync: %w", err)
	}
	if progressFn != nil {
		progressFn(100)
	}
	return nil
}

func (m *Manager) setDownload(active bool, progress int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.downloadState.Downloading = active
	if active {
		m.downloadState.Progress = progress
	} else {
		m.downloadState.Progress = 0
	}
}

func parseContentLength(s string) int64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

type progressReader struct {
	inner   io.Reader
	total   int64
	current int64
	cb      func(int)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.inner.Read(b)
	if n > 0 {
		p.current += int64(n)
		if p.total > 0 {
			pct := int(p.current * 100 / p.total)
			if pct > 100 {
				pct = 100
			}
			p.cb(pct)
		}
	}
	return n, err
}

// extractTGZ extracts a .tgz archive containing cloudflared into destDir.
func extractTGZ(archive, destDir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// Only extract the cloudflared binary itself; skip dirs and other entries.
		base := filepath.Base(hdr.Name)
		want := "cloudflared"
		if runtime.GOOS == "windows" {
			want = "cloudflared.exe"
		}
		if base != want {
			continue
		}
		out, err := os.OpenFile(filepath.Join(destDir, base), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}

// ctxOrErr returns ctx.Err() when the context is cancelled, otherwise
// the original error — keeps error attribution clean for callers.
func ctxOrErr(ctx context.Context, fallback error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fallback
}

// pidFilePath is exported for tests.
func (m *Manager) pidFilePath() string { return m.currentPIDFile }

// savePID writes the PID of child to {dataDir}/tunnel/cloudflared.pid so
// the process can be reaped on a subsequent start.
func (m *Manager) savePID(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(m.currentPIDFile), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(m.currentPIDFile, []byte(strconv.Itoa(pid)), 0o644)
}

// loadPID returns the PID previously saved by savePID, or 0 if none.
func (m *Manager) loadPID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := os.ReadFile(m.currentPIDFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// clearPID removes the PID file.
func (m *Manager) clearPID() {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = os.Remove(m.currentPIDFile)
}

// IsCloudflaredRunning reports whether a previously-saved PID is still
// alive. It does NOT start a process itself; callers should combine this
// with knowledge of any in-process child they own.
func (m *Manager) IsCloudflaredRunning() bool {
	pid := m.loadPID()
	if pid == 0 {
		return false
	}
	return processAlive(pid)
}

// processAlive sends signal 0 to pid and reports whether it succeeded.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

// SpawnQuickTunnel starts `cloudflared tunnel --url http://127.0.0.1:port`
// and reports the discovered public URL via onUrlUpdate. The returned
// channel emits the first observed URL once it is detected. The exec.Cmd
// is returned for later cleanup via KillCloudflared.
//
// Behaviour:
//   - Ensures the binary is on disk (downloads if needed).
//   - Watches stdout/stderr for the trycloudflare.com URL.
//   - Calls onUrlUpdate whenever the URL changes after the first
//     detection (cloudflared can rotate URLs on reconnect).
//   - Returns ctx.Err() if the context is cancelled before the first URL.
func (m *Manager) SpawnQuickTunnel(ctx context.Context, port int, onUrlUpdate func(string)) (*exec.Cmd, string, error) {
	binPath, err := m.EnsureCloudflared(ctx, nil)
	if err != nil {
		return nil, "", err
	}

	// Use a temp config dir so we don't trample ~/.cloudflared/config.yml.
	configDir, err := os.MkdirTemp("", "cloudflared-quick-")
	if err != nil {
		return nil, "", fmt.Errorf("config dir: %w", err)
	}
	configPath := filepath.Join(configDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("# quick-tunnel placeholder\n"), 0o644); err != nil {
		_ = os.RemoveAll(configDir)
		return nil, "", fmt.Errorf("write config: %w", err)
	}

	// Detach into its own process group so we can signal the whole tree.
	cmd := exec.CommandContext(ctx, binPath,
		"tunnel", "--url", fmt.Sprintf("http://127.0.0.1:%d", port),
		"--config", configPath,
		"--no-autoupdate",
		"--retries", "99",
	)
	cmd.Dir = os.TempDir()
	cmd.SysProcAttr = processGroupAttr()
	cmd.Env = append(os.Environ(), "TUNNEL_TRANSPORT_PROTOCOL="+envProtocol())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(configDir)
		return nil, "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = os.RemoveAll(configDir)
		return nil, "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(configDir)
		return nil, "", fmt.Errorf("start cloudflared: %w", err)
	}
	m.mu.Lock()
	m.currentProcess = cmd
	m.mu.Unlock()
	m.savePID(cmd.Process.Pid)

	// Watch for the first URL; return it. Subsequent URLs flow through
	// onUrlUpdate.
	type result struct {
		url string
		err error
	}
	done := make(chan result, 1)

	go scanURLs(stdout, stderr, func(url string, first bool) {
		if first {
			select {
			case done <- result{url: url}:
			default:
			}
		} else if onUrlUpdate != nil {
			onUrlUpdate(url)
		}
	})

	cleanup := func() {
		_ = os.RemoveAll(configDir)
	}

	timeout := DefaultQuickTunnelTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining < timeout {
			timeout = remaining
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// Reap the child asynchronously so cmd.Wait() doesn't block this
	// function forever — KillCloudflared will handle final cleanup.
	go func() {
		_ = cmd.Wait()
		m.clearPID()
		cleanup()
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		cleanup()
		return cmd, "", ctx.Err()
	case <-timer.C:
		_ = cmd.Process.Kill()
		cleanup()
		return cmd, "", fmt.Errorf("cloudflared: timed out waiting for quick tunnel URL after %s", timeout)
	case r := <-done:
		return cmd, r.url, nil
	}
}

// envProtocol honours TUNNEL_TRANSPORT_PROTOCOL (http2/quic/auto), defaulting to http2.
func envProtocol() string {
	if v := os.Getenv("TUNNEL_TRANSPORT_PROTOCOL"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "quic", "auto", "http2":
			return strings.ToLower(v)
		}
	}
	return "http2"
}

// scanURLs reads line-by-line from both pipes, calling cb for each new
// trycloudflare.com URL. The first callback is "first=true", later ones
// fire only when the URL differs from the last one seen. Returns when
// both readers have hit EOF.
func scanURLs(stdout, stderr io.Reader, cb func(url string, first bool)) {
	var (
		mu     sync.Mutex
		first  = true
		latest string
	)
	process := func(b []byte) {
		matches := quickTunnelURLRegex.FindAll(b, -1)
		if len(matches) == 0 {
			return
		}
		var url string
		for _, m := range matches {
			candidate := string(m)
			if !strings.Contains(candidate, "://api.") {
				url = candidate
			}
		}
		if url == "" {
			return
		}
		mu.Lock()
		emitFirst := first
		emitUpdate := !first && url != latest
		latest = url
		first = false
		mu.Unlock()
		if emitFirst {
			cb(url, true)
		} else if emitUpdate {
			cb(url, false)
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = copyAndProcess(stdout, process) }()
	go func() { defer wg.Done(); _ = copyAndProcess(stderr, process) }()
	wg.Wait()
}

func copyAndProcess(r io.Reader, process func([]byte)) error {
	buf := make([]byte, 4096)
	var carry []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := append(carry, buf[:n]...)
			last := chunk
			// Process complete lines only; carry partial forward.
			for {
				idx := -1
				for i, b := range chunk {
					if b == '\n' {
						idx = i
						break
					}
				}
				if idx < 0 {
					break
				}
				line := chunk[:idx]
				process(line)
				chunk = chunk[idx+1:]
				last = chunk
			}
			carry = last
		}
		if err != nil {
			if len(carry) > 0 {
				process(carry)
			}
			return err
		}
	}
}

// KillCloudflared terminates the running cloudflared process. It tries:
//  1. The in-memory *exec.Cmd (if any)
//  2. The PID file
//  3. A port-based pkill fallback (Linux/macOS only)
//
// All three are best-effort — callers should treat the call as a
// "make sure it's gone" request, not a transactional operation.
func (m *Manager) KillCloudflared(port int) error {
	m.mu.Lock()
	cmd := m.currentProcess
	m.currentProcess = nil
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = killProcessGroup(cmd.Process.Pid)
	}

	if pid := m.loadPID(); pid != 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		// Give it a moment, then SIGKILL.
		time.Sleep(200 * time.Millisecond)
		if processAlive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		m.clearPID()
	}

	if port > 0 && runtime.GOOS != "windows" {
		_ = killByPort(port)
	}
	return nil
}

// killByPort invokes pkill -f against any process whose command line
// contains cloudflared and the given port (with a non-digit boundary).
// Errors are intentionally swallowed — this is a best-effort cleanup.
func killByPort(port int) error {
	pattern := fmt.Sprintf("cloudflared.*:%d([^0-9]|$)", port)
	// Use -SIGKILL for force termination; ignore exit code (no matches = 1).
	cmd := exec.Command("pkill", "-f", pattern)
	cmd.SysProcAttr = processGroupAttr()
	return cmd.Run()
}
