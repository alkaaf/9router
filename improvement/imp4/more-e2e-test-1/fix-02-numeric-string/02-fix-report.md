# Fix Report: Bug #2 — NUMERIC columns returned as strings

## Summary
Fixed PostgreSQL `pg` driver returning NUMERIC and BIGINT columns as JavaScript strings, which caused string concatenation in cost aggregation (`"0.03" + "0.05" = "0.030.05"`).

## What was changed

**File:** `src/lib/db/adapters/postgresAdapter.js`
**Location:** Lines 21–29 (after `const { Pool } = pg;` import)

Added type parsers to coerce numeric PostgreSQL types to JS numbers:

```js
pg.types.setTypeParser(1700, (val) => val === null ? null : parseFloat(val));  // NUMERIC
pg.types.setTypeParser(20, (val) => val === null ? null : parseInt(val, 10));  // BIGINT
pg.types.setTypeParser(23, (val) => val === null ? null : parseInt(val, 10));  // INT4
pg.types.setTypeParser(21, (val) => val === null ? null : parseInt(val, 10));  // INT2
pg.types.setTypeParser(700, parseFloat);  // FLOAT4
pg.types.setTypeParser(701, parseFloat);  // FLOAT8
```

The type parsers are registered globally at adapter initialization time. All subsequent queries through the pool will return numbers for these types instead of strings.

## OIDs registered

| OID | PostgreSQL Type | JS Conversion |
|-----|----------------|---------------|
| 1700 | NUMERIC | parseFloat |
| 20 | BIGINT | parseInt(base 10) |
| 23 | INT4 | parseInt(base 10) |
| 21 | INT2 | parseInt(base 10) |
| 700 | FLOAT4 | parseFloat |
| 701 | FLOAT8 | parseFloat |

## Affected columns now correctly typed

- `usageHistory.cost` (NUMERIC(12,6))
- `usageHistory.promptTokens` (INTEGER)
- `usageHistory.completionTokens` (INTEGER)
- Rollup table columns: `requestCount`, `inputTokens`, `outputTokens`, `totalTokens` (all BIGINT)
- Rollup table: `cost` (NUMERIC(12,6))

## Test Results

**Test command:**
```
POSTGRES_URL="postgres://rahasia:rahasia@localhost:5433/9router_test" vitest run unit/db-adapter-postgres.test.js unit/db-pg-e2e-flush.test.js unit/db-pg-e2e-schema.test.js --reporter=verbose
```

**Results:**
| Suite | Pass | Fail | Skip |
|-------|------|------|------|
| db-adapter-postgres.test.js | 11 | 0 | 1 |
| db-pg-e2e-flush.test.js | 4 | 0 | 1 |
| db-pg-e2e-schema.test.js | 4 | 1 | 1 |
| **Total** | **21** | **1** | **3** |

**Key passing tests for Bug #2:**
- ✓ `reads usageHistory back with correct types` — verified NUMERIC/INTEGER types are JS numbers
- ✓ `numeric types (NUMERIC(12,6)) handle decimal correctly` — decimal precision preserved as number

## Remaining Issues

**1 failure unrelated to Bug #2:**
- `column id should exist in usageHistory` in `db-pg-e2e-schema.test.js`
- This is **Bug #1** (column case sensitivity), explicitly out of scope for this fix
- The test expects `id` but pg returns lowercase column names — separate agent handles this

## Files Modified

- `src/lib/db/adapters/postgresAdapter.js` — Added pg.types.setTypeParser calls at module load time
