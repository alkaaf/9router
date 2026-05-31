# Atomic Task: but-1 — Test `getUsageStats()` with `filter.apiKey`

**Domain**: Backend Unit Testing
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- `src/lib/db/repos/usageRepo.js` — `getUsageStats(period, filter)` function
- Test database setup (in-memory SQLite via `getAdapter()`)
- Known test data: multiple API keys with usage in `usageHistory` and `usageDaily`

## Output

- Test file with 5+ test cases
- All tests passing
- Coverage report showing `getUsageStats` paths exercised

## Process

1. Create test file `src/lib/db/repos/__tests__/usageRepo.perKey.test.js`
2. Setup: create in-memory DB, run schema sync, insert test data:
   - 2 API keys: `sk-test-key-A`, `sk-test-key-B`
   - Usage entries for both keys across multiple days
   - Populate both `usageHistory` and `usageDaily` tables
3. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | `getUsageStats("7d", { apiKey: "sk-test-key-A" })` | Returns stats ONLY for key A. `totalRequests` matches key A's data. `byApiKey` contains only key A entries. |
| 2 | `getUsageStats("7d", { apiKey: "sk-test-key-B" })` | Returns stats ONLY for key B. Totals match key B. |
| 3 | `getUsageStats("7d", {})` (no filter) | Returns stats for ALL keys combined. `totalRequests` = key A + key B. |
| 4 | `getUsageStats("7d", { apiKey: "sk-nonexistent" })` | Returns stats with all totals = 0. `byApiKey` empty. |
| 5 | `getUsageStats("today", { apiKey: "sk-test-key-A" })` | Live history path filters correctly. Only today's entries for key A. |
| 6 | `getUsageStats("24h", { apiKey: "sk-test-key-A" })` | Same as today, different cutoff. |
| 7 | Verify `lastUsed` timestamp | `lastUsed` field reflects the most recent entry for the filtered key, not all keys. |

## Dependencies

- be-2: `getUsageStats()` extended with `filter.apiKey` (DONE)
- Test framework: Jest/Vitest already configured in project

## Success Criteria

- All 7 test cases pass
- No existing tests broken
- `getUsageStats` with no filter returns identical results as before (backward compatibility)
