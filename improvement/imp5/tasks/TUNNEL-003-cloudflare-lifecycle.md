---
id: TUNNEL-003
domain: tunnel
status: DONE
estimate: 3h
title: Cloudflare Tunnel Lifecycle Manager
---

## Agent Log
- Started: 2026-06-04 18:06
- Completed: 2026-06-04 18:25
- Agent: agent-tunnel
- All AC verified: ✓
- All tests passed: ✓ (26/26)

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/tunnel/cloudflare/manager.go, internal/tunnel/cloudflare/manager_test.go

## Description

Implement high-level Cloudflare tunnel lifecycle management: enable, disable, and status. Orchestrates binary download, process spawning, health checks, state persistence, and worker registration.

## Input

```go
type EnableInput struct {
    Context   context.Context
    LocalPort int    // Default 20128
}
type DisableInput struct {
    Context context.Context
}
```

## Output

```go
type TunnelResult struct {
    Success        bool   `json:"success"`
    TunnelURL      string `json:"tunnelUrl"`
    ShortID        string `json:"shortId"`
    PublicURL      string `json:"publicUrl"`
    AlreadyRunning bool   `json:"alreadyRunning"`
}
type TunnelStatus struct {
    Enabled         bool   `json:"enabled"`
    SettingsEnabled bool   `json:"settingsEnabled"`
    TunnelURL       string `json:"tunnelUrl"`
    ShortID         string `json:"shortId"`
    PublicURL       string `json:"publicUrl"`
    Running         bool   `json:"running"`
}
```

## Logic

### Enable Flow
1. Check if tunnel already running (health probe on both direct + public URL)
2. If running with valid URL, return `AlreadyRunning: true`
3. If stale process running, kill it first
4. Call `EnsureCloudflared` to download binary if needed
5. Generate new short ID (6-char alphanumeric)
6. Spawn `cloudflared tunnel --url http://127.0.0.1:20128` with config in temp dir
7. Capture tunnel URL from stdout
8. Save state to `{DATA_DIR}/tunnel/state.json` (shortId, tunnelUrl)
9. Update settings: `tunnelEnabled: true`, `tunnelUrl`
10. Register URL with worker via HTTP POST
11. Wait for health check (60s timeout, 2s interval)
12. Return TunnelResult

### Disable Flow
1. Kill cloudflared process (child + PID file + port-based)
2. Clear tunnel URL from state (preserve shortId)
3. Update settings: `tunnelEnabled: false`
4. Return success

### Status Flow
1. Read state from state.json
2. Read settings from DB
3. Check if cloudflared process is running
4. Check health of tunnel URL if available
5. Return TunnelStatus

## Acceptance Criteria
- [x] Enable downloads binary if missing
- [x] Enable spawns process and captures tunnel URL
- [x] Enable saves state and updates settings
- [x] Enable registers URL with worker
- [x] Enable waits for health (60s timeout)
- [x] Enable detects already-running tunnel
- [x] Enable kills stale processes before spawning
- [x] Disable kills process and clears state
- [x] Disable updates settings to disabled
- [x] Status returns accurate running/URL state
- [x] All operations support context cancellation

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Enable fresh | No existing tunnel | New tunnel spawned, URL returned |
| Enable when running | Tunnel already active | AlreadyRunning: true, same URL |
| Enable stale process | Zombie cloudflared | Killed, new tunnel spawned |
| Disable running | Active tunnel | Process killed, state cleared |
| Disable not running | No tunnel | Success, no error |
| Status active | Running tunnel | Running: true, correct URL |
| Status inactive | No tunnel | Running: false |
| Cancel enable | Cancel ctx during spawn | Returns error, no orphan process |
