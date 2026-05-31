# Atomic Task: but-3 — Test `getUsageHistory()` with `filter.apiKey`

**Domain**: Backend Unit Testing
**Priority**: Medium
**Estimated effort**: 15 min

---

## Input

- `src/lib/db/repos/usageRepo.js` — `getUsageHistory(filter)` function
- Test database with `usageHistory` entries for multiple API keys

## Output

- Test cases for `getUsageHistory` with `apiKey` filter
- All tests passing

## Process

1. Setup: insert 10+ `usageHistory` rows for key A, 10+ for key B, mixed timestamps
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | `getUsageHistory({ apiKey: "sk-test-key-A", limit: 5 })` | Returns max 5 rows, ALL for key A. |
| 2 | `getUsageHistory({ apiKey: "sk-test-key-A", limit: 200 })` | Returns all key A rows (capped at 200). |
| 3 | `getUsageHistory({ apiKey: "sk-test-key-A", offset: 5 })` | Returns key A rows starting from offset 5. |
| 4 | `getUsageHistory({ apiKey: "sk-test-key-A", provider: "openai" })` | Returns key A rows filtered by BOTH apiKey AND provider. |
| 5 | `getUsageHistory({ apiKey: "sk-nonexistent" })` | Returns empty array. |
| 6 | `getUsageHistory({})` (no filter) | Returns all rows (existing behavior unchanged). |

## Dependencies

- be-4: `getUsageHistory()` extended with `filter.apiKey` (DONE)
- but-1 (test setup reuse)

## Success Criteria

- All 6 test cases pass
- SQL query includes `apiKey = ?` condition when filter provided
- Pagination (limit/offset) works correctly with apiKey filter
