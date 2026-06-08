package cloudflare

import (
	"context"
	"time"
)

// HealthAdapter satisfies the HealthChecker interface used by
// TunnelManager by delegating to the package-level HealthConfig-based
// functions. This lets the manager use the real health probe logic
// without taking a hard dependency on it.
type HealthAdapter struct {
	Config HealthConfig
}

// ProbeURL implements HealthChecker.
func (a HealthAdapter) ProbeURL(ctx context.Context, rawURL string) (bool, error) {
	return ProbeURLAlive(ctx, rawURL, a.Config)
}

// WaitForHealth implements HealthChecker. The timeout parameter is
// respected by setting TotalWait on a fresh config built from a.Config.
func (a HealthAdapter) WaitForHealth(ctx context.Context, rawURL string, timeout time.Duration) error {
	cfg := a.Config
	if timeout > 0 {
		cfg.TotalWait = timeout
	}
	return WaitForHealth(ctx, rawURL, cfg)
}
