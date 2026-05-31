# Atomic Task: but-5 — Test Schema Index Creation

**Domain**: Backend Unit Testing
**Priority**: Low
**Estimated effort**: 10 min

---

## Input

- `src/lib/db/schema.js` — `indexes` array
- `src/lib/db/schema.js` — `syncSchemaFromTables()` function

## Output

- Test confirming `idx_uh_apiKey` index is created
- Test confirming additive migration doesn't break existing indexes

## Process

1. Create fresh in-memory database
2. Run `syncSchemaFromTables()` 
3. Query `sqlite_master` to verify indexes:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Verify `idx_uh_apiKey` exists | `SELECT name FROM sqlite_master WHERE type='index' AND name='idx_uh_apiKey'` returns the index |
| 2 | Verify all existing indexes still exist | `idx_uh_ts`, `idx_uh_provider`, `idx_uh_model`, `idx_uh_conn` all present |
| 3 | Verify idempotency | Running `syncSchemaFromTables()` twice doesn't error or create duplicates |
| 4 | Verify index on `usageHistory(apiKey)` | Index covers the `apiKey` column specifically |

## Dependencies

- db-1: `idx_uh_apiKey` added to schema.js (DONE)
- db-2: `syncSchemaFromTables()` verified (DONE)

## Success Criteria

- All 4 test cases pass
- Index creation is idempotent
