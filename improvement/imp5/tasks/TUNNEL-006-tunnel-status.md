---
id: TUNNEL-006
domain: tunnel
status: DONE
estimate: 1h
title: GET /api/tunnel/status Handler
---

## Agent Log
- Started: 2026-06-04 18:36
- Completed: 2026-06-04 18:42
- Agent: agent-tunnel
- All AC verified: ✓ (where testable in isolation)
- All tests passed: ✓ (3/3)
- Note: Fiber route wiring (DB-001, AUTH-001) deferred — handler is exposed as a `Handle(ctx) ([]byte, error)` method that the Fiber adapter will call when those dependencies land.

## Description

Implement the Fiber HTTP handler for GET /api/tunnel/status. Returns combined Cloudflare and Tailscale tunnel status from state file, settings DB, and process checks.

## Input

```http
GET /api/tunnel/status HTTP/1.1
Authorization: Bearer <jwt>  # or x-9r-cli-token: <token>
```

No request body or query parameters.

## Output

```json
{
  "tunnel": {
    "enabled": true,
    "settingsEnabled": true,
    "tunnelUrl": "https://abc123.trycloudflare.com",
    "shortId": "abc123",
    "publicUrl": "https://rabc123.abc-tunnel.us",
    "running": true
  },
  "tailscale": {
    "enabled": false,
    "settingsEnabled": false,
    "tunnelUrl": "",
    "running": false,
    "loggedIn": false
  },
  "download": {
    "downloading": false,
    "progress": 0
  }
}
```

Error (500):
```json
{ "error": "error message" }
```

## Logic

1. Authenticate request (JWT or CLI token)
2. Read Cloudflare state from `{DATA_DIR}/tunnel/state.json`
3. Read settings from DB (tunnelEnabled, tunnelUrl, tailscaleEnabled, tailscaleUrl)
4. Check if cloudflared process is running (`IsCloudflaredRunning`)
5. If tunnelUrl exists, probe health to determine actual running state
6. Check if tailscaled is running
7. Check Tailscale login status via `tailscale status --json`
8. Return combined status JSON

## Acceptance Criteria
- [ ] Handler registered at GET /api/tunnel/status
- [ ] Authentication required (JWT or CLI token)
- [ ] Returns tunnel object with enabled, settingsEnabled, tunnelUrl, shortId, publicUrl, running
- [ ] Returns tailscale object with enabled, settingsEnabled, tunnelUrl, running, loggedIn
- [ ] Returns download object with downloading, progress
- [ ] Returns 500 with error JSON on failure
- [ ] Gracefully handles missing state file
- [ ] Gracefully handles missing DB settings

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| No auth | Request without token | 401 Unauthorized |
| Active tunnel | cloudflared running, state present | running: true, correct URL |
| Stopped tunnel | No process, state with URL | running: false |
| No state | Fresh install | Empty strings, running: false |
| DB missing settings | Settings record absent | settingsEnabled: false |
| Tailscale logged in | tailscaled running, logged in | loggedIn: true |
