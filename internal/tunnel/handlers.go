package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
)

// EnableResult mirrors the JSON shape returned by POST /api/tunnel/enable.
type EnableResult struct {
	Success        bool   `json:"success"`
	TunnelURL      string `json:"tunnelUrl"`
	ShortID        string `json:"shortId"`
	PublicURL      string `json:"publicUrl"`
	AlreadyRunning bool   `json:"alreadyRunning"`
}

// DisableResult mirrors the JSON shape returned by POST /api/tunnel/disable.
type DisableResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// HTTPError is the standard error response body.
type HTTPError struct {
	Error string `json:"error"`
}

// EnableHandler is a thin adapter that wraps a TunnelService and
// serialises its result as JSON. The actual HTTP wiring (Fiber route +
// auth middleware) is added in TUNNEL-011.
type EnableHandler struct {
	Service TunnelService
}

// Handle performs the enable. Returns JSON bytes (success body or error body).
func (h *EnableHandler) Handle(ctx context.Context) (status int, body []byte) {
	if h.Service == nil {
		return errorResponse("tunnel service not configured")
	}
	res, err := h.Service.Enable(ctx)
	if err != nil {
		return errorResponse(err.Error())
	}
	out := EnableResult{
		Success:        true,
		TunnelURL:      res.TunnelURL,
		ShortID:        res.ShortID,
		PublicURL:      res.PublicURL,
		AlreadyRunning: res.AlreadyRunning,
	}
	body, err = json.Marshal(out)
	if err != nil {
		return errorResponse(fmt.Sprintf("marshal: %v", err))
	}
	return 200, body
}

// DisableHandler is a thin adapter that wraps a TunnelService and
// serialises its result as JSON.
type DisableHandler struct {
	Service TunnelService
}

// Handle performs the disable. Returns JSON bytes (success body or error body).
func (h *DisableHandler) Handle(ctx context.Context) (status int, body []byte) {
	if h.Service == nil {
		return errorResponse("tunnel service not configured")
	}
	if err := h.Service.Disable(ctx); err != nil {
		return errorResponse(err.Error())
	}
	body, err := json.Marshal(DisableResult{Success: true})
	if err != nil {
		return errorResponse(fmt.Sprintf("marshal: %v", err))
	}
	return 200, body
}

func errorResponse(msg string) (int, []byte) {
	body, _ := json.Marshal(HTTPError{Error: msg})
	return 500, body
}
