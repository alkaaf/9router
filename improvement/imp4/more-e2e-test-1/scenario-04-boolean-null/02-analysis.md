# Scenario 04 Analysis: BOOLEAN + NULL Handling

## Test Results: 4 FAILED / 1 PASSED / 1 SKIPPED

### Failed Tests

| Test | Issue |
|------|-------|
| `isActive=true stores and retrieves as boolean` | `row.isActive` is `undefined` |
| `isActive=false stores and retrieves as boolean` | `row.isActive` is `undefined` |
| `providerConnections isActive works` | `row.isActive` is `undefined` |
| `NULL connectionId stays NULL` | `row.connectionId` is `undefined` (should be `null`) |

### Passed Tests

| Test | Result |
|------|--------|
| `NULL in JSONB field (meta) stays null` | PASS |

## Root Cause

**PostgresAdapter column name case-sensitivity bug**

The `postgresAdapter` uses `rows[0]` (lowercase) to access columns from `pg` driver, but the `get()` method in `postgresAdapter.js` uses `row[col.name]` with original case-sensitive column names.

```javascript
// Problem in postgresAdapter get() method:
const { rows } = await this.client.query(sql, params);
const row = rows[0];
// ...
return row[col.name];  // col.name may have different casing than actual PG result
```

PostgreSQL returns lowercase column names (`isactive`, `connectionid`) but the code may be looking for `isActive` with mixed case.

**NULL being returned as undefined**: The `postgresAdapter.get()` result handler converts `undefined` columns to `undefined` instead of preserving PostgreSQL's `null` as JavaScript `null`.

## Key Findings

1. **BOOLEAN columns**: All 3 tests fail because `row.isActive` is `undefined` - the column value is not being retrieved correctly from the result set.

2. **NULL handling**: The TEXT column `connectionId` with NULL value is being returned as `undefined` instead of `null`.

3. **JSONB NULL**: The JSONB field `meta` correctly preserves `null` values inside JSON objects - only direct column NULLs are affected.

## Impact

- All BOOLEAN-based filtering/conditional logic will be broken (isActive checks always evaluate to falsy)
- NULL connection handling in usage tracking will fail silently
