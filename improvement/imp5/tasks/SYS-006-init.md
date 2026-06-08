---
id: SYS-006
domain: settings
status: DONE
estimate: 1h
title: GET /api/init — Initialization marker
---

## Agent Log

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (all PASS)
- Code location: internal/system/006-init.go + internal/system/006-init_test.go
- Started: 2026-06-04
- Agent: agent-sys

## Description

Called automatically on app startup to confirm the server is ready. Returns plain text `"Initialized"` with status 200. No database calls, no auth required. This is a simple liveness/startup confirmation endpoint.

## Input

None.

## Output

Plain text: `"Initialized"` — HTTP 200, `Content-Type: text/plain`.

## Logic

1. Register a `GET` handler that returns the plain text string `"Initialized"`.
2. Set `Content-Type: text/plain` header.
3. Return HTTP 200.
4. No database queries, no auth checks, no external calls.

## Acceptance Criteria

- [ ] `GET /api/init` returns 200
- [ ] Response body is exactly the string `"Initialized"` (plain text)
- [ ] `Content-Type: text/plain` header is set
- [ ] No database calls are made
- [ ] No auth is required
- [ ] Endpoint is accessible immediately on server start

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Startup ping | GET /api/init | 200, body = `"Initialized"`, `Content-Type: text/plain` |
| No auth | GET without auth header | 200 |
| Concurrent calls | Multiple simultaneous GETs | All return 200 consistently |
