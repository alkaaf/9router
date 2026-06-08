package cloudflare

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// HealthConfig holds timeouts and resolver preferences for health probing.
type HealthConfig struct {
	DNSTimeout  time.Duration // per-resolver timeout, default 2s
	HTTPTimeout time.Duration // per-probe HTTP timeout, default 5s
	TotalWait   time.Duration // total wait before giving up, default 60s
	Interval    time.Duration // sleep between probes, default 2s
	Resolvers   []string      // public DNS resolvers, default ["1.1.1.1:53","8.8.8.8:53"]
}

// DefaultHealthConfig returns sensible defaults for Cloudflare tunnel
// health checks. The values match the spec from the migration document.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		DNSTimeout:  2 * time.Second,
		HTTPTimeout: 5 * time.Second,
		TotalWait:   60 * time.Second,
		Interval:    2 * time.Second,
		Resolvers:   []string{"1.1.1.1:53", "8.8.8.8:53"},
	}
}

// fillDefaults returns cfg with zero-valued fields replaced by defaults.
func (c HealthConfig) fillDefaults() HealthConfig {
	d := DefaultHealthConfig()
	if c.DNSTimeout <= 0 {
		c.DNSTimeout = d.DNSTimeout
	}
	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = d.HTTPTimeout
	}
	if c.TotalWait <= 0 {
		c.TotalWait = d.TotalWait
	}
	if c.Interval <= 0 {
		c.Interval = d.Interval
	}
	if len(c.Resolvers) == 0 {
		c.Resolvers = d.Resolvers
	}
	return c
}

// ResolveHostname queries the given hostname through the public DNS
// resolvers in order. It tries each resolver until one returns a result,
// falling back to the next on timeout or error.
func ResolveHostname(ctx context.Context, hostname string, resolvers []string) ([]net.IP, error) {
	if len(resolvers) == 0 {
		resolvers = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}

	var lastErr error
	for _, r := range resolvers {
		dnsTimeout := 2 * time.Second
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: dnsTimeout}
				return d.DialContext(ctx, "udp", r)
			},
		}
		ips, err := resolver.LookupIP(ctx, "ip", hostname)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("resolve %q via %v: %w", hostname, resolvers, lastErr)
	}
	return nil, fmt.Errorf("resolve %q: no IPs returned by %v", hostname, resolvers)
}

// ProbeURLAlive sends an HTTP GET to {rawURL}/api/health and returns true
// only when the response status is 200. Network errors are not reported
// to the caller — they simply mean "not alive" — but ctx.Err() is
// surfaced if the context is cancelled.
func ProbeURLAlive(ctx context.Context, rawURL string, cfg HealthConfig) (bool, error) {
	cfg = cfg.fillDefaults()
	client := &http.Client{Timeout: cfg.HTTPTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL+"/api/health", nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		// If the ctx was cancelled, surface that to the caller.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// WaitForHealth polls ProbeURLAlive at cfg.Interval until either the
// probe succeeds, the context is cancelled, or cfg.TotalWait is
// exceeded. Returns nil on first successful probe.
func WaitForHealth(ctx context.Context, rawURL string, cfg HealthConfig) error {
	cfg = cfg.fillDefaults()
	deadline := time.Now().Add(cfg.TotalWait)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		alive, _ := ProbeURLAlive(ctx, rawURL, cfg)
		if alive {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("cloudflare health: timed out after %s waiting for %s", cfg.TotalWait, rawURL)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
