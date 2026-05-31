# Atomic Task: but-2 — Test `getChartData()` with `filter.apiKey`

**Domain**: Backend Unit Testing
**Priority**: High
**Estimated effort**: 25 min

---

## Input

- `src/lib/db/repos/usageRepo.js` — `getChartData(period, filter)` function
- Test database with `usageHistory` and `usageDaily` entries for multiple API keys

## Output

- Test cases covering all period paths with `filter.apiKey`
- All tests passing

## Process

1. Extend existing test setup (from but-1) with chart-relevant data:
   - Entries spread across multiple time buckets (hours for today/24h, days for 7d+)
   - `usageDaily` JSON with `byApiKey` entries for both keys
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | `getChartData("today", { apiKey: "sk-test-key-A" })` | Returns time-series with ONLY key A's data. SQL `WHERE apiKey = ?` used. |
| 2 | `getChartData("24h", { apiKey: "sk-test-key-A" })` | Returns hourly buckets for key A only. |
| 3 | `getChartData("7d", { apiKey: "sk-test-key-A" })` | Parses `usageDaily.byApiKey`, sums only key A entries per day. 7 data points. |
| 4 | `getChartData("30d", { apiKey: "sk-test-key-A" })` | Same as 7d, 30 data points. Tokens/cost match direct JSON calculation. |
| 5 | `getChartData("7d", { apiKey: "sk-nonexistent" })` | Returns empty chartData array or all zeros. |
| 6 | `getChartData("7d", {})` (no filter) | Returns combined data for all keys (existing behavior). |
| 7 | Verify `tokens` field | `tokens = promptTokens + completionTokens` for each bucket. |
| 8 | Verify `cost` field | `cost` matches sum of `cost` from matching `byApiKey` entries. |

## Dependencies

- be-3: `getChartData()` extended with `filter.apiKey` (DONE)
- but-1 (test setup reuse)

## Success Criteria

- All 8 test cases pass
- Chart data for filtered key matches manual calculation from raw data
- No-filter behavior unchanged (backward compatible)
