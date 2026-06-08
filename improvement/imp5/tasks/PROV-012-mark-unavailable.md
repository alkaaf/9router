---
id: PROV-012
domain: providers
status: DONE
estimate: 2h
title: Mark Unavailable
---

  evidence: |
  (verified 2026-06-04)
## Description

Lock a connection+model after a failed request (429, 401, 5xx, etc.) via `markAccountUnavailable`. Compute cooldown with exponential backoff based on status code and error text. If provider sends `resetsAtMs` (e.g., codex `resets_at`), use that as precise cooldown. Set `modelLock_${model}` (or `modelLock___all`) with expiry timestamp in the DB. Also set `testStatus: "unavailable"`, `lastError`, `errorCode`, `lastErrorAt`, `backoffLevel`.

## Input

- `connectionId`, `status` (HTTP code), `errorText`, `provider`, `model`, `resetsAtMs` (optional)

## Output

- `{ shouldFallback: bool, cooldownMs: number }`

## Logic

1. Load connection from DB by `connectionId`.
2. Call `checkFallbackError(status, errorText, backoffLevel)` to determine if fallback needed and compute cooldown (PROV-023).
3. If `resetsAtMs` provided, use it directly as cooldownMs (precise timer from provider).
4. Compute lock key: `modelLock___all` for account-level errors, `modelLock_${model}` for model-specific errors.
5. Compute lock expiry: `now + cooldownMs`.
6. Update DB connection:
   - Set lock key column with expiry timestamp
   - `testStatus = "unavailable"`
   - `lastError = errorText`
   - `errorCode = status`
   - `lastErrorAt = now`
   - Increment `backoffLevel` (0-3)
7. Return `{ shouldFallback, cooldownMs }`.

## Acceptance Criteria
- [x] 429 error → shouldFallback=true with computed cooldown
- [x] 401 → shouldFallback=true (auth error, no retry)
- [x] 500 → shouldFallback=true with backoff
- [x] 400 (bad request) → shouldFallback=false (don't lock)
- [x] resetsAtMs overrides backoff with precise timer
- [x] Model lock key written to DB with correct expiry
- [x] backoffLevel incremented on repeated errors

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Rate limit 429 | status=429, backoffLevel=0 | shouldFallback=true, cooldownMs=30000 |
| Auth error 401 | status=401 | shouldFallback=true, cooldownMs=0 |
| Server error 500 | status=500, backoffLevel=1 | shouldFallback=true, cooldownMs=120000 |
| Bad request 400 | status=400 | shouldFallback=false |
| resetsAtMs | resetsAtMs=1234567890000 | Uses resetsAtMs as precise cooldown |
| Model lock key | model="gpt-4o" | modelLock_gpt-4o column set with expiry |
| Account lock | status=401 | modelLock___all column set |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- All AC verified: ✓
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: MarkUnavailable + CheckFallbackError in lifecycle.go
- Tests: TestCheckFallbackError_429, TestCheckFallbackError_401, TestCheckFallbackError_500, TestCheckFallbackError_400, TestMarkUnavailable_429, TestMarkUnavailable_401, TestMarkUnavailable_400, TestMarkUnavailable_ResetsAtMs
