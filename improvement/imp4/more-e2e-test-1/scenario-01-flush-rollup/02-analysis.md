# Scenario 01 — Flush + Rollup: Analysis

## Results Summary
- **Pass**: 2/4
- **Fail**: 2/4
- **Skipped**: 1 (conditional skip group)

## Test Results

- ✅ **bulk inserts 5 usageHistory rows** — Bulk INSERT with 60 positional params ($1..$60) succeeds. `db.run()` returns `{ changes: 5 }` correctly.
- ✅ **all 5 rollup tables exist with correct schema** — All five rollup tables confirmed present via `to_regclass()`.
- ❌ **reads usageHistory back with correct types** — `typeof rows[0].cost` is `"string"` instead of `"number"`.
- ❌ **usageDailyByProvider upsert works** — `typeof row.cost` is `"string"` instead of `"number"`.

## Root Cause

Both failures share the same root cause.

**Error**: `typeof row.cost === "string"` for NUMERIC(12,6) columns on PostgreSQL.
- `usageHistory.cost` — schema defines `NUMERIC(12,6)`
- `usageDailyByProvider.cost` — schema defines `NUMERIC(12,6)`
- Same applies to all 5 rollup tables (all use `NUMERIC(12,6)` for cost)

**Location**: `/Users/alkaaf/project/9router/src/lib/db/adapters/postgresAdapter.js` — `all()`, `get()`, `run()`, and transaction adapter variants all return `r.rows` (or `r.rows[0]`) directly without coercing pg's `NUMERIC` values to JavaScript numbers.

**Type**: `type-mapping` — pg's node-postgres driver returns `NUMERIC/DECIMAL` values as JavaScript strings (arbitrary precision). The adapter comment at line 9 claims pg "auto-parses on read" for JSONB but this only applies to JSON/JSONB; numeric types require explicit coercion.

**Severity**: `high` — `NUMERIC` cost columns are used throughout `usageRepo.js` (e.g., `aggregateEntryToDay`, rollup accumulation, `totalCost` calculations). Type inconsistency means `+` operations on cost values produce string concatenation instead of numeric addition, silently corrupting all cost aggregation.

**Reason**: pg's `pg` library (node-postgres) returns PostgreSQL `NUMERIC` columns as JavaScript strings by default (to preserve arbitrary precision). The adapter does not apply any type coercion for numeric columns. While `JSONB` auto-parses to objects (pg handles this), `NUMERIC(12,6)` does not auto-coerce to `number`. All the code that does `row.cost + otherCost` or `+= row.cost` will silently fail (string concatenation) if not explicitly coerced.

**Affected code paths** (from `usageRepo.js`):
- `aggregateEntryToDay()` at lines 44–47: `target[key].cost += values.cost` — receives string cost, produces string
- `_flushWriteQueue()` at line 323: `const cost = entry.cost || 0` — string `||` 0 doesn't help since `"0.15"` is truthy
- All `getUsageStats()` reads from `usageHistory` and rollup tables will also see string costs

**Recommended fix**: Add a coercion step in `postgresAdapter.js` for `NUMERIC` columns. The cleanest fix is to add a helper that walks returned rows and converts known `NUMERIC` column names (`cost`, `inputTokens`, `outputTokens`, `totalTokens`, `requestCount`) to `Number()`. Alternatively, use pg's `parseFloat8` / custom type parsers on the pool config to register numeric coercion for NUMERIC/OID 1700.

**Fix location**: `/Users/alkaaf/project/9router/src/lib/db/adapters/postgresAdapter.js` — `all()`, `get()`, transaction adapter's `get()`/`all()`, and the `prepare()` return values should coerce numeric string values from NUMERIC columns to `Number`.

## SQL Patterns Observed

### usageHistory bulk insert (usageRepo.js line 299–304)
```sql
INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
VALUES (?, ?, ...), (?, ?, ...), ...   -- one row-ph per entry
```
Uses positional `?` placeholders (translated to `$1, $2...` by the adapter's `convertPlaceholders()`).

### Rollup upsert (usageRepo.js lines 324–335)
```sql
INSERT INTO usageDailyByProvider(date, provider, requestCount, inputTokens, outputTokens, totalTokens, cost)
VALUES (?, ?, 1, ?, ?, ?, ?)
ON CONFLICT(date, provider) DO UPDATE SET
  requestCount = usageDailyByProvider.requestCount + 1,
  inputTokens  = usageDailyByProvider.inputTokens + EXCLUDED.inputTokens,
  outputTokens = usageDailyByProvider.outputTokens + EXCLUDED.outputTokens,
  totalTokens  = usageDailyByProvider.totalTokens + EXCLUDED.totalTokens,
  cost         = usageDailyByProvider.cost + EXCLUDED.cost,
  updatedAt    = NOW()
```
Each rollup table has its own dimension column (`provider`, `model`, `apiKeyId`, `accountId`, `endpoint`) with the same 7-column shape and identical ON CONFLICT DO UPDATE pattern.

### Rollup table shape (all 5 tables)
```sql
CREATE TABLE IF NOT EXISTS usageDailyByXXX (
  date         DATE NOT NULL,
  XXX          TEXT NOT NULL,
  requestCount BIGINT NOT NULL DEFAULT 0,
  inputTokens  BIGINT NOT NULL DEFAULT 0,
  outputTokens BIGINT NOT NULL DEFAULT 0,
  totalTokens  BIGINT NOT NULL DEFAULT 0,
  cost         NUMERIC(12,6) NOT NULL DEFAULT 0,
  updatedAt    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (date, XXX)
)
```
All 5 rollup tables share the same 8-column schema. `updatedAt` is set by the DB on each upsert.
