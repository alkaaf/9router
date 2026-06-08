---
id: CHAT-007
domain: chat-core
status: DONE
estimate: 2h
title: Credential Round Robin
---

## Agent Log
- Started: 2026-06-04 19:00
- Completed: 2026-06-04 19:05
- Agent: agent-chat
- AC-001 verified: TestRoundRobin_NoLastUsed_PicksByPriority — falls back to priority
- AC-002 verified: TestRoundRobin_StaysUnderLimit — stays under sticky limit
- AC-003 verified: TestRoundRobin_RotatesAtLimit — rotates at limit
- AC-004 verified: TestRoundRobin_ExcludesModelLocked — model-locked skipped
- AC-005 verified: TestRoundRobin_StaysUnderLimit — count incremented on stay
- AC-006 verified: TestRoundRobin_RotatesAtLimit — count reset to 1 on rotate
- AC-007 verified: TestRoundRobin_DBPersistsUpdates — update payload contains fresh lastUsedAt + count

## Acceptance Criteria
- [x] No lastUsedAt picks first by priority
- [x] Stays on current until stickyLimit reached
- [x] Rotates to least-recently-used after limit
- [x] Excludes model-locked accounts
- [x] Increments consecutiveUseCount on stay
- [x] Resets consecutiveUseCount on rotate
- [x] DB is updated with new timestamps

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/credentials.go (StrategyRoundRobin branch + pickRoundRobin)
- Tests: internal/chatcore/credentials_test.go (covers both strategies)

## Description

Select provider credentials using `round-robin` strategy with sticky limit. Stays on current account for N consecutive requests before rotating. Sort by `lastUsedAt` ascending (least recent first). Resets counter on rotation.

## Input

- `string` (provider)
- `Set[string]` (excludeIds)
- `string|null` (model)
- `int` (stickyLimit)
- ProviderConnection repository

## Output

- `*Credentials`
- Updates `lastUsedAt` and `consecutiveUseCount` in DB

## Logic

1. Query available accounts (same filtering as fill-first)
2. Sort by `lastUsedAt ASC NULLS FIRST, priority ASC`
3. Check consecutiveUseCount for current top account
4. If < stickyLimit, stay on current
5. If >= stickyLimit, rotate to next by lastUsedAt
6. Update lastUsedAt and reset/increment consecutiveUseCount
7. Return selected credentials

## Acceptance Criteria
- [ ] No lastUsedAt picks first by priority
- [ ] Stays on current until stickyLimit reached
- [ ] Rotates to least-recently-used after limit
- [ ] Excludes model-locked accounts
- [ ] Increments consecutiveUseCount on stay
- [ ] Resets consecutiveUseCount on rotate
- [ ] DB is updated with new timestamps

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| First request | no lastUsedAt | First by priority |
| Under limit | count=2, limit=5 | Same account |
| At limit | count=5, limit=5 | Rotate |
| Multiple rotates | 10 requests, limit=3 | 3 rotations |
| Model lock | account locked | Skip to next |
