---
id: CHAT-010
domain: chat-core
status: DONE
estimate: 1h
title: Clear Error
---

## Description

Clear account error state on successful request. Removes `modelLock_<model>`, lazy-cleans expired locks, resets `testStatus` to "active" only if no remaining active locks.

## Input

- `string` (connectionId)
- `*Connection` (current account state)
- `string` (model)
- ProviderConnection repository

## Output

- `error` (nil on success)
- Updates account in database

## Logic

1. If noAuth connection, return nil (no persistence)
2. Delete modelLock_<model> from account
3. Lazy-clean: delete any expired locks
4. Check remaining active locks
5. If no remaining locks:
   - Reset testStatus to "active"
   - Clear lastError
   - Clear lastErrorAt
   - Reset backoffLevel to 0

## Agent Log
- Started: 2026-06-04 19:20
- Completed: 2026-06-04 19:25
- Agent: agent-chat
- AC-001 verified: TestClearAccountError_SingleLock — model unlocked
- AC-002 verified: TestClearAccountError_ExpiredLockLazyClean — expired locks auto-cleaned
- AC-003 verified: TestClearAccountError_MultipleLocks — active locks remain, testStatus stays
- AC-004 verified: TestClearAccountError_SingleLock — all locks cleared → testStatus=active
- AC-005 verified: TestClearAccountError_AlreadyClean — short-circuit
- AC-006 verified: TestClearAccountError_NoAuth — no-op

## Acceptance Criteria
- [x] Successful model unlocks that specific model
- [x] Expired locks are auto-cleaned
- [x] Active locks remain (testStatus not reset)
- [x] All locks cleared resets testStatus to active
- [x] No-op if connection was already clean
- [x] noAuth connection is no-op

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/backoff.go (ClearAccountError)
- Tests: internal/chatcore/backoff_test.go

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Single lock | lock on model A | Lock A deleted |
| Multiple locks | locks on A, B | Lock A deleted, B remains |
| All locks | only lock cleared | testStatus=active, backoff=0 |
| Expired lock | lock expired | Lock auto-deleted |
| NoAuth | noauth | No-op |
