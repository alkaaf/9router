package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/9router/9router/internal/tunnel"
)

// SettingsStore is the subset of the settings repository the manager
// needs. The DB layer (not yet written) will satisfy this interface; for
// now callers wire it in.
type SettingsStore interface {
	GetTunnelEnabled() (bool, string, error)
	SetTunnelEnabled(enabled bool, tunnelURL string) error
}

// HealthChecker is implemented by the health package; we accept an
// interface here to keep this package free of network dependencies for
// unit tests.
type HealthChecker interface {
	ProbeURL(ctx context.Context, rawURL string) (bool, error)
	WaitForHealth(ctx context.Context, rawURL string, timeout time.Duration) error
}

// WorkerRegisterer is implemented by the worker package; same rationale
// as HealthChecker.
type WorkerRegisterer interface {
	Register(ctx context.Context, workerURL, shortID, tunnelURL string) error
}

// ManagerResult is the result of Enable. Mirrors the TunnelResult in the
// task spec, kept here to avoid an import cycle with internal/tunnel.
type ManagerResult struct {
	Success        bool   `json:"success"`
	TunnelURL      string `json:"tunnelUrl"`
	ShortID        string `json:"shortId"`
	PublicURL      string `json:"publicUrl"`
	AlreadyRunning bool   `json:"alreadyRunning"`
}

// ManagerStatus is the result of GetStatus.
type ManagerStatus struct {
	Enabled         bool   `json:"enabled"`
	SettingsEnabled bool   `json:"settingsEnabled"`
	TunnelURL       string `json:"tunnelUrl"`
	ShortID         string `json:"shortId"`
	PublicURL       string `json:"publicUrl"`
	Running         bool   `json:"running"`
}

// TunnelManager orchestrates the Cloudflare quick-tunnel lifecycle.
type TunnelManager struct {
	cfg       tunnel.TunnelConfig
	state     *tunnel.StateManager
	binary    *Manager
	health    HealthChecker
	worker    WorkerRegisterer
	settings  SettingsStore
	httpClient *http.Client

	mu          sync.Mutex
	enableInFlight bool
}

// NewTunnelManager wires together a manager with its dependencies.
func NewTunnelManager(
	cfg tunnel.TunnelConfig,
	state *tunnel.StateManager,
	binary *Manager,
	health HealthChecker,
	worker WorkerRegisterer,
	settings SettingsStore,
) *TunnelManager {
	return &TunnelManager{
		cfg:        cfg,
		state:      state,
		binary:     binary,
		health:     health,
		worker:     worker,
		settings:   settings,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// EnableInput carries the parameters to Enable.
type EnableInput struct {
	Context   context.Context
	LocalPort int
}

// DisableInput carries the parameters to Disable.
type DisableInput struct {
	Context context.Context
}

// Enable starts a Cloudflare quick-tunnel. If a healthy tunnel is already
// running, returns AlreadyRunning. If a stale process is found, kills it
// first.
//
// Steps:
//  1. Probe current state.json URL + cloudflared PID.
//  2. If healthy, return AlreadyRunning.
//  3. Otherwise kill any existing cloudflared.
//  4. Ensure binary is on disk.
//  5. Generate new shortID and spawn quick tunnel.
//  6. Persist state (shortId, tunnelUrl).
//  7. Update settings (tunnelEnabled=true).
//  8. Register URL with worker (non-fatal on failure).
//  9. Wait for health (60s timeout, non-fatal on timeout — caller may
//     still want to surface the URL while DNS propagates).
func (m *TunnelManager) Enable(in EnableInput) (ManagerResult, error) {
	if in.Context == nil {
		return ManagerResult{}, errors.New("cloudflare: nil context")
	}
	if in.LocalPort <= 0 {
		in.LocalPort = m.cfg.LocalPort
	}
	if in.LocalPort <= 0 {
		in.LocalPort = 20128
	}

	// Coalesce concurrent Enable calls.
	m.mu.Lock()
	if m.enableInFlight {
		m.mu.Unlock()
		return ManagerResult{}, errors.New("cloudflare: enable already in progress")
	}
	m.enableInFlight = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.enableInFlight = false
		m.mu.Unlock()
	}()

	// 1. Check existing state.
	state, err := m.state.LoadState()
	if err != nil {
		return ManagerResult{}, fmt.Errorf("load state: %w", err)
	}

	// 2. If we have a URL and process, probe it before deciding.
	if state.TunnelURL != "" && m.binary.IsCloudflaredRunning() {
		probeCtx, cancel := context.WithTimeout(in.Context, 3*time.Second)
		alive, _ := m.health.ProbeURL(probeCtx, state.TunnelURL)
		cancel()
		if alive {
			return ManagerResult{
				Success:        true,
				TunnelURL:      state.TunnelURL,
				ShortID:        state.ShortID,
				PublicURL:      buildPublicURL(m.cfg.WorkerURL, state.ShortID),
				AlreadyRunning: true,
			}, nil
		}
	}

	// 3. Kill any stale cloudflared (no URL match, or process not alive).
	if err := m.binary.KillCloudflared(in.LocalPort); err != nil {
		return ManagerResult{}, fmt.Errorf("kill stale: %w", err)
	}

	// 4. Ensure binary is on disk.
	if _, err := m.binary.EnsureCloudflared(in.Context, nil); err != nil {
		return ManagerResult{}, fmt.Errorf("ensure binary: %w", err)
	}

	// 5. Generate new shortID + spawn.
	shortID := tunnel.GenerateShortID()
	if shortID == "" {
		return ManagerResult{}, errors.New("cloudflare: failed to generate short id")
	}

	cmd, tunnelURL, err := m.binary.SpawnQuickTunnel(in.Context, in.LocalPort, nil)
	if err != nil {
		return ManagerResult{}, fmt.Errorf("spawn tunnel: %w", err)
	}
	_ = cmd // kept for KillCloudflared via PID file

	// 6. Persist state.
	if err := m.state.SaveState(tunnel.TunnelState{
		ShortID:   shortID,
		TunnelURL: tunnelURL,
	}); err != nil {
		// Best-effort: we still want the user to see the URL.
		// But the process is alive; just log via error chain.
		// Kill before returning so we don't leak a process with no state.
		_ = m.binary.KillCloudflared(in.LocalPort)
		return ManagerResult{}, fmt.Errorf("save state: %w", err)
	}

	// 7. Update settings.
	if m.settings != nil {
		_ = m.settings.SetTunnelEnabled(true, tunnelURL)
	}

	publicURL := buildPublicURL(m.cfg.WorkerURL, shortID)

	// 8. Register with worker (non-fatal).
	if m.worker != nil && m.cfg.WorkerURL != "" {
		regCtx, cancel := context.WithTimeout(in.Context, 5*time.Second)
		_ = m.worker.Register(regCtx, m.cfg.WorkerURL, shortID, tunnelURL)
		cancel()
	}

	// 9. Wait for health (60s timeout, non-fatal).
	healthCtx, cancel := context.WithTimeout(in.Context, 60*time.Second)
	defer cancel()
	_ = m.health.WaitForHealth(healthCtx, tunnelURL, 60*time.Second)

	return ManagerResult{
		Success:        true,
		TunnelURL:      tunnelURL,
		ShortID:        shortID,
		PublicURL:      publicURL,
		AlreadyRunning: false,
	}, nil
}

// Disable stops the running Cloudflare tunnel.
func (m *TunnelManager) Disable(in DisableInput) error {
	if in.Context == nil {
		return errors.New("cloudflare: nil context")
	}

	port := m.cfg.LocalPort
	if port <= 0 {
		port = 20128
	}

	// Kill process. Don't propagate the kill error: caller wants the
	// state cleared regardless.
	_ = m.binary.KillCloudflared(port)

	// Clear tunnel URL but preserve shortID.
	state, err := m.state.LoadState()
	if err == nil && state.ShortID != "" {
		_ = m.state.SaveState(tunnel.TunnelState{
			ShortID:   state.ShortID,
			TunnelURL: "",
		})
	} else {
		_ = m.state.ClearState()
	}

	// Update settings.
	if m.settings != nil {
		_ = m.settings.SetTunnelEnabled(false, "")
	}

	// Honour cancellation: we can't actually interrupt the above
	// syscalls, but we can refuse to no-op if the context is done
	// and the caller wants strict cancellation semantics.
	select {
	case <-in.Context.Done():
		return in.Context.Err()
	default:
		return nil
	}
}

// GetStatus returns the current state of the Cloudflare tunnel.
func (m *TunnelManager) GetStatus(ctx context.Context) (ManagerStatus, error) {
	if ctx == nil {
		return ManagerStatus{}, errors.New("cloudflare: nil context")
	}

	state, err := m.state.LoadState()
	if err != nil {
		return ManagerStatus{}, fmt.Errorf("load state: %w", err)
	}

	running := m.binary.IsCloudflaredRunning()
	settingsEnabled := false
	settingsURL := ""
	if m.settings != nil {
		settingsEnabled, settingsURL, _ = m.settings.GetTunnelEnabled()
	}

	// Probe if we think we're running, but only briefly.
	if running && state.TunnelURL != "" {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		alive, _ := m.health.ProbeURL(probeCtx, state.TunnelURL)
		cancel()
		if !alive {
			running = false
		}
	}

	status := ManagerStatus{
		Enabled:         running,
		SettingsEnabled: settingsEnabled,
		TunnelURL:       state.TunnelURL,
		ShortID:         state.ShortID,
		PublicURL:       buildPublicURL(m.cfg.WorkerURL, state.ShortID),
		Running:         running,
	}
	if settingsURL != "" && status.TunnelURL == "" {
		status.TunnelURL = settingsURL
	}
	return status, nil
}

// buildPublicURL composes the worker-redirected URL for a given short ID.
// It is intentionally lenient: invalid WorkerURLs are tolerated and the
// resulting PublicURL is left empty.
func buildPublicURL(workerURL, shortID string) string {
	if workerURL == "" || shortID == "" {
		return ""
	}
	u, err := url.Parse(workerURL)
	if err != nil || u.Host == "" {
		return ""
	}
	// Insert "r<shortid>" subdomain in front of the existing host,
	// preserving any port suffix.
	host := u.Host
	u.Host = "r" + shortID + "." + host
	return u.String()
}

// httpJSONPost is a small helper for tests and ad-hoc callers.
func httpJSONPost(ctx context.Context, client *http.Client, endpoint string, body interface{}) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// Drain for connection reuse.
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("worker: HTTP %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
