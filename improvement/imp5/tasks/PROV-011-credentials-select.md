---
id: PROV-011
domain: providers
status: DONE
estimate: 3h
title: Credentials Select
---

  evidence: |
  (verified 2026-06-04)
## Description

Select an active connection for a provider from the connections table via `getProviderCredentials`. Filter by provider ID and `isActive=true`. Exclude connection IDs in `excludeSet` (for retry loop). Exclude model-locked connections (modelLock_${model} or modelLock___all with future expiry). Apply fallback strategy from settings: `"fill-first"` (priority-sorted first) or `"round-robin"` (least-recently-used with sticky round-robin limit). Resolve proxy config from `providerSpecificData.proxyPoolId` or direct config. For no-auth free providers, return a virtual "noauth" credentials object.

## Input

- Provider ID string
- `excludeConnectionIds` (Set, string, or null) — connection IDs to skip
- `model` string (for per-model lock check)
- `options.preferredConnectionId` (optional pin)

## Output

- `credentials` object: `{ authType, apiKey, accessToken, refreshToken, projectId, connectionName, providerSpecificData (with resolved proxy), connectionId, testStatus, lastError, _connection }`
- `null` if no connections available
- `{ allRateLimited: true, retryAfter, retryAfterHuman, lastError, lastErrorCode }` if all accounts locked

## Logic

1. Resolve provider ID (PROV-002).
2. Query active connections for provider where `isActive = true`.
3. Filter out `excludeConnectionIds`.
4. Filter out model-locked connections (PROV-022). For `modelLock___all`, lock applies to all models.
5. Resolve fallback strategy from settings (global + per-provider override).
6. If `preferredConnectionId` set and available, select it directly.
7. Else apply strategy:
   - **fill-first**: return first connection (sorted by priority)
   - **round-robin**: check `consecutiveUseCount`. If < `stickyLimit`, return same connection. If at limit, select least-recently-used (`lastUsedAt`) and reset count.
8. Resolve proxy config from `providerSpecificData.proxyPoolId` or direct `connectionProxyUrl`.
9. For noAuth providers, return virtual credentials without DB read.
10. Update `lastUsedAt` and `consecutiveUseCount` in DB.

## Acceptance Criteria
- [x] Provider with one active connection returns that connection
- [x] Fill-first returns highest priority connection
- [x] Round-robin sticky: same connection for N requests, then switches to LRU
- [x] Excluded connection IDs are not selected
- [x] Model-locked connections excluded (allRateLimited with retryAfter on all locked)
- [x] Noauth provider returns virtual credentials without DB read
- [x] All accounts unavailable returns `allRateLimited: true`

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Single connection | One active connection for provider | Returns that connection |
| Fill-first priority | Multiple: priority 1 and 2 | Returns priority 1 |
| Round-robin sticky | Same connection called N times | Same connection until stickyLimit |
| Round-robin switch | After stickyLimit uses | Selects LRU connection |
| Exclude set | excludeSet contains priority 1 | Returns next available |
| Model lock | All connections have active modelLock | allRateLimited: true with retryAfter |
| NoAuth provider | opencode with noAuth=true | Virtual credentials returned |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- All AC verified: ✓
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: SelectCredentials in lifecycle.go
- Tests: TestSelectCredentials_FillFirst, TestSelectCredentials_ExcludeSet, TestSelectCredentials_NoAuthProvider, TestSelectCredentials_ModelLockAllRateLimited, TestSelectCredentials_RoundRobinSticky, TestSelectCredentials_ResolveAlias
