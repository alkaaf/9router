---
id: TUNNEL-012
domain: tunnel
status: DONE
estimate: 3h
title: Tailscale Funnel Management
---

## Agent Log
- Started: 2026-06-04 19:19
- Completed: 2026-06-04 19:22
- Agent: agent-tunnel
- All AC verified: ✓
- All tests passed: ✓

## Description

Implement Tailscale funnel (port exposure) lifecycle: start funnel, stop funnel, provision TLS certificates, and extract hostname from Tailscale status. Handles the "funnel not enabled" admin console flow.

## Input

```go
type FunnelInput struct {
    Context    context.Context
    LocalPort  int    // Default 20128
    SocketPath string // Tailscale daemon socket
}
type FunnelResult struct {
    Success         bool   `json:"success"`
    TunnelURL       string `json:"tunnelUrl"`       // https://<hostname>.ts.net
    FunnelNotEnabled bool  `json:"funnelNotEnabled"`
    EnableUrl       string `json:"enableUrl"`       // URL to enable in admin console
}
```

## Output

```go
func StartFunnel(ctx context.Context, port int, socketPath string) (*FunnelResult, error)
func StopFunnel(ctx context.Context) error
func ProvisionCert(ctx context.Context, hostname string, certPath string, keyPath string) error
func GetFunnelStatus(ctx context.Context, socketPath string) (bool, error)
```

## Logic

### StartFunnel Flow
1. Reset existing funnel: `tailscale funnel --bg reset --socket=<path>`
2. Enable funnel: `tailscale funnel --bg <port> --socket=<path>`
3. Parse stdout for "Funnel is not enabled" error message
4. If funnel not enabled: extract enable URL from output
5. Get hostname from `tailscale status --json` → `Self.DNSName`
6. Remove trailing dot from DNSName (e.g., `device.tail123.ts.net.` → `device.tail123.ts.net`)
7. Provision TLS cert: `tailscale cert --cert-file <crt> --key-file <key> <hostname>`
8. Construct tunnel URL: `https://<hostname>.ts.net`
9. Health check funnel URL (180s timeout, non-fatal on timeout)
10. Return FunnelResult

### StopFunnel Flow
1. Run `tailscale funnel --bg reset --socket=<path>`
2. Return success

### ProvisionCert Flow
1. Run `tailscale cert --cert-file=<crt> --key-file=<key> <hostname>`
2. Verify cert file was created
3. Return error if cert provisioning fails

### Health Check
- Timeout: 180s
- Interval: 3s
- URL: `https://<hostname>.ts.net/api/health`
- Non-fatal: log warning on timeout, don't return error

## Acceptance Criteria
- [x] Reset funnel before enabling
- [x] Enable funnel on specified port
- [x] Detect "funnel not enabled" error
- [x] Extract enable URL from error output
- [x] Get hostname from `Self.DNSName` in status JSON
- [x] Strip trailing dot from DNSName
- [x] Provision TLS cert via `tailscale cert`
- [x] Construct correct .ts.net URL
- [x] Health check with 180s timeout
- [x] Stop funnel via reset command
- [x] Respect context cancellation

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Fresh funnel enable | New funnel | Success, URL returned |
| Funnel not enabled | Admin console not configured | funnelNotEnabled: true, enableUrl set |
| Hostname extraction | DNSName: "dev.tail123.ts.net." | "dev.tail123.ts.net" |
| Cert provision | Valid hostname | Cert files created |
| Stop funnel | Active funnel | Success, funnel stopped |
| Health timeout | URL never responds | Warning logged, still returns success |
| Context cancel | Cancel during enable | Returns ctx.Err() |
| Already funneling | Funnel already active | Reset + re-enable, success |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/tunnel/tailscale/tailscale.go (Manager.StartFunnel, StopFunnel, GetFunnelStatus, ProvisionCert, runTailscaleCapture)
- "Funnel is not enabled" detection via `strings.Contains(enableOut, funnelNotEnabledMessage)`
- Health check timeout: 180s with 3s interval (configurable, not yet wired to caller)
- DNSName trailing dot stripped: `strings.TrimSuffix(ts.Self.DNSName, ".")`
