# Domain 7: Data Migration (SQLite → PostgreSQL)

## Overview

Create `scripts/migrate-sqlite-to-postgres.js` — a one-time data migration script
that copies all data from the existing SQLite database to a fresh PostgreSQL instance.
Also creates `scripts/migrate-postgres-to-sqlite.js` (rollback).

The script handles type conversion (TEXT timestamps → TIMESTAMPTZ, INTEGER booleans →
BOOLEAN, TEXT JSON → JSONB), streams large tables (usageHistory) to avoid loading
everything into memory, and verifies row counts after migration.

## Scope

- **In scope**: migrate-sqlite-to-postgres.js, migrate-postgres-to-sqlite.js,
  type conversion, streaming for large tables, verification, rollback
- **Out of scope**: online dual-write migration (future), in-place schema migration

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `scripts/migrate-sqlite-to-postgres.js` | CREATE | ~150 |
| `scripts/migrate-postgres-to-sqlite.js` | CREATE | ~100 |

## Dependencies

Domains 1-2 must be complete (both adapters working, both schemas created).

## Sub-Tasks

1. **Create migration script skeleton**
   - Description: Script accepts `--sqlite <path>` and `--postgres <dsn>` CLI args.
     Opens SQLite via better-sqlite3 (synchronous, fast). Opens PostgreSQL via
     the postgresAdapter. Creates both schemas. Logs progress for each table.
   - Acceptance: `node scripts/migrate-sqlite-to-postgres.js --sqlite ./data.db
     --postgres postgres://...` connects to both databases, prints "Connected to
     SQLite (N rows in X tables)" and "Connected to PostgreSQL".
   - Risk: Low

2. **Implement type conversion helpers**
   - Description: `convertRow(row, tableName)` transforms a SQLite row to PostgreSQL:
     - `TEXT` timestamps → pass through (pg accepts ISO strings for TIMESTAMPTZ)
     - `INTEGER` booleans (`isActive`, etc.) → `row.isActive ? true : false`
     - `TEXT` JSON columns (`data`, `tokens`, `meta`, `models`) → `JSON.parse(value)`
       if non-null (pg JSONB accepts JS objects)
     - `REAL` cost → pass through (NUMERIC(12,6) accepts JS numbers)
     - `INTEGER` id → pass through (BIGSERIAL auto-generates, but we need the
       original id for FK references during data load)
   - Acceptance: Converted rows insert into PostgreSQL without type errors.
   - Risk: Medium (edge cases: null JSON, malformed JSON, null timestamps)

3. **Migrate small tables first (settings, _meta, kv, combos, apiKeys, etc.)**
   - Description: For tables with <10k rows, load all rows at once:
     `const rows = sqlite.all("SELECT * FROM table")` → convert → multi-row INSERT
     into PostgreSQL. Process order: `_meta` → `settings` → `kv` → `combos` →
     `apiKeys` → `providerNodes` → `proxyPools` → `providerConnections`.
     Truncate PostgreSQL table before INSERT (clean slate).
   - Acceptance: Row counts match. Spot-check a few rows for data integrity.
   - Risk: Low

4. **Migrate usageHistory with streaming**
   - Description: For the potentially millions of rows in usageHistory, use a cursor:
     `const stmt = sqlite.prepare("SELECT * FROM usageHistory ORDER BY id")` then
     iterate with `stmt.step()` in batches of 10,000. For each batch: convert rows,
     build multi-row INSERT, execute. This avoids loading all rows into memory.
   - Acceptance: All usageHistory rows migrated. Memory usage stays under 100MB
     regardless of source table size. Progress logged every 10k rows.
   - Risk: Medium (large datasets; need to handle interruptions gracefully)

5. **Migrate usageDaily**
   - Description: Load all usageDaily rows (typically <365). Convert JSON blob to
     JSONB (parse → pass as object). Also populate the normalized rollup tables
     (usage_daily_by_provider, etc.) by parsing each day's JSON blob and inserting
     into rollup tables. This ensures the new read paths work immediately after migration.
   - Acceptance: usageDaily rows match. Rollup tables have correct aggregated totals
     that sum to the JSON blob values.
   - Risk: Medium (JSON blob parsing + redistribution to 5 tables)

6. **Migrate requestDetails**
   - Description: Load all requestDetails rows (capped at observabilityMaxRecords,
     typically <200). Convert data column to JSONB.
   - Acceptance: Row counts match. Data integrity verified.
   - Risk: Low

7. **Verification step**
   - Description: After all tables migrated, run verification queries:
     - Row count comparison: `SELECT COUNT(*)` on each table in both databases
     - Spot checks: sample 10 random rows from each table, compare key fields
     - usageHistory: verify MIN(id), MAX(id), total cost sum matches
     - usageDaily: verify date range matches
   - Acceptance: All counts match. No type errors in PostgreSQL.
   - Risk: Low

8. **Rollback script (postgres → sqlite)**
   - Description: Reverse of the above. Reads from PostgreSQL, converts types back,
     writes to SQLite. Less commonly needed but essential for safety.
   - Acceptance: Rollback restores SQLite to pre-migration state (or close enough
     for manual reconciliation).
   - Risk: Low

## Effort Estimate

2-3 days

## Risk Level

Medium — large table streaming and JSON blob parsing have edge cases.

## Testing Requirements

- Unit tests: Yes — type conversion helpers, batch INSERT builder
- Integration tests: Yes — end-to-end migration against Docker PostgreSQL with
  a seeded SQLite database, verify row counts and data integrity
- Manual testing: Run against a real 9Router SQLite database, verify all data
  appears correctly in PostgreSQL dashboard
