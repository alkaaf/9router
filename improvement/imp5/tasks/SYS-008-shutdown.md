---
id: SYS-008
domain: settings
status: DONE
estimate: 2h
title: POST /api/shutdown — Development shutdown
---

## Agent Log

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (all PASS)
- Code location: internal/system/008-shutdown.go + internal/system/008-shutdown_test.go
- Started: 2026-06-04
- Agent: agent-sys

## Description

Development-only graceful shutdown endpoint. Requires `SHUTDOWN_SECRET` env var and `Authorization: Bearer <secret>` header. Returns 403 in production. Exits the process after a 500ms delay to allow the response to be sent. Does not kill sibling processes (unlike the version/shutdown endpoint).

## Input

Header: `Authorization: Bearer <token>`

## Output

```json
{ "success": true, "message": "Shutting down..." }
```

## Logic

1. Register a `POST` handler.
2. Check `NODE_ENV` — if `production`, return 403 immediately.
3. Read `SHUTDOWN_SECRET` from environment variables.
4. Extract the Bearer token from the `Authorization` header.
5. If token is missing or does not match `SHUTDOWN_SECRET`, return 401.
6. If token matches, return 200 with `{ "success": true, "message": "Shutting down..." }`.
7. After sending the response, wait 500ms then call `process.exit(0)`.
8. Do NOT call `killAppProcesses()` — that is handled by the version/shutdown endpoint.

## Acceptance Criteria

- [ ] `POST /api/shutdown` returns 403 when `NODE_ENV === "production"`
- [ ] Returns 401 when `Authorization` header is missing
- [ ] Returns 401 when Bearer token does not match `SHUTDOWN_SECRET`
- [ ] Returns 200 with success message when token matches
- [ ] Process exits after 500ms delay on successful auth
- [ ] Does not kill sibling processes (cloudflared, MITM, next-server)
- [ ] Requires `SHUTDOWN_SECRET` env var to be set (returns 401/403 if not set and no token provided)

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Production env | POST with NODE_ENV=production | 403 |
| Missing token | POST without Authorization header | 401 |
| Wrong token | `Bearer wrong-secret` | 401 |
| Correct token (dev) | `Bearer <SHUTDOWN_SECRET>` with NODE_ENV=development | 200, then process exits after 500ms |
| No SHUTDOWN_SECRET env | POST with no token, NODE_ENV=development | 401 |
