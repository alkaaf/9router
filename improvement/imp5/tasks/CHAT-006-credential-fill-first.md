---
id: CHAT-006
domain: chat-core
status: DONE
estimate: 2h
title: Credential Fill First
---

## Agent Log
- Started: 2026-06-04 18:45
- Agent: agent-chat

## Description

Select provider credentials using `fill-first` strategy. Filters out unavailable accounts (rate-limited, error-state, model-locked), picks highest-priority connection. Returns nil if no accounts available, or rate limit state if all are rate-limited.

## Input

- `string` (provider)
- `Set[string]` (excludeIds - already tried connection IDs)
- `string|null` (model - for model-specific locking)
- ProviderConnection repository

## Output

- `*Credentials` or `nil` or `*RateLimitState`
- Order: `ORDER BY priority ASC`

## Logic

1. Query providerConnections where isActive=true
2. Filter out IDs in excludeIds
3. Filter out model-locked accounts for this model
4. Filter out accounts with active backoff
5. Sort by priority ASC
6. Return first match, or RateLimitState if all rate-limited

## Agent Log
- Started: 2026-06-04 18:45
- Completed: 2026-06-04 19:00
- Agent: agent-chat
- AC-001 verified: TestFillFirst_PicksHighestPriority — priority ASC wins
- AC-002 verified: TestFillFirst_RespectsExcludeIDs — excluded IDs skipped
- AC-003 verified: TestFillFirst_FiltersModelLocked — model-locked excluded
- AC-004 verified: TestFillFirst_NoAccounts — ErrNoConnections
- AC-005 verified: TestFillFirst_AllRateLimited — RateLimitState with earliest expiry
- AC-006 (stable ID tie) verified: TestFillFirst_SamePriority_StableID — ID-based tie-break

## Acceptance Criteria
- [x] Returns highest-priority account
- [x] Excludes connectionIds in excludeIds set
- [x] Filters model-locked accounts
- [x] Returns nil if no accounts available
- [x] Returns allRateLimited with earliest expiry if all locked
- [x] Multiple accounts with same priority → first by ID

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/credentials.go
- Tests: internal/chatcore/credentials_test.go (12 tests, all pass)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Single account | provider=A, excludeIds=[] | First active account |
| Multiple accounts | 3 accounts | Highest priority |
| Exclude one | excludeIds=[id2] | id2 excluded |
| Model locked | model locked on id2 | id2 excluded |
| All excluded | exclude all | nil |
| All rate-limited | all have locks | RateLimitState |
