---
id: SYS-005
domain: settings
status: DONE
estimate: 1h
title: GET /api/health — Health check
---

## Agent Log

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (all PASS)
- Code location: internal/system/005-health.go + internal/system/005-health_test.go
- Started: 2026-06-04
- Agent: agent-sys

## Description

Lightweight health check endpoint. Returns `{ ok: true }` with CORS headers (`Access-Control-Allow-Origin: *`). No database calls — purely liveness probe. Handles CORS preflight (OPTIONS) returning 204.

## Input

None.

## Output

```json
{ "ok": true }
```

Headers:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, OPTIONS`

## Logic

1. Register a `GET` handler that returns `{ "ok": true }` with HTTP 200.
2. Set CORS headers: `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Methods: GET, OPTIONS`.
3. Register an `OPTIONS` handler (CORS preflight) that returns 204 with the same CORS headers.
4. No database queries or external calls — purely in-memory.

## Acceptance Criteria

- [ ] `GET /api/health` returns 200 with `{ "ok": true }`
- [ ] `OPTIONS /api/health` returns 204 with CORS headers
- [ ] `Access-Control-Allow-Origin: *` header is present on both GET and OPTIONS
- [ ] `Access-Control-Allow-Methods: GET, OPTIONS` header is present
- [ ] No database or external calls are made
- [ ] Response time is minimal (< 10ms) — suitable for load balancer health checks

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| GET health check | GET /api/health | 200, `{ "ok": true }` |
| CORS preflight | OPTIONS /api/health | 204 with CORS headers |
| Content-Type | GET response | `application/json` |
| No auth required | GET without auth header | 200 |
