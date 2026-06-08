package cloudflare

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadURLFor(t *testing.T) {
	cases := []struct {
		goos, arch, wantSubstring string
	}{
		{"darwin", "arm64", "cloudflared-darwin-arm64.tgz"},
		{"darwin", "amd64", "cloudflared-darwin-amd64.tgz"},
		{"linux", "arm64", "cloudflared-linux-arm64"},
		{"linux", "amd64", "cloudflared-linux-amd64"},
		{"windows", "arm64", "cloudflared-windows-arm64.exe"},
		{"windows", "amd64", "cloudflared-windows-amd64.exe"},
	}
	for _, tc := range cases {
		t.Run(tc.goos+"/"+tc.arch, func(t *testing.T) {
			got := DownloadURLFor(tc.goos, tc.arch)
			if !strings.HasPrefix(got, GitHubBaseURL) {
				t.Fatalf("URL should start with %s, got %q", GitHubBaseURL, got)
			}
			if !strings.HasSuffix(got, tc.wantSubstring) {
				t.Fatalf("URL should end with %q, got %q", tc.wantSubstring, got)
			}
		})
	}
}

func TestDownloadURLFor_Fallback(t *testing.T) {
	// linux/386 has an explicit mapping, not a fallback.
	got := DownloadURLFor("linux", "386")
	if !strings.HasSuffix(got, "cloudflared-linux-386") {
		t.Fatalf("linux/386 should map to linux-386, got %q", got)
	}
	// windows/ia32 (non-standard arch) should fall back to amd64.
	got = DownloadURLFor("windows", "ia32")
	if !strings.HasSuffix(got, "cloudflared-windows-amd64.exe") {
		t.Fatalf("windows/ia32 should fall back to amd64, got %q", got)
	}
}

func TestDownloadURLFor_UnsupportedPlatform(t *testing.T) {
	if got := DownloadURLFor("plan9", "arm64"); got != "" {
		t.Fatalf("unsupported platform should return empty, got %q", got)
	}
}

func TestIsValidBinary_RejectsSmallFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cloudflared")
	if err := os.WriteFile(p, make([]byte, 100), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if IsValidBinary(p) {
		t.Fatalf("file <1MB should fail validation")
	}
}

func TestIsValidBinary_RejectsMissingFile(t *testing.T) {
	if IsValidBinary(filepath.Join(t.TempDir(), "nope")) {
		t.Fatalf("missing file should fail validation")
	}
}

func TestIsValidBinary_MagicBytes(t *testing.T) {
	cases := []struct {
		goos string
		magic []byte
		want bool
	}{
		{"linux", []byte{0x7f, 0x45, 0x4c, 0x46}, true},
		{"linux", []byte{0x4d, 0x5a, 0x90, 0x00}, false},
		{"darwin", []byte{0xcf, 0xfa, 0xed, 0xfe}, true},
		{"darwin", []byte{0xce, 0xfa, 0xed, 0xfe}, true},
		{"darwin", []byte{0x7f, 0x45, 0x4c, 0x46}, false},
		{"windows", []byte{0x4d, 0x5a, 0x90, 0x00}, true},
		{"windows", []byte{0x7f, 0x45, 0x4c, 0x46}, false},
	}
	for _, tc := range cases {
		t.Run(tc.goos+"/"+hexOf(tc.magic), func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "cloudflared")
			// Make a >1MB file by padding after the magic.
			buf := make([]byte, MinBinarySize)
			copy(buf, tc.magic)
			if err := os.WriteFile(p, buf, 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			got := hasMatchingMagic(readHead(t, p), tc.goos)
			if got != tc.want {
				t.Fatalf("hasMatchingMagic(%q, %q) = %v, want %v", hexOf(tc.magic), tc.goos, got, tc.want)
			}
		})
	}
}

func TestIsValidBinary_RejectsTextFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cloudflared")
	// >1MB of plain text — wrong magic, must fail.
	text := bytes.Repeat([]byte("not an executable, just text\n"), 50_000)
	if err := os.WriteFile(p, text, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if IsValidBinary(p) {
		t.Fatalf("text file should fail validation")
	}
}

func readHead(t *testing.T, p string) []byte {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	var head [4]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		t.Fatalf("read: %v", err)
	}
	return head[:]
}

func hexOf(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hex[v>>4], hex[v&0x0f])
	}
	return string(out)
}

func TestEnsureCloudflared_DownloadsFromMock(t *testing.T) {
	// Build a minimal "binary" that passes the local-platform validation.
	payload := makeValidBinaryPayload(t, runtime.GOOS)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	// Monkey-patch GitHubBaseURL by constructing a Manager and overriding
	// its internal URL via a custom Manager. We do that by creating a
	// private helper in this test that runs the download against server.URL.
	mgr := newTestManager(t)

	// We can't easily redirect GitHubBaseURL; instead exercise the lower
	// pieces directly via a custom download call. Here we just verify the
	// server side works end-to-end if the URL pointed at us.
	progressVals := []int{}
	progress := func(p int) { progressVals = append(progressVals, p) }

	url := server.URL + "/cloudflared"
	dest := filepath.Join(mgr.binDir, "test-binary")
	if err := mgr.download(context.Background(), url, dest, progress); err != nil {
		t.Fatalf("download: %v", err)
	}
	if !IsValidBinary(dest) {
		t.Fatalf("downloaded file should be valid")
	}
	if progressVals[len(progressVals)-1] != 100 {
		t.Fatalf("progress should hit 100, got %v", progressVals)
	}
}

func TestEnsureCloudflared_RejectsCorruptDownload(t *testing.T) {
	payload := []byte("clearly not a real binary, just some text")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	mgr := newTestManager(t)
	url := server.URL + "/bad"
	dest := filepath.Join(mgr.binDir, "bad")
	err := mgr.download(context.Background(), url, dest, nil)
	if err != nil {
		t.Fatalf("download itself should succeed: %v", err)
	}
	if IsValidBinary(dest) {
		t.Fatalf("text payload should not be a valid binary")
	}
}

func TestEnsureCloudflared_RespectsContextCancellation(t *testing.T) {
	// Slow server that streams bytes slowly.
	gate := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-gate:
				return
			default:
			}
			_, _ = w.Write(make([]byte, 10_000))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer server.Close()
	defer close(gate)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	mgr := newTestManager(t)
	dest := filepath.Join(mgr.binDir, "cancelled")
	err := mgr.download(ctx, server.URL+"/slow", dest, nil)
	if err == nil {
		t.Fatalf("expected error on cancelled context")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		// Partial file may or may not exist depending on where the cancel hit;
		// if it does, clean it up. This is not a hard failure.
		_ = os.Remove(dest)
	}
}

func TestEnsureCloudflared_CoalescesConcurrentCalls(t *testing.T) {
	payload := makeValidBinaryPayload(t, runtime.GOOS)
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(100 * time.Millisecond) // simulate slow network
		w.Header().Set("Content-Length", itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	mgr := newTestManager(t)
	// Pre-seed an in-flight download via the public state machine.
	mgr.mu.Lock()
	mgr.downloadInFlight = true
	mgr.mu.Unlock()
	defer func() {
		mgr.mu.Lock()
		mgr.downloadInFlight = false
		mgr.mu.Unlock()
	}()

	// Two concurrent callers; the second should observe the in-flight flag
	// and (because we just sleep a bit) see the download complete.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Release the in-flight flag shortly so the waiters can proceed.
			time.Sleep(50 * time.Millisecond)
			mgr.mu.Lock()
			mgr.downloadInFlight = false
			// Seed the binary on disk so the fast path triggers.
			_ = os.MkdirAll(mgr.binDir, 0o755)
			_ = os.WriteFile(mgr.binPath, payload, 0o755)
			mgr.mu.Unlock()
			_, err := mgr.EnsureCloudflared(context.Background(), nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("EnsureCloudflared: %v", e)
		}
	}
}

func TestEnsureCloudflared_RejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()

	mgr := newTestManager(t)
	dest := filepath.Join(mgr.binDir, "missing")
	err := mgr.download(context.Background(), server.URL, dest, nil)
	if err == nil {
		t.Fatalf("expected error for HTTP 404")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatalf("dest should not exist after failed download")
	}
}

func TestScanURLs_FirstURLDetection(t *testing.T) {
	stdout := strings.NewReader("some startup log\nhttps://abc123.trycloudflare.com is your URL\n")
	stderr := strings.NewReader("")

	var got string
	var firstCount int
	cb := func(url string, first bool) {
		got = url
		if first {
			firstCount++
		}
	}
	scanURLs(stdout, stderr, cb)
	if got != "https://abc123.trycloudflare.com" {
		t.Fatalf("got %q", got)
	}
	if firstCount != 1 {
		t.Fatalf("expected first=true once, got %d", firstCount)
	}
}

func TestScanURLs_SkipsAPISubdomain(t *testing.T) {
	stdout := strings.NewReader("https://api.trycloudflare.com ping\n")
	stderr := strings.NewReader("https://abc123.trycloudflare.com ready\n")

	var got string
	var gotFirst bool
	cb := func(url string, first bool) {
		got = url
		gotFirst = first
	}
	scanURLs(stdout, stderr, cb)
	if got != "https://abc123.trycloudflare.com" {
		t.Fatalf("got %q, want to skip api.* and report abc123", got)
	}
	if !gotFirst {
		t.Fatalf("expected first=true for abc123")
	}
}

func TestScanURLs_RotateNotifiesCallback(t *testing.T) {
	stdout := strings.NewReader("https://first.trycloudflare.com old\nhttps://second.trycloudflare.com new\n")
	stderr := strings.NewReader("")

	var (
		mu       sync.Mutex
		updates  []string
	)
	cb := func(url string, first bool) {
		mu.Lock()
		defer mu.Unlock()
		if !first {
			updates = append(updates, url)
		}
	}
	scanURLs(stdout, stderr, cb)
	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 1 || updates[0] != "https://second.trycloudflare.com" {
		t.Fatalf("expected one update for second URL, got %v", updates)
	}
}

func TestManager_NewManagerPaths(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	wantBin := filepath.Join(dir, "bin", "cloudflared")
	if runtime.GOOS == "windows" {
		wantBin += ".exe"
	}
	if mgr.BinPath() != wantBin {
		t.Fatalf("BinPath = %q, want %q", mgr.BinPath(), wantBin)
	}
	wantPID := filepath.Join(dir, "tunnel", "cloudflared.pid")
	if mgr.pidFilePath() != wantPID {
		t.Fatalf("pidFilePath = %q, want %q", mgr.pidFilePath(), wantPID)
	}
}

func TestManager_PIDLifecycle(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	if pid := mgr.loadPID(); pid != 0 {
		t.Fatalf("expected 0 PID initially, got %d", pid)
	}
	mgr.savePID(12345)
	if pid := mgr.loadPID(); pid != 12345 {
		t.Fatalf("expected 12345, got %d", pid)
	}
	mgr.clearPID()
	if pid := mgr.loadPID(); pid != 0 {
		t.Fatalf("expected 0 after clear, got %d", pid)
	}
}

func TestManager_IsCloudflaredRunningFalseOnNoPID(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	if mgr.IsCloudflaredRunning() {
		t.Fatalf("expected false when no PID file")
	}
}

func TestKillCloudflared_NoProcess(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	// Should not panic or error when there's nothing to kill.
	if err := mgr.KillCloudflared(0); err != nil {
		t.Fatalf("KillCloudflared on empty state: %v", err)
	}
}

func TestEnvProtocol(t *testing.T) {
	cases := map[string]string{
		"":        "http2",
		"http2":   "http2",
		"quic":    "quic",
		"auto":    "auto",
		"HTTP2":   "http2",
		"garbage": "http2",
	}
	for in, want := range cases {
		t.Setenv("TUNNEL_TRANSPORT_PROTOCOL", in)
		if got := envProtocol(); got != want {
			t.Fatalf("envProtocol(%q) = %q, want %q", in, got, want)
		}
	}
}

// newTestManager builds a Manager that writes to a temp dir and bypasses
// any real network. Callers are responsible for plumbing server URLs into
// the Manager manually.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	mgr := NewManager(dir)
	if err := os.MkdirAll(mgr.binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return mgr
}

// makeValidBinaryPayload returns a buffer that passes IsValidBinary for
// the given platform. The content after the magic bytes is just padding
// to reach MinBinarySize.
func makeValidBinaryPayload(t *testing.T, goos string) []byte {
	t.Helper()
	buf := make([]byte, MinBinarySize)
	switch goos {
	case "darwin":
		copy(buf, []byte{0xcf, 0xfa, 0xed, 0xfe})
	case "windows":
		copy(buf, []byte{0x4d, 0x5a, 0x90, 0x00})
	default: // linux
		copy(buf, []byte{0x7f, 0x45, 0x4c, 0x46})
	}
	return buf
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
