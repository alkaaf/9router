# Atomic Task: api-2 — Test `GET /api/usage/per-key/[keyId]/chart`

**Domain**: API Testing
**Priority**: Medium
**Estimated effort**: 25 min

---

## Input

- `src/app/api/usage/per-key/[keyId]/chart/route.js`
- Test database with usage data spread across time periods

## Output

- Integration tests for chart endpoint
- All tests passing

## Process

1. Setup: seed usage data spanning at least 30 days for chart testing
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Period "today" | `GET /api/usage/per-key/{uuid}/chart?period=today` → 200. `chartData` has hourly buckets. |
| 2 | Period "24h" | `GET /.../chart?period=24h` → 200. `chartData` has 24 hourly entries. |
| 3 | Period "7d" | `GET /.../chart?period=7d` → 200. `chartData` has 7 daily entries. |
| 4 | Period "30d" | `GET /.../chart?period=30d` → 200. `chartData` has 30 daily entries. |
| 5 | Period "60d" | `GET /.../chart?period=60d` → 200. `chartData` has 60 daily entries. |
| 6 | Invalid period | `GET /.../chart?period=invalid` → 400 |
| 7 | Non-existent key | `GET /.../chart?period=7d` (bad uuid) → 404 |
| 8 | Chart data accuracy | For a day with known usage (e.g., 1000 tokens, $0.50 cost), chart entry matches exactly |
| 9 | No data period | Key with no usage in requested period → chartData has entries but all `tokens: 0, cost: 0` |
| 10 | Response schema | Response has `keyId`, `period`, `chartData` (array of `{label, tokens, cost}`) |

## Dependencies

- be-6: Chart API route (DONE)
- api-1: API testing setup (DONE)

## Success Criteria

- All 10 test cases pass
- Chart data matches direct database calculation for all periods
