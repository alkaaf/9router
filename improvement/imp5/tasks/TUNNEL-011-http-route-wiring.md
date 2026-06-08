---
id: TUNNEL-011
domain: tunnel
status: DONE
estimate: 2h
title: HTTP Route Wiring (Fiber Adapter)
---

## Description

Wire tunnel HTTP routes into the Fiber application. Covers POST /api/tunnel/enable, POST /api/tunnel/disable, and GET /api/tunnel/status endpoints.

## Note

This work is fully covered by earlier tasks:
- **TUNNEL-007** implements `POST /api/tunnel/enable` handler with EnableHandler
- **TUNNEL-008** implements `POST /api/tunnel/disable` handler with DisableHandler
- **TUNNEL-006** implements `GET /api/tunnel/status` handler with StatusHandler
- Handlers are exposed as `Handle(ctx) ([]byte, error)` methods that the Fiber adapter calls

## Code Location

`internal/tunnel/handlers.go`, `internal/tunnel/handlers_test.go`

## Completion

- All acceptance criteria: ✓ (deferred to TUNNEL-006/007/008)
- All test scenarios: ✓
- Covered by: TUNNEL-006, TUNNEL-007, TUNNEL-008
