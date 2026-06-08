---
id: CHAT-009
domain: chat-core
status: DONE
estimate: 1h
title: Mark Unavailable
---

## Description

Mark account as unavailable after upstream failure. Calculates backoff, writes model lock, updates error state. Called from fallback loop when a credential attempt fails.

## Input

- `*Credentials` (current credentials)
- `int` (status code)
- `string` (error message)
- `string` (provider)
- `string` (model)
- `int64` (resetsAtMs)

## Output

- `error` (nil on success)
- Updates account in database

## Logic

1. Calculate shouldFallback and cooldownMs
2. If shouldFallback:
   - Write modelLock_<model> with cooldown expiry
   - Increment backoffLevel
   - Set lastError and lastErrorAt
3. If noAuth, skip (no persistence needed)
4. Return error for caller to handle

## Agent Log
- Started: 2026-06-04 19:15
- Completed: 2026-06-04 19:20
- Agent: agent-chat
- AC-001 verified: TestMarkUnavailable_429 — modelLock + expiry set
- AC-002 verified: TestCheckFallbackError_429_LevelIncreases — backoffLevel incremented
- AC-003 verified: TestMarkUnavailable_429 — lastError + lastErrorAt set
- AC-004 verified: TestMarkUnavailable_NoAuth — no-op
- AC-005 verified: TestMarkUnavailable_429 + TestMarkUnavailable_400 — shouldFallback flag returned

## Acceptance Criteria
- [x] Model lock written with correct expiry
- [x] Backoff level incremented
- [x] Error state updated (lastError, lastErrorAt)
- [x] NoAuth connections skipped
- [x] Fallback continues on shouldFallback=true

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/backoff.go (MarkUnavailable)
- Tests: internal/chatcore/backoff_test.go

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Rate limit | 429 | Model lock set, fallback continues |
| Auth error | 401 | shouldFallback=false, no lock |
| Server error | 500 | Model lock set, fallback continues |
| NoAuth | noauth | No-op |
