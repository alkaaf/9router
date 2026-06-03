# Domain 2: PostgreSQL Schema

## Overview

Create `src/lib/db/schema.postgres.js` — a parallel schema definition that mirrors
`schema.js` but uses PostgreSQL-native types (JSONB, TIMESTAMPTZ, BOOLEAN, BIGSERIAL,
NUMERIC). This file is consumed by the PostgreSQL adapter's `exec()` method to create
tables and indexes.

The SQLite schema (`schema.js`) is never modified. Both schemas coexist; the driver
selects which to use at runtime.

## Scope

- **In scope**: schema.postgres.js with all 12 tables, 19 indexes, PRAGMA_SQL_POSTGRES
- **Out of scope**: data migration script (Domain 7), K8s manifests (Domain 6)

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `src/lib/db/schema.postgres.js` | CREATE | ~300 |
| `src/lib/db/index.js` | MODIFY (import + pass schema to adapter) | +10 |
| `src/lib/db/driver.js` | MODIFY (pass schema.postgres to adapter) | +5 |

## Dependencies

Domain 1 (adapter foundation) must be done first — schema is applied through the adapter's `exec()`.

## Sub-Tasks

1. **Define PRAGMA_SQL_POSTGRES**
   - Description: PostgreSQL equivalent of SQLite PRAGMAs. Sets: `synchronous_commit
     = 'on'`, `shared_buffers = '256MB'`, `effective_cache_size = '1GB'`,
     `work_mem = '16MB'`, `timezone = 'UTC'`. These are `SET` commands executed once
     on connection acquisition.
   - Acceptance: Executing `PRAGMA_SQL_POSTGRES` via `adapter.exec()` succeeds.
     No equivalent of WAL/busy_timeout needed (PostgreSQL handles concurrency natively).
   - Risk: Low

2. **Create `_meta` table**
   - Description: `CREATE TABLE _meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`.
     Identical to SQLite — no type changes needed.
   - Acceptance: Table created. INSERT + SELECT round-trip works.
   - Risk: Low

3. **Create `settings` table**
   - Description: `id INTEGER PRIMARY KEY CHECK (id = 1)`, `data JSONB NOT NULL`.
     JSONB enables native JSON operators but the existing `parseJson`/`stringifyJson`
     pattern works unchanged (pg returns JSONB as JS objects). Keep CHECK constraint.
   - Acceptance: Single row insert/update works. `data` column stores and retrieves
     JS objects directly. CHECK (id=1) enforced.
   - Risk: Low

4. **Create `providerConnections` table**
   - Description: `id TEXT PRIMARY KEY`, `provider TEXT NOT NULL`, `authType TEXT NOT NULL`,
     `name TEXT`, `email TEXT`, `priority INTEGER`, `isActive BOOLEAN DEFAULT TRUE`,
     `data JSONB NOT NULL`, `createdAt TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
     `updatedAt TIMESTAMPTZ NOT NULL DEFAULT NOW()`. 3 indexes: `idx_pc_provider`,
     `idx_pc_provider_active` (composite), `idx_pc_priority` (composite).
   - Acceptance: All CRUD operations work. BOOLEAN column accepts true/false. TIMESTAMPTZ
     columns accept ISO strings and Date objects. Composite index on (provider, isActive)
     covers the hot-path filter.
   - Risk: Low

5. **Create `providerNodes` table**
   - Description: `id TEXT PRIMARY KEY`, `type TEXT`, `name TEXT`, `data JSONB NOT NULL`,
     `createdAt TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `updatedAt TIMESTAMPTZ NOT NULL
     DEFAULT NOW()`. Index: `idx_pn_type` on (type).
   - Acceptance: CRUD works. JSONB `data` stores JS objects.
   - Risk: Low

6. **Create `proxyPools` table**
   - Description: `id TEXT PRIMARY KEY`, `isActive BOOLEAN DEFAULT TRUE`, `testStatus TEXT`,
     `data JSONB NOT NULL`, `createdAt TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
     `updatedAt TIMESTAMPTZ NOT NULL DEFAULT NOW()`. Indexes: `idx_pp_active`, `idx_pp_status`.
   - Acceptance: CRUD works. BOOLEAN handling correct.
   - Risk: Low

7. **Create `apiKeys` table**
   - Description: `id TEXT PRIMARY KEY`, `key TEXT UNIQUE NOT NULL`, `name TEXT`,
     `machineId TEXT`, `isActive BOOLEAN DEFAULT TRUE`, `createdAt TIMESTAMPTZ NOT NULL
     DEFAULT NOW()`. Index: `idx_ak_key` (redundant with UNIQUE, kept for parity).
   - Acceptance: `validateApiKey` hot path works — `SELECT isActive FROM apiKeys
     WHERE key = $1` uses UNIQUE index. UNIQUE constraint on `key` enforced.
   - Risk: Low

8. **Create `combos` table**
   - Description: `id TEXT PRIMARY KEY`, `name TEXT UNIQUE NOT NULL`, `kind TEXT`,
     `models JSONB NOT NULL`, `createdAt TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
     `updatedAt TIMESTAMPTZ NOT NULL DEFAULT NOW()`. Index: `idx_combo_name`.
   - Acceptance: CRUD works. JSONB `models` stores JS arrays.
   - Risk: Low

9. **Create `kv` table**
   - Description: `scope TEXT NOT NULL`, `key TEXT NOT NULL`, `value TEXT NOT NULL`,
     `PRIMARY KEY (scope, key)`. Index: `idx_kv_scope` on (scope).
   - Acceptance: Composite PK enforces uniqueness. All kvStore operations work.
   - Risk: Low

10. **Create `usageHistory` table**
    - Description: `id BIGSERIAL PRIMARY KEY`, `timestamp TIMESTAMPTZ NOT NULL`,
      `provider TEXT`, `model TEXT`, `connectionId TEXT`, `apiKey TEXT`, `endpoint TEXT`,
      `promptTokens BIGINT DEFAULT 0`, `completionTokens BIGINT DEFAULT 0`,
      `cost NUMERIC(12,6) DEFAULT 0`, `status TEXT`, `tokens JSONB`, `meta JSONB`.
      6 indexes: `idx_uh_ts` (timestamp DESC), `idx_uh_provider`, `idx_uh_model`,
      `idx_uh_conn`, `idx_uh_apiKey`, `idx_uh_apiKey_ts` (composite).
    - Acceptance: Batch INSERT of 50 rows works. Time-range queries use idx_uh_ts.
      NUMERIC(12,6) preserves cost precision. BIGSERIAL auto-increments correctly.
    - Risk: Low (this is the most critical table but schema is straightforward)

11. **Create `usageDaily` table**
    - Description: `dateKey DATE PRIMARY KEY`, `data JSONB NOT NULL`. No additional
      indexes needed (PK covers lookups). JSONB enables future GIN indexing if
      dashboard starts filtering by `byApiKey` keys.
    - Acceptance: UPSERT by dateKey works. JSONB `data` stores and retrieves the
      nested `{byProvider, byModel, byApiKey, byAccount, byEndpoint}` structure.
    - Risk: Low

12. **Create `requestDetails` table**
    - Description: `id TEXT PRIMARY KEY`, `timestamp TIMESTAMPTZ NOT NULL`,
      `provider TEXT`, `model TEXT`, `connectionId TEXT`, `status TEXT`,
      `data JSONB NOT NULL`. 4 indexes: `idx_rd_ts`, `idx_rd_provider`,
      `idx_rd_model`, `idx_rd_conn`.
    - Acceptance: Self-pruning DELETE works. Paginated SELECT works. JSONB `data`
      stores full request/response payload.
    - Risk: Low

13. **Export schema builder functions**
    - Description: Export `SCHEMA_VERSION`, `TABLES_POSTGRES`, `PRAGMA_SQL_POSTGRES`,
      and a `buildCreateTableSql(name, def)` helper (same as SQLite version). The
      adapter's `init()` calls `exec(PRAGMA_SQL_POSTGRES)` then iterates
      `TABLES_POSTGRES` calling `exec(buildCreateTableSql(...))` for each table,
      then `exec(index)` for each index.
    - Acceptance: Calling `initSchema(adapter)` creates all tables and indexes.
      Idempotent — running twice does not error (uses IF NOT EXISTS).
    - Risk: Low

## Effort Estimate

2-3 days

## Risk Level

Low — type mapping is mechanical; no logic changes.

## Testing Requirements

- Unit tests: Yes — verify each CREATE TABLE statement is valid PostgreSQL syntax
  (can be done by executing against Docker PG and checking no error)
- Integration tests: Yes — create schema, then verify CRUD via adapter
- Manual testing: Connect to PG via psql, verify tables exist with correct types
