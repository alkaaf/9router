package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
)

// StatusRequestHandler handles GET /api/tunnel/status.
//
// It returns a combined status response for both Cloudflare and
// Tailscale tunnels plus the download state. The handler is wired up
// when the Fiber framework is initialised.
//
// Dependencies (to be supplied by the caller):
//
//	cfEnabled, cfRunning, cfURL, cfShortID: Cloudflare state
//	tsEnabled, tsRunning, tsLoggedIn: Tailscale state
//	downloading, progress: binary download state
type StatusRequestHandler struct {
	cfEnabled         bool
	cfSettingsEnabled bool
	cfURL             string
	cfShortID         string
	cfPublicURL       string
	cfRunning         bool

	tsEnabled         bool
	tsSettingsEnabled bool
	tsURL             string
	tsRunning         bool
	tsLoggedIn        bool

	downloading bool
	progress    int
}

// StatusResponse is the JSON returned by the status endpoint.
type StatusResponse struct {
	Tunnel TunnelStatusSection  `json:"tunnel"`
	TS     TailscaleStatusSection `json:"tailscale"`
	DL     DownloadSection      `json:"download"`
	Err    string               `json:"error,omitempty"`
}

type TunnelStatusSection struct {
	Enabled         bool   `json:"enabled"`
	SettingsEnabled bool   `json:"settingsEnabled"`
	TunnelURL       string `json:"tunnelUrl"`
	ShortID         string `json:"shortId"`
	PublicURL       string `json:"publicUrl"`
	Running         bool   `json:"running"`
}

type TailscaleStatusSection struct {
	Enabled         bool   `json:"enabled"`
	SettingsEnabled bool   `json:"settingsEnabled"`
	TunnelURL       string `json:"tunnelUrl"`
	Running         bool   `json:"running"`
	LoggedIn        bool   `json:"loggedIn"`
}

type DownloadSection struct {
	Downloading bool `json:"downloading"`
	Progress    int  `json:"progress"`
}

// Handle composes the response JSON. ctx is accepted for future
// auth/settings integration but unused in the current stub.
func (h *StatusRequestHandler) Handle(_ context.Context) ([]byte, error) {
	resp := StatusResponse{
		Tunnel: TunnelStatusSection{
			Enabled:         h.cfEnabled,
			SettingsEnabled: h.cfSettingsEnabled,
			TunnelURL:       h.cfURL,
			ShortID:         h.cfShortID,
			PublicURL:       h.cfPublicURL,
			Running:         h.cfRunning,
		},
		TS: TailscaleStatusSection{
			Enabled:         h.tsEnabled,
			SettingsEnabled: h.tsSettingsEnabled,
			TunnelURL:       h.tsURL,
			Running:         h.tsRunning,
			LoggedIn:        h.tsLoggedIn,
		},
		DL: DownloadSection{
			Downloading: h.downloading,
			Progress:    h.progress,
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal status response: %w", err)
	}
	return b, nil
}
