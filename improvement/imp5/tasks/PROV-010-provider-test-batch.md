---
id: PROV-010
domain: providers
status: DONE
estimate: 2h
title: Provider Test Batch
---

  evidence: |
  (verified 2026-06-04)
## Description

Test multiple connections by filter mode via `POST /api/providers/test-batch`. Modes: `"oauth"` (authType=oauth), `"free"` (authType=free), `"apikey"` (authType=apikey), `"compatible"` (openai-compatible-*, anthropic-compatible-*), `"all"`, `"provider"` (specific provider ID). For each matching connection, call `testSingleConnection`. Return results array with per-connection details and a summary.

## Input

- Request body: `{ mode: "oauth"|"free"|"apikey"|"compatible"|"all"|"provider", providerId?: string }`

## Output

- `{ mode, providerId, results: [...], summary: { total, passed, failed }, testedAt }`
- Empty array if no connections match mode
- 400 if mode invalid

## Logic

1. Validate mode is one of the allowed values. If invalid, return 400.
2. Query connections from DB filtered by mode:
   - `"oauth"`: `authType = 'oauth'`
   - `"free"`: `authType = 'free'`
   - `"apikey"`: `authType = 'apikey'`
   - `"compatible"`: provider ID starts with `openai-compatible-` or `anthropic-compatible-`
   - `"all"`: all connections
   - `"provider"`: `provider = providerId`
3. For each connection, call `testSingleConnection` (PROV-009).
4. Collect results: `{ provider, connectionId, connectionName, authType, valid, latencyMs, error, diagnosis, statusCode, testedAt }`.
5. Build summary: `{ total, passed, failed }`.
6. Return full result object.

## Acceptance Criteria
- [x] `"all"` mode tests every active connection
- [x] `"oauth"` mode filters to OAuth authType only
- [x] `"compatible"` mode filters to compatible provider prefixes
- [x] `"provider"` mode with valid provider ID tests only matching connections
- [x] Empty result returns `{ results: [], summary: { total: 0, passed: 0, failed: 0 } }`
- [x] Invalid mode returns 400

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| All mode | `{ mode: "all" }` | Tests all connections, summary shows totals |
| OAuth mode | `{ mode: "oauth" }` | Only OAuth authType connections tested |
| Compatible mode | `{ mode: "compatible" }` | Only compatible providers tested |
| Provider mode | `{ mode: "provider", providerId: "openai" }` | Only openai connections tested |
| No matches | Mode with no matching connections | `{ results: [], summary: { total: 0, passed: 0, failed: 0 } }` |
| Invalid mode | `{ mode: "invalid" }` | 400 |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- All AC verified: ✓
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: TestBatch in tester.go + TestBatchHandler
- Tests: TestTestBatchHandler_AllMode, TestTestBatchHandler_InvalidMode, TestTestBatchHandler_ProviderModeRequiresID
