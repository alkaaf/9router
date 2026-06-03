# Scenario 05: DATE + TIMESTAMPTZ Handling — Analysis

## Summary

| Metric              | Value |
|---------------------|-------|
| Total tests         | 5     |
| Passed              | 4     |
| Failed              | 1     |
| Skipped             | 0     |

## Pass/Fail Detail

| # | Test | Status |
|---|------|--------|
| 1 | TIMESTAMPTZ accepts ISO 8601 string and returns as string | FAIL |
| 2 | DATE accepts YYYY-MM-DD string | PASS |
| 3 | DATE range query works | PASS |
| 4 | TIMESTAMPTZ comparison with date range works | PASS |
| 5 | (skipped placeholder) | SKIPPED |

## Failure Detail

**Test:** TIMESTAMPTZ accepts ISO 8601 string and returns as string

**Error:** `expected 'object' to be 'string'` at line 20 (`typeof row.timestamp`)

**Actual type returned:** `object` (a JavaScript `Date` instance)

**Expected type:** `string`

## Root Cause

The `pg` (node-postgres) library's default type parser converts PostgreSQL `TIMESTAMPTZ` values to JavaScript `Date` objects on read, not to ISO 8601 strings. This is standard and documented pg behavior — it applies `text_parse` to result rows which invokes the `Date` constructor for timestamp types.

The test assertion `expect(typeof row.timestamp).toBe("string")` is therefore incorrect for the PostgreSQL adapter. The value IS correctly retrieved from the database; it just arrives as a `Date` object rather than a string.

**Relevant code:** `src/lib/db/adapters/postgresAdapter.js` — the `get()` and `all()` methods return `r.rows[0]` directly from `pool.query()` with no type coercion for timestamp columns (by design; pg handles deserialization).

## Impact on imp4 Migration

Any downstream code that assumes `row.timestamp` or `row.date` is a string (e.g. string concatenation, `.includes("T")`, `.split("T")`) will break when the PostgreSQL adapter is used instead of a SQLite adapter. SQLite adapters return raw text from the database, so timestamps arrive as strings there. The pg adapter returns `Date` objects.

## Schema Confirmation (from `src/lib/db/schema.postgres.js`)

- `usageHistory.timestamp` → `TIMESTAMPTZ NOT NULL` (line 173)
- `usageDailyByProvider.date` → `DATE NOT NULL` (line 214)
- `usageDaily.dateKey` → `DATE PRIMARY KEY` (line 206)
- `usageDailyByProvider.updatedAt` → `TIMESTAMPTZ NOT NULL DEFAULT NOW()` (line 221)

Both DATE and TIMESTAMPTZ columns are confirmed in the schema. DATE columns also return `Date` objects from pg (but the test did not assert on their type, so it passed).

## Recommendation

Code that consumes timestamp/date values from the db layer should either:
1. Call `.toISOString()` on the returned `Date` object before string operations, or
2. Use `pg`'s `types.setTypeParser(types.builtins.TIMESTAMPTZ, val => val)` to override the parser and return raw strings (at the cost of losing Date convenience), or
3. Add a type-normalization layer in the adapter that converts Date objects to ISO strings for timestamp/date columns.
