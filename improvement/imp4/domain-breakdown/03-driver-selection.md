# Domain 3: Driver Selection (driver.js Update)

## Overview

Update `src/lib/db/driver.js` to add PostgreSQL as the highest-priority adapter when
`POSTGRES_URL` is set. The existing SQLite fallback chain is preserved as-is when
`POSTGRES_URL` is empty/undefined. This is the decision point that makes the entire
migration opt-in via environment variable.

## Scope

- **In scope**: `tryPostgres()` function, insertion into init chain, fallback to SQLite
- **Out of scope**: adapter implementation (Domain 1), schema creation (Domain 2),
  repository changes (Domain 4+)

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `src/lib/db/driver.js` | MODIFY | +25 |
| `src/lib/db/adapters/postgresAdapter.js` | Already created in Domain 1 | — |
| `.env.example` | MODIFY (add POSTGRES_URL docs) | +5 |

## Dependencies

Domain 1 must be complete (postgresAdapter.js exists).

## Sub-Tasks

1. **Add `tryPostgres()` function**
   - Description: Check `process.env.POSTGRES_URL`. If empty/undefined/null → return
     `null` (skip). Otherwise, dynamically import `createPostgresAdapter` from
     `./adapters/postgresAdapter.js`. Call with config derived from env:
     - `connectionString`: `process.env.POSTGRES_URL`
     - `poolSize`: `parseInt(process.env.POSTGRES_POOL_SIZE || "20", 10)`
     - `ssl`: `process.env.POSTGRES_SSL === "true"`
     Wrap in try/catch — on failure, log warning and return `null` (triggers SQLite
     fallback).
   - Acceptance: `POSTGRES_URL` unset → `tryPostgres()` returns null. `POSTGRES_URL`
     set to valid DSN → returns a PostgreSQL adapter. Invalid DSN → logs warning,
     returns null.
   - Risk: Low

2. **Update init chain to prefer PostgreSQL**
   - Description: In `initAdapter()` (or equivalent entrypoint), insert PostgreSQL
     as the first attempt:
     ```
     let adapter = await tryPostgres();
     if (!adapter) adapter = await tryBunSqlite();
     if (!adapter) adapter = await tryBetterSqlite();
     if (!adapter) adapter = await tryNodeSqlite();
     if (!adapter) adapter = await trySqlJs();
     ```
   - Acceptance: With `POSTGRES_URL` set, adapter.driver === "postgres". Without it,
     adapter.driver is one of the SQLite variants (unchanged behavior).
   - Risk: Low

3. **Guard PRAGMA_SQL execution to SQLite-only**
   - Description: The existing `PRAGMA_SQL` (WAL, busy_timeout, etc.) is currently
     executed on every adapter init. Move the execution behind a SQLite check:
     `if (adapter.driver !== "postgres") await adapter.exec(PRAGMA_SQL)`. The
     PostgreSQL adapter's `checkpoint()` is a no-op, so no WAL timer is started.
   - Acceptance: PostgreSQL adapter init does NOT execute SQLite PRAGMAs. SQLite
     adapters still execute them.
   - Risk: Low

4. **Add env var documentation**
   - Description: Document `POSTGRES_URL`, `POSTGRES_POOL_SIZE`, `POSTGRES_SSL` in
     `.env.example` or a config README snippet. Include example connection strings
     for local Docker, managed PG (RDS/Cloud SQL), and K8s.
   - Acceptance: `.env.example` has the three new variables with descriptions.
   - Risk: Low

## Effort Estimate

1-2 days

## Risk Level

Low — purely additive change to the existing selection chain.

## Testing Requirements

- Unit tests: Yes — `tryPostgres()` with various env var combinations
- Integration tests: Yes — start app with/without `POSTGRES_URL`, verify correct
  adapter selected
- Manual testing: `POSTGRES_URL=postgres://test:test@localhost:5432/9router npm start`
  → verify PostgreSQL adapter is used. Unset → verify SQLite fallback.
