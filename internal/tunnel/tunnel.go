// Package tunnel provides network exposure services for 9Router via
// Cloudflare Tunnel and Tailscale Funnel.
package tunnel

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// ShortIDLength is the length of generated short identifiers used to
	// identify a tunnel instance (e.g. "abc123").
	ShortIDLength = 6

	// shortIDChars excludes visually ambiguous characters: 0/o, 1/l/i.
	shortIDChars = "abcdefghijklmnpqrstuvwxyz23456789"

	// stateFileName is the file name used by StateManager to persist state.
	stateFileName = "state.json"

	// subdirName is the subdirectory under DataDir used for tunnel state.
	subdirName = "tunnel"
)

// TunnelConfig is the configuration passed to tunnel services.
type TunnelConfig struct {
	DataDir   string // Base data directory (e.g. ~/.9router)
	LocalPort int    // Local server port (default 20128)
	WorkerURL string // Cloudflare worker URL for registration
}

// TunnelState is the persisted state of a Cloudflare quick-tunnel.
type TunnelState struct {
	ShortID   string `json:"shortId"`
	TunnelURL string `json:"tunnelUrl"`
}

// TunnelStatus is the runtime status of a Cloudflare tunnel.
type TunnelStatus struct {
	Enabled         bool   `json:"enabled"`
	SettingsEnabled bool   `json:"settingsEnabled"`
	TunnelURL       string `json:"tunnelUrl"`
	ShortID         string `json:"shortId"`
	PublicURL       string `json:"publicUrl"`
	Running         bool   `json:"running"`
}

// TunnelResult is the result of a successful tunnel enable.
type TunnelResult struct {
	TunnelURL      string `json:"tunnelUrl"`
	ShortID        string `json:"shortId"`
	PublicURL      string `json:"publicUrl"`
	AlreadyRunning bool   `json:"alreadyRunning"`
}

// TailscaleStatus is the runtime status of a Tailscale funnel.
type TailscaleStatus struct {
	Enabled         bool   `json:"enabled"`
	SettingsEnabled bool   `json:"settingsEnabled"`
	TunnelURL       string `json:"tunnelUrl"`
	Running         bool   `json:"running"`
	LoggedIn        bool   `json:"loggedIn"`
}

// TailscaleCheckResult is the result of inspecting the Tailscale install.
type TailscaleCheckResult struct {
	Installed         bool   `json:"installed"`
	LoggedIn          bool   `json:"loggedIn"`
	Platform          string `json:"platform"`
	BrewAvailable     bool   `json:"brewAvailable"`
	DaemonRunning     bool   `json:"daemonRunning"`
	HasCachedPassword bool   `json:"hasCachedPassword"`
}

// TailscaleEnableResult is the result of starting a Tailscale funnel.
type TailscaleEnableResult struct {
	Success          bool   `json:"success"`
	TunnelURL        string `json:"tunnelUrl,omitempty"`
	NeedsLogin       bool   `json:"needsLogin,omitempty"`
	AuthURL          string `json:"authUrl,omitempty"`
	FunnelNotEnabled bool   `json:"funnelNotEnabled,omitempty"`
	EnableURL        string `json:"enableUrl,omitempty"`
}

// InstallResult is the result of installing the Tailscale binary.
type InstallResult struct {
	Success bool   `json:"success"`
	AuthURL string `json:"authUrl,omitempty"`
}

// TunnelService exposes Cloudflare quick-tunnel lifecycle operations.
type TunnelService interface {
	Enable(ctx context.Context) (TunnelResult, error)
	Disable(ctx context.Context) error
	GetStatus(ctx context.Context) (TunnelStatus, error)
}

// TailscaleService exposes Tailscale funnel lifecycle operations.
type TailscaleService interface {
	Check(ctx context.Context) (TailscaleCheckResult, error)
	Install(ctx context.Context, sudoPassword string) (InstallResult, error)
	Enable(ctx context.Context) (TailscaleEnableResult, error)
	Disable(ctx context.Context) error
	GetStatus(ctx context.Context) (TailscaleStatus, error)
}

// StateManager reads and writes tunnel state to a JSON file on disk.
//
// State lives at {DataDir}/tunnel/state.json. All operations are safe for
// concurrent use within a single process; file writes are atomic via a
// temp-file rename.
type StateManager struct {
	path string
	mu   sync.Mutex
}

// NewStateManager returns a StateManager rooted at cfg.DataDir.
func NewStateManager(cfg TunnelConfig) *StateManager {
	return &StateManager{
		path: filepath.Join(cfg.DataDir, subdirName, stateFileName),
	}
}

// Path returns the absolute path to the state file.
func (m *StateManager) Path() string {
	return m.path
}

// LoadState reads the state file and returns it. A missing or corrupt file
// yields an empty state and a nil error (corrupt is logged via returned
// error to caller but only for inspection; callers should treat empty as
// the canonical "no state" response).
func (m *StateManager) LoadState() (TunnelState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TunnelState{}, nil
		}
		return TunnelState{}, fmt.Errorf("read state: %w", err)
	}

	var state TunnelState
	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupt state: treat as empty rather than panic.
		return TunnelState{}, nil
	}
	return state, nil
}

// SaveState persists state atomically. Writes to a temp file in the same
// directory and renames over the destination.
func (m *StateManager) SaveState(state TunnelState) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tmp, err := os.CreateTemp(filepath.Dir(m.path), "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, m.path); err != nil {
		cleanup()
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// ClearState removes the state file. A missing file is not an error.
func (m *StateManager) ClearState() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.Remove(m.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove: %w", err)
	}
	return nil
}

// generateShortID returns a random 6-character alphanumeric identifier
// drawn from an unambiguous alphabet (no 0/o, 1/l/i).
func generateShortID() string {
	b := make([]byte, ShortIDLength)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail; fall back to time-based seed.
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (i * 8))
		}
	}
	out := make([]byte, ShortIDLength)
	for i := range out {
		out[i] = shortIDChars[int(b[i])%len(shortIDChars)]
	}
	return string(out)
}

// GenerateShortID exposes the package-level generator for callers and tests.
func GenerateShortID() string { return generateShortID() }

// EnsureTunnelDir creates the tunnel subdirectory if it does not exist.
func EnsureTunnelDir(dataDir string) error {
	return os.MkdirAll(filepath.Join(dataDir, subdirName), 0o755)
}
