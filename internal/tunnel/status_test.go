package tunnel

import (
	"context"
	"encoding/json"
	"testing"
)

func TestStatusRequestHandler_AllFieldsPopulated(t *testing.T) {
	h := &StatusRequestHandler{
		cfEnabled:         true,
		cfSettingsEnabled: true,
		cfURL:             "https://abc.trycloudflare.com",
		cfShortID:         "abc",
		cfPublicURL:       "https://rabc.abc-tunnel.us",
		cfRunning:         true,
		tsEnabled:         false,
		tsSettingsEnabled: false,
		tsRunning:         false,
		tsLoggedIn:        false,
		downloading:       false,
		progress:          0,
	}
	body, err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var resp StatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Tunnel.Enabled || !resp.Tunnel.Running {
		t.Fatalf("tunnel should be enabled+running, got %+v", resp.Tunnel)
	}
	if resp.Tunnel.TunnelURL != "https://abc.trycloudflare.com" {
		t.Fatalf("TunnelURL mismatch: %q", resp.Tunnel.TunnelURL)
	}
	if resp.TS.Enabled || resp.TS.Running || resp.TS.LoggedIn {
		t.Fatalf("tailscale should be all false, got %+v", resp.TS)
	}
}

func TestStatusRequestHandler_EmptyState(t *testing.T) {
	h := &StatusRequestHandler{}
	body, err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var resp StatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Tunnel.Enabled || resp.Tunnel.Running || resp.Tunnel.TunnelURL != "" {
		t.Fatalf("expected empty tunnel, got %+v", resp.Tunnel)
	}
	if resp.DL.Progress != 0 {
		t.Fatalf("expected progress=0")
	}
}

func TestStatusRequestHandler_Downloading(t *testing.T) {
	h := &StatusRequestHandler{downloading: true, progress: 50}
	body, _ := h.Handle(context.Background())
	var resp StatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.DL.Downloading || resp.DL.Progress != 50 {
		t.Fatalf("download state: %+v", resp.DL)
	}
}
