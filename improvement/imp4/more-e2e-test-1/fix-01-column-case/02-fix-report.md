# Bug #1 Fix Report — PostgreSQL Column Name Case Sensitivity

## Bug Summary
The `pg` Node.js driver lowercases all unquoted column names in result rows. The
`postgresAdapter.js` `get()` and `all()` methods returned `r.rows[0]` / `r.rows`
directly, so when SQL was `SELECT isActive FROM ...`, the row key was
`isactive`. Application code reading `row.isActive` got `undefined`.

## Fix Approach
Add an adapter-level row post-processor that maps lowercase keys back to their
camelCase equivalents defined in the schema. Implemented as a hardcoded
allow-list of camelCase columns (derived from
`src/lib/db/schema.postgres.js`):

- `isActive`
- `connectionId`
- `apiKey`, `apiKeyId`
- `createdAt`, `updatedAt`
- `promptTokens`, `completionTokens`
- `requestCount`, `inputTokens`, `outputTokens`, `totalTokens`
- `dateKey`, `currentDate`, `startDate`, `endDate`

All-lowercase columns (id, key, name, data, provider, model, etc.) need no
mapping because they round-trip unchanged.

## Files Modified

1. **`/Users/alkaaf/project/9router/src/lib/db/adapters/postgresAdapter.js`**
   - Added module-level `CAMEL_CASE_COLUMNS` array and derived
     `LOWER_TO_CAMEL` map
   - Added `transformRowKeys(row)` helper that renames lowercase keys to
     camelCase when a mapping exists
   - Applied `transformRowKeys` in 4 sites:
     - Main adapter `get()` — `return transformRowKeys(r.rows[0]);`
     - Main adapter `all()` — `return r.rows.map(transformRowKeys);`
     - `getPreparedEntry().get` — `return transformRowKeys(r.rows[0]);`
     - `getPreparedEntry().all` — `return r.rows.map(transformRowKeys);`
   - The transaction adapter's `get()`/`all()` reuse the main adapter's
     `prepare()` path (via `parent.prepare(sql, client)`) and therefore
     inherit the transformation automatically.

2. **`/Users/alkaaf/project/9router/tests/unit/db-adapter-postgres.test.js`**
   - Updated one assertion in the "BOOLEAN columns return JS booleans" test
     (lines 102-103) to read `row.isActive` (camelCase) instead of the
     previous `row.isactive` (lowercase). The previous test was codifying
     the bug; the application code uses camelCase, so the test must match.

## Test Results

```
Test Files  2 passed (2)
     Tests  18 passed | 2 skipped (20)
```

- `tests/unit/db-adapter-postgres.test.js`: 13/13 passed
  (1 test in the file is meta-skipped because the describe is only run when
  `POSTGRES_URL` is set, and a separate skipped describe for the
  `!POSTGRES_URL` path)
- `tests/unit/db-pg-e2e-boolean-null.test.js`: 5/5 passed
  - BOOLEAN columns > isActive=true stores and retrieves as boolean
  - BOOLEAN columns > isActive=false stores and retrieves as boolean
  - BOOLEAN columns > providerConnections isActive works
  - NULL handling > NULL connectionId stays NULL
  - NULL handling > NULL in JSONB field (meta) stays null

Raw output saved to:
`/Users/alkaaf/project/9router/improvement/imp4/more-e2e-test-1/fix-01-column-case/01-raw-output.txt`

## Why Not Double-Quote Identifiers In The SQL?

An alternative would be to change every SELECT in the repos to use
`SELECT "isActive" ...` so pg preserves case. This is more invasive (touches
every repo that reads camelCase columns) and still leaves a footgun: a new
developer writing `SELECT isActive` would silently get the bug. The
adapter-level transformer is invisible to callers, requires no changes to
repos, and is centralized so the camelCase list lives in one place.

## Out Of Scope
- Bug #2 (NUMERIC precision)
- Bug #3 (NULL handling for other columns)
