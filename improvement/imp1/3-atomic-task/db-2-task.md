# Atomic Task: db-2 — Verify Additive Migration System

**Domain**: Database
**Priority**: Medium
**Estimated effort**: 10 min

---

## Input

- `src/lib/db/schema.js` with new `idx_uh_apiKey` index (from db-1)
- `src/lib/db/migrations.js` — `syncSchemaFromTables()` function
- Existing SQLite database with data

## Output

- Confirmation that additive migration works correctly
- No data loss, no index conflicts

## Process

1. Verify `syncSchemaFromTables()` reads `TABLES` and `indexes` from `schema.js`
2. Confirm the function uses `CREATE INDEX IF NOT EXISTS` — idempotent
3. Verify no data migration is needed:
   - `usageDaily.data.byApiKey` already contains `apiKey` field in each entry
   - `usageHistory` already has `apiKey` column
4. Check that existing indexes (`idx_uh_ts`, `idx_uh_provider`, `idx_uh_model`, `idx_uh_conn`) are not affected

### Verification Steps

| Step | Action | Expected |
|------|--------|----------|
| 1 | Start app with existing DB | No migration errors |
| 2 | Check DB after boot | `idx_uh_apiKey` exists in `sqlite_master` |
| 3 | Restart app again | No duplicate index errors (idempotent) |
| 4 | Query existing data | All `usageHistory` and `usageDaily` rows intact |
| 5 | Verify existing indexes | All 4 original indexes still present |

## Dependencies

- db-1: Index added to schema.js (DONE)

## Success Criteria

- New index created without errors
- Existing data and indexes unaffected
- Migration is idempotent (can run multiple times safely)
