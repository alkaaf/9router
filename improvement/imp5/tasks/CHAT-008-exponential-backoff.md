---
id: CHAT-008
domain: chat-core
status: DONE
estimate: 1h
title: Exponential Backoff
---

## Description

Mark account+model as unavailable. Calculates cooldown using exponential backoff OR precise `resetsAtMs` (from upstream Retry-After). Writes `modelLock_<model>` field with expiry timestamp. Increments `backoffLevel` for repeated failures.

## Input

- `string` (connectionId)
- `int` (status code from upstream)
- `string` (error message)
- `string` (provider)
- `string` (model)
- `int64` (resetsAtMs - precise cooldown from upstream)

## Output

- `{shouldFallback bool, cooldownMs int}`
- Updates account record in DB

## Logic

1. Check status code:
   - 429 → shouldFallback=true
   - 400 → shouldFallback=false (client error)
   - 500+ → shouldFallback=true
2. Calculate cooldown:
   - If resetsAtMs > now, use precise: `min(resetsAtMs - now, MAX_RATE_LIMIT_COOLDOWN_MS)`
   - Else use exponential backoff based on backoffLevel
3. Exponential levels: 5s, 30s, 2min, 10min, 1hr
4. Write modelLock_<model> with expiry to account
5. Increment backoffLevel in account
6. If noAuth connection, return no-op

## Agent Log
- Started: 2026-06-04 19:05
- Completed: 2026-06-04 19:15
- Agent: agent-chat
- AC-001 verified: TestCheckFallbackError_429 — returns shouldFallback=true
- AC-002 verified: TestCheckFallbackError_400 — returns shouldFallback=false
- AC-003 verified: TestCheckFallbackError_500 — returns shouldFallback=true
- AC-004 verified: TestComputeLockExpiry_Precise / TestMarkUnavailable_ResetsAtMs — precise reset overrides backoff
- AC-005 verified: TestCheckFallbackError_429_LevelIncreases — level increments
- AC-006 verified: TestMarkUnavailable_429 — modelLock written
- AC-007 verified: TestMarkUnavailable_NoAuth — no-op for noAuth

## Acceptance Criteria
- [x] 429 status returns shouldFallback=true
- [x] 400 status returns shouldFallback=false
- [x] 500 status returns shouldFallback=true
- [x] resetsAtMs overrides backoff when provided
- [x] Backoff level increases on consecutive errors
- [x] modelLock written to connection record
- [x] noAuth connection is no-op

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/backoff.go
- Tests: internal/chatcore/backoff_test.go (20 tests, all pass)
