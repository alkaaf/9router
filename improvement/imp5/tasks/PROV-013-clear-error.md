---
id: PROV-013
domain: providers
status: DONE
estimate: 1h
title: Clear Error
---

  evidence: |
  (verified 2026-06-04)
## Description

On successful request, clear the model lock for the model that just succeeded via `clearAccountError`. Lazy-clean any other expired `modelLock_*` keys. Reset error state (`testStatus`, `lastError`, `lastErrorAt`, `backoffLevel`) only if no active locks remain.

## Input

- `connectionId`, `currentConnection` (credentials object with `_connection` or raw connection), `model` (the model that succeeded)

## Output

- DB update with cleared lock keys + optional error state reset
- No-op if no active locks and no error state

## Logic

1. Load connection from DB by `connectionId`.
2. If `model` is provided, clear the `modelLock_${model}` key (set to null/past).
3. Scan all `modelLock_*` columns. For each with a past expiry timestamp, clear it (lazy cleanup).
4. Check if any `modelLock_*` keys remain with future expiry. If yes, skip error state reset.
5. If no active locks remain, reset:
   - `testStatus = "active"` (or null, based on last test)
   - `lastError = null`
   - `lastErrorAt = null`
   - `backoffLevel = 0`
6. Return no-op indication if nothing to clear.

## Acceptance Criteria
- [x] Successful model request clears that model's lock
- [x] All expired locks cleaned (lazy cleanup)
- [x] Error state reset only when no active locks remain
- [x] No-op when nothing to clear (no locks, no errors)
- [x] Partial lock cleanup: only clears specified model, other locks preserved

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Clear model lock | model="gpt-4o" has active lock | modelLock_gpt-4o cleared, other locks preserved |
| Clear expired locks | Some locks expired, some active | Only expired locks cleared, active locks remain |
| Reset error state | No active locks remain | testStatus, lastError, backoffLevel reset |
| No-op | No locks and no error state | No DB update performed |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- All AC verified: ✓
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: ClearAccountError in lifecycle.go
- Tests: TestClearAccountError_ClearModelLock, TestClearAccountError_NoOp, TestClearAccountError_ExpiredLocksCleaned, TestClearAccountError_ErrorStateResetOnlyWhenNoLocks
