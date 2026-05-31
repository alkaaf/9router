# Atomic Task: api-1 — Test `GET /api/usage/per-key/[keyId]`

**Domain**: API Testing
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- `src/app/api/usage/per-key/[keyId]/route.js`
- Test database with API keys and usage data
- HTTP test client (supertest, fetch, or similar)

## Output

- Integration tests for the main per-key endpoint
- All tests passing

## Process

1. Setup: seed test database with:
   - 2 API keys with distinct UUIDs and full key strings
   - Usage data for both keys (entries in `usageHistory` + `usageDaily`)
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Valid keyId, default period | `GET /api/usage/per-key/{validUuid}` → 200. Response has `keyId`, `keyName`, `keyMasked`, `period` ("7d"), `stats`, `byModel`, `chartData`, `history`. |
| 2 | Valid keyId, explicit period | `GET /api/usage/per-key/{uuid}?period=24h` → 200. `period` field = "24h". |
| 3 | Invalid keyId | `GET /api/usage/per-key/nonexistent` → 404. Response: `{ error: "API key not found" }` |
| 4 | Invalid period | `GET /api/usage/per-key/{uuid}?period=invalid` → 400. Response: `{ error: "Invalid period" }` |
| 5 | `keyMasked` format | `keyMasked` = first 8 chars of full key + "..." + last 4 chars. E.g., `sk-abc1234...efgh` |
| 6 | `stats` structure | `stats` has: `totalRequests`, `totalPromptTokens`, `totalCompletionTokens`, `totalCost` — all numbers |
| 7 | `byModel` structure | Each entry has: `name`, `provider`, `requests`, `promptTokens`, `completionTokens`, `cost`, `lastUsed` |
| 8 | `chartData` structure | Each entry has: `label`, `tokens`, `cost` |
| 9 | `history` structure | Each entry has: `id`, `timestamp`, `model`, `provider`, `promptTokens`, `completionTokens`, `cost` |
| 10 | Data accuracy | `stats.totalRequests` = sum of `byModel[].requests`. `chartData` tokens match per-period aggregation. |

## Dependencies

- be-5: API route implementation (DONE)

## Success Criteria

- All 10 test cases pass
- Response schema matches documented API contract exactly
