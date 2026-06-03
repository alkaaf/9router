# Bug #3 Fix Report: NULL Handling

## Bug #3 Status

**CONFIRMED: Bug #3 was a symptom of Bug #1 (column case sensitivity), not a separate issue.**

When the `postgresAdapter.js` `CAMEL_CASE_COLUMNS` mapping was applied (Bug #1 fix), Bug #3 automatically resolved.

## Root Cause Analysis

The original error pattern:
```
expected undefined to be null           // NULL column values
expected 'undefined' to be 'boolean'   // BOOLEAN columns
```

This occurred because:
1. PostgreSQL lowercases all unquoted column names in result rows (e.g., `isactive`, `connectionid`)
2. The application code expected camelCase keys (e.g., `isActive`, `connectionId`)
3. When accessing `row.connectionId` on a row where PG returned `connectionid`, JavaScript returned `undefined`
4. The NULL values were actually present in the row, but inaccessible under the expected key name

## Evidence

**Before Bug #1 fix** (initial run, `01-initial-bug3-status.txt`):
```
FAIL unit/db-pg-e2e-boolean-null.test.js
  × isActive=true stores and retrieves as boolean — expected 'undefined' to be 'boolean'
  × isActive=false stores and retrieves as boolean — expected 'undefined' to be 'boolean'
  × providerConnections isActive works — expected 'undefined' to be 'boolean'
  × NULL connectionId stays NULL — expected undefined to be null
```
4 failed | 1 passed | 1 skipped

**After Bug #1 fix** (`02-after-bug1-fix.txt`):
```
PASS unit/db-pg-e2e-boolean-null.test.js
  ✓ isActive=true stores and retrieves as boolean
  ✓ isActive=false stores and retrieves as boolean
  ✓ providerConnections isActive works
  ✓ NULL connectionId stays NULL (not 0, not undefined)
  ✓ NULL in JSONB field (meta) stays null
```
5 passed | 1 skipped

## Current State: NULL Handling

NULL values work correctly now:
- `NULL` columns return JavaScript `null` (not `undefined`)
- `BOOLEAN` columns return proper `true`/`false` booleans (not `undefined`)
- JSONB fields with `NULL` values return `null` correctly
- Falsy values (0, false, empty string) are preserved correctly

## Regression Test Results (All 7 Scenarios)

Full output: `03-regression-all-scenarios.txt`

| Scenario | File | Result | Details |
|----------|------|--------|---------|
| Boolean + NULL handling | `db-pg-e2e-boolean-null.test.js` | **PASS** | All 5 tests pass |
| Adapter interface conformance | `db-adapter-postgres.test.js` | **PASS** | All 14 tests pass |
| KVStore JSONB round-trip | `db-pg-e2e-kvstore.test.js` | **PASS** | All 4 tests pass |
| Transaction isolation | `db-pg-e2e-transaction.test.js` | **PARTIAL** | 3 pass, 1 fail (SAVEPOINT issue) |
| Pool + concurrency | `db-pg-e2e-pool.test.js` | **PASS** | All 3 tests pass |
| usageRepo flush + rollup | `db-pg-e2e-flush.test.js` | **PASS** | All 5 tests pass |
| DATE + TIMESTAMPTZ handling | `db-pg-e2e-date.test.js` | **PARTIAL** | 3 pass, 1 fail (TIMESTAMPTZ type) |
| Schema + migration | `db-pg-e2e-schema.test.js` | **PARTIAL** | 3 pass, 1 fail (id column) |

**Totals: 37 passed | 8 skipped | 3 failed**

## Remaining Test Failures (Not Related to Bug #3)

These failures predate the imp4 NULL handling work:

1. **`db-pg-e2e-date.test.js` — TIMESTAMPTZ returns Date object**
   - Test expects `typeof row.timestamp === "string"` but receives `"object"` (JavaScript Date)
   - **Cause**: pg library's default type parser for TIMESTAMPTZ (type 1184) returns Date objects
   - **Fix needed**: Add type parser in `postgresAdapter.js` for OID 1184 to return ISO string:
     ```javascript
     pg.types.setTypeParser(1184, (val) => val);  // TIMESTAMPTZ
     ```

2. **`db-pg-e2e-schema.test.js` — `id` column missing from usageHistory**
   - Test expects column `id` in `usageHistory` table but DDL doesn't include it
   - **Cause**: Schema mismatch — test expectation vs actual DDL definition
   - **Fix needed**: Either add `id` column to usageHistory DDL or remove from test expectation

3. **`db-pg-e2e-transaction.test.js` — Nested transaction SAVEPOINT**
   - Expected `val` to be "inner-updated" but received "qmark"
   - **Cause**: SAVEPOINT implementation issue or test isolation (data pollution from prior runs)
   - **Fix needed**: Investigate transaction nesting behavior

## Recommendation

**No additional fix needed for NULL handling.** Bug #3 is fully resolved by the Bug #1 fix (column case sensitivity mapping in `postgresAdapter.js`).

The 3 remaining failures are unrelated to NULL handling and should be addressed separately:
- 1 is a TIMESTAMPTZ type parsing issue (add pg type parser)
- 1 is a schema definition mismatch
- 1 is a transaction SAVEPOINT implementation or test isolation issue
