# Scenario 07 — Schema Migration / Idempotency — Analysis

## Test Results Summary
- **Total tests**: 5 (4 active + 1 always-skipped placeholder)
- **Passed**: 3
- **Failed**: 2
- **Skipped**: 1 (no-POSTGRES_URL path — N/A, PG URL was set)

```
✓ all 16 tables (11 base + 5 rollup) exist in PG         (26ms)
× all expected columns exist in usageHistory              (5ms)
✓ schema.postgres.js DDL is idempotent (re-apply)        (10ms)
✓ inserts work on re-initialized schema                  (2ms)
× numeric types (NUMERIC(12,6)) handle decimal correctly  (3ms)
```

Driver initialized: `[DB] Driver: postgres` — driver selection + DDL bootstrap on the PG URL succeeded.

## Failed Tests — Root Cause Analysis

### Failure 1 — "all expected columns exist in usageHistory"
**Test code (line 25):**
```js
const rows = await db.all("SELECT * FROM usageHistory LIMIT 0");
const actual = rows.length === 0 ? [] : Object.keys(rows[0]);
// ...expects "id" to be present
```

**Root cause: test discovery heuristic, not a schema defect.**

The test relies on `SELECT * FROM usageHistory LIMIT 0` to introspect column names via `Object.keys(rows[0])`. With an empty result set (`rows.length === 0`), the test falls through to the `actual = []` branch, so the loop never finds any column.

Looking at `postgresAdapter.js` lines 177-180 — `all()` returns `r.rows` from `pg` directly. `pg` returns column metadata in `r.fields`, but `r.rows` is `[]` for `LIMIT 0` queries. The test's `Object.keys(rows[0])` introspection is therefore broken in PG (and would be broken in any adapter that returns an empty array for `LIMIT 0`).

**Schema itself is correct** — the existence test ("all 16 tables exist") already passed, and the schema for `usageHistory` declares `id BIGSERIAL PRIMARY KEY` at `schema.postgres.js:172`. An alternative introspection using `information_schema.columns` would succeed:

```js
const cols = await db.all(`SELECT column_name FROM information_schema.columns WHERE table_name='usagehistory'`);
```

**Verdict: test bug, not implementation bug.**

### Failure 2 — "numeric types (NUMERIC(12,6)) handle decimal correctly"
**Test code (line 49):**
```js
expect(typeof row.cost).toBe("number");
```

**Actual:** `typeof row.cost === "string"`.

**Root cause: pg default type-parser returns NUMERIC as string.**

Node-postgres (`pg`) ships NUMERIC/DECIMAL columns as strings by default to preserve precision (avoids the IEEE-754 float round-trip that JS `Number` would impose). The schema in `schema.postgres.js` declares cost as `NUMERIC(12,6)` (line 181, 220, 235, 250, 265, 280), and `postgresAdapter.js` does NOT register a custom type parser for `pg.types.builtins.NUMERIC` (1700). Result: `row.cost` is `"0.001234"`, not `0.001234`.

This is a design choice, not a bug — keeping NUMERIC as a string is the only way to round-trip `0.001234` without lossy float coercion. But the test's assertion `typeof === "number"` doesn't match this contract.

**Two ways to fix (analysis only — not applying):**
1. **Test-side fix**: assert `expect(Number(row.cost)).toBeCloseTo(0.001234, 6)` and stop checking the JS type. Or use `pg.types.setTypeParser(1700, parseFloat)` in a test setup hook.
2. **Adapter-side fix**: register a parser in `postgresAdapter.js`:
   ```js
   import { types } from "pg";
   types.setTypeParser(1700, parseFloat); // 1700 = NUMERIC OID
   ```
   This would break the round-trip guarantee for high-precision values (anything > 9 digits of mantissa or > 6 digits of exponent loses precision in JS doubles), so the current string-default is intentional.

**Verdict: contract mismatch, not a functional bug.** Cost is being stored and retrieved correctly — just typed as string by `pg`.

## Passed Tests — What They Confirm

| Test | What it validates |
|------|-------------------|
| `all 16 tables (11 base + 5 rollup) exist` | `to_regclass()` resolves all 11 base tables (`_meta`, `settings`, `providerConnections`, `providerNodes`, `proxyPools`, `apiKeys`, `combos`, `kv`, `usageHistory`, `usageDaily`, `requestDetails`) and all 5 rollup tables (`usageDailyByProvider`, `usageDailyByModel`, `usageDailyByApiKey`, `usageDailyByAccount`, `usageDailyByEndpoint`). |
| `schema.postgres.js DDL is idempotent` | `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` clauses in the bundled DDL can be re-executed without error. `getPostgresSchema()` returns one bundle; `adapter.exec()` splits on `;` and runs each (see `postgresAdapter.js:186-191`). |
| `inserts work on re-initialized schema` | `ON CONFLICT (id) DO UPDATE` upsert pattern works. JSONB auto-serialization of `{postinit:"test"}` works on the PG adapter (no manual `JSON.stringify` needed). The `r.changes` shape is `{changes: number}`. |

## Top-Level Finding

The PostgreSQL schema/migration pipeline is functionally correct:
- Driver selection works
- DDL is fully idempotent (re-runs cleanly)
- Table creation, JSONB serialization, upsert semantics, and rollup-table coverage all work
- `NUMERIC(12,6)` stores and retrieves the value to 6 decimal places

The two test failures are both **assertion-side issues** in the test file:
1. `LIMIT 0` introspection is not portable across adapters (works for SQLite's `pragma_table_info`, fails for `pg` which doesn't return column metadata on empty result sets)
2. `pg` returns NUMERIC as string by default; `typeof === "number"` assertion is incorrect for this adapter

Neither failure indicates a real bug in the production code path under test.
