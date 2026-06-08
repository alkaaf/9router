---
id: TUNNEL-011
domain: tunnel
status: DONE
estimate: 2h
title: Tailscale Login Flow
---

## Agent Log
- Started: 2026-06-04 19:16
- Completed: 2026-06-04 19:18
- Agent: agent-tunnel
- All AC verified: ✓
- All tests passed: ✓

## Description

Implement Tailscale login flow with auth URL capture. Runs `tailscale up` to trigger login, captures the auth URL from output, and polls status until login completes or times out.

## Input

```go
type LoginInput struct {
    Context   context.Context
    Hostname  string   // ShortID for --hostname flag
    SocketPath string  // Tailscale daemon socket
}
type LoginResult struct {
    AuthURL         string // URL for browser login (if needsLogin)
    AlreadyLoggedIn bool
}
type LoginStatus int
const (
    LoginStatusNeedsAuth LoginStatus = iota
    LoginStatusLoggedIn
    LoginStatusTimeout
)
```

## Output

```go
func Login(ctx context.Context, hostname string, socketPath string) (*LoginResult, error)
```

## Logic

### Login Flow
1. Check if already logged in via `tailscale status --json`
   - If BackendState == "Running": return `AlreadyLoggedIn: true`
   - If BackendState == "NeedsLogin": proceed with login
2. Run `tailscale up --accept-routes --hostname=<shortId> --socket=<path>`
3. Capture stdout/stderr for auth URL pattern: `https://login.tailscale.com/a/...`
4. If auth URL found in output: poll status every 2s for up to 15s
5. Parse `tailscale status --json` for:
   - `AuthURL` field (Windows-specific)
   - `BackendState` transition from "NeedsLogin" to "Running"
6. Return LoginResult with AuthURL if login required
7. Return timeout error if not logged in within 15s

### URL Extraction
```
Regex: https://login\.tailscale\.com/a/[a-zA-Z0-9_-]+
```

### Timeout Behavior
- Poll interval: 2s
- Total timeout: 15s
- On timeout: return `LoginStatusTimeout` with partial result

## Acceptance Criteria
- [x] Detects already-logged-in state
- [x] Runs `tailscale up` with --accept-routes and --hostname
- [x] Extracts auth URL from command output
- [x] Polls `tailscale status --json` for state changes
- [x] Returns AuthURL when login required
- [x] Returns AlreadyLoggedIn when already connected
- [x] Times out after 15s if login not completed
- [x] Respects context cancellation

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Already logged in | BackendState: "Running" | AlreadyLoggedIn: true |
| Needs login | BackendState: "NeedsLogin" | AuthURL returned |
| Login completes | User auth during poll | Logged in within timeout |
| Auth URL in output | stdout contains URL | Extracted and returned |
| Timeout | No login within 15s | LoginStatusTimeout error |
| Context cancel | Cancel during poll | Returns ctx.Err() |
| URL regex | "https://login.tailscale.com/a/abc123" | Matched correctly |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/tunnel/tailscale/tailscale.go (Manager.Login, extractAuthURL, extractEnableURL)
- URL regex matches the spec: `https://login\.tailscale\.com/a/[a-zA-Z0-9_-]+`
- 15s timeout with 2s poll interval (configurable via caller)
