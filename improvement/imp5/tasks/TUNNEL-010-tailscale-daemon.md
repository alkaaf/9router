---
id: TUNNEL-010
domain: tunnel
status: DONE
estimate: 2h
title: Tailscale Daemon Management
---

## Agent Log
- Started: 2026-06-04 19:11
- Completed: 2026-06-04 19:15
- Agent: agent-tunnel
- All AC verified: ✓
- All tests passed: ✓ (covered by tailscale_test.go)

## Description

Implement tailscaled daemon lifecycle management: start, stop, status check, and login detection. Handles TUN mode vs userspace-networking based on sudo availability.

## Input

```go
type DaemonInput struct {
    Context      context.Context
    SudoPassword string  // Optional: enables TUN mode
    SocketPath   string  // Default: {DATA_DIR}/tailscale/tailscaled.sock
    StateDir     string  // Default: {DATA_DIR}/tailscale/
}
```

## Output

```go
func StartDaemon(ctx context.Context, sudoPassword string, socketPath string, stateDir string) error
func StopDaemon(ctx context.Context, sudoPassword string) error
func IsDaemonRunning() bool
func IsLoggedIn() (bool, error)
func GetDaemonStatus() (DaemonStatus, error)
type DaemonStatus struct {
    Running    bool
    LoggedIn   bool
    BackendState string
    SocketPath string
}
```

## Logic

### StartDaemon
1. Create state directory if missing
2. Build args: `--socket=<path>`, `--statedir=<dir>`
3. If no sudo password: add `--tun=userspace-networking`
4. If sudo password available: use TUN mode (default)
5. Set `SysProcAttr.Setpgid = true` for process group isolation
6. Spawn `tailscaled` as detached subprocess
7. Wait up to 5s for socket to become available
8. Return error if socket not created in time

### StopDaemon
1. Run `tailscale down` to disconnect
2. Kill tailscaled process (PID file, process group, or systemd)
3. Remove socket file

### IsDaemonRunning
1. Check if socket file exists
2. Try `tailscale status --json` via socket
3. Parse JSON for BackendState field

### IsLoggedIn
1. Run `tailscale status --json`
2. Parse `BackendState` field
3. Return true if state is "Running" (not "NeedsLogin", "Stopped", etc.)

## Acceptance Criteria
- [x] StartDaemon creates state directory
- [x] StartDaemon spawns tailscaled with correct args
- [x] StartDaemon uses userspace-networking without sudo
- [x] StartDaemon uses TUN mode with sudo
- [x] StartDaemon waits for socket availability
- [x] StopDaemon disconnects and kills process
- [x] IsDaemonRunning checks socket + status command
- [x] IsLoggedIn parses BackendState correctly
- [x] Process group isolation via Setpgid
- [x] Socket path configurable

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Start without sudo | No sudoPassword | Daemon starts, userspace-networking |
| Start with sudo | SudoPassword provided | Daemon starts, TUN mode |
| Start when running | Daemon already active | No error, returns success |
| Stop running | Active daemon | Stopped, socket removed |
| Stop not running | No daemon | No error |
| IsLoggedIn fresh | New install | false (NeedsLogin) |
| IsLoggedIn active | Connected device | true (Running) |
| Socket timeout | Slow system | Error after 5s |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/tunnel/tailscale/tailscale.go (Manager.StartDaemon, StopDaemon, IsDaemonRunning, IsLoggedIn, GetDaemonStatus, statusJSON)
- Socket timeout is 5s (configurable via StartDaemon's caller)
