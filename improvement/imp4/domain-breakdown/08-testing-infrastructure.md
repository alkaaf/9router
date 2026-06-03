# Domain 8: Testing Infrastructure

## Overview

Create Docker Compose configuration for local PostgreSQL testing, integration test
suite for the PostgreSQL adapter and repositories, and CI pipeline configuration
to run PostgreSQL tests alongside existing SQLite tests.

The Docker Compose setup provides a reproducible PostgreSQL 16 instance with
tmpfs for fast test runs. Integration tests run against this instance via
`POSTGRES_URL=postgres://test:test@localhost:5432/9router_test`.

## Scope

- **In scope**: Docker Compose, integration test directory structure, CI config,
  test helpers/utilities
- **Out of scope**: Load testing (separate effort), TimescaleDB testing (future)

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `docker-compose.test.yml` | CREATE | ~40 |
| `tests/integration/postgres/setup.js` | CREATE | ~50 |
| `tests/integration/postgres/adapter.test.js` | CREATE | ~80 |
| `tests/integration/postgres/settings.test.js` | CREATE | ~40 |
| `tests/integration/postgres/apiKeys.test.js` | CREATE | ~40 |
| `tests/integration/postgres/connections.test.js` | CREATE | ~50 |
| `tests/integration/postgres/usage.test.js` | CREATE | ~100 |
| `tests/integration/postgres/requestDetails.test.js` | CREATE | ~40 |
| `tests/integration/postgres/helpers.js` | CREATE | ~60 |
| `.github/workflows/ci.yml` | MODIFY | +30 |

## Dependencies

Domains 1-4 must be complete (adapter works, schema exists, repos migrated).

## Sub-Tasks

1. **Create Docker Compose for testing**
   - Description: `docker-compose.test.yml` with `postgres:16-alpine` service.
     Environment: `POSTGRES_DB=9router_test`, `POSTGRES_USER=test`, `POSTGRES_PASSWORD=test`.
     Ports: `5432:5432`. tmpfs mount at `/var/lib/postgresql/data` for fast
     ephemeral storage. Health check: `pg_isready -U test`. No volume mounts
     (data is throwaway for tests).
   - Acceptance: `docker compose -f docker-compose.test.yml up -d` starts PostgreSQL.
     `docker compose -f docker-compose.test.yml exec postgres psql -U test -d
     9router_test -c "SELECT 1"` returns 1.
   - Risk: Low

2. **Create test setup helper (`setup.js`)**
   - Description: Before all tests: start Docker Compose if not running, wait for
     health check, run schema creation via adapter. After all tests: truncate all
     tables, close adapter, optionally stop Docker Compose. Export `setupPostgresTest()`
     and `teardownPostgresTest()`.
   - Acceptance: Calling `setupPostgresTest()` returns a connected adapter with
     all tables created. Calling `teardownPostgresTest()` cleans up.
   - Risk: Low

3. **Create integration test helpers (`helpers.js`)**
   - Description: Shared utilities: `createTestAdapter()`, `truncateAllTables(adapter)`,
     `seedSettings(adapter)`, `seedApiKey(adapter)`, `seedConnection(adapter)`,
     `compareWithSqlite(adapter, sqliteAdapter, testFn)` — runs the same test against
     both adapters and asserts identical results.
   - Acceptance: All helper functions work correctly. `compareWithSqlite` catches
     any behavioral divergence.
   - Risk: Low

4. **Write adapter integration tests (`adapter.test.js`)**
   - Description: Test the full adapter interface against PostgreSQL:
     - `run()`: INSERT, UPDATE, DELETE — verify `{ changes, lastInsertRowid }` shape
     - `get()`: SELECT single row, SELECT no rows → `undefined`
     - `all()`: SELECT multiple rows, SELECT no rows → `[]`
     - `exec()`: DDL execution (CREATE TABLE, CREATE INDEX)
     - `transaction()`: commit on success, rollback on error
     - `prepare()`: prepared statement run N times
     - `checkpoint()`: no-op, does not throw
     - `close()`: pool ends, subsequent queries throw
     - Placeholder translation: `?` → `$1, $2` works for 0-10 params
     - JSONB: store and retrieve nested objects
     - Boolean: store and retrieve true/false
     - Concurrent: 10 parallel transactions on different keys all succeed
   - Acceptance: All adapter tests pass. Adapter behavior matches SQLite adapter
     exactly (use `compareWithSqlite`).
   - Risk: Low

5. **Write repository integration tests**
   - Description: For each of the 10 migrated repos, write integration tests that:
     - Create, read, update, delete records via the repo
     - Run against both SQLite and PostgreSQL adapters
     - Verify identical return values (use `compareWithSqlite`)
     Specific tests:
     - `settings.test.js`: getSettings, updateSettings, cache invalidation
     - `apiKeys.test.js`: validateApiKey hot path, CRUD
     - `connections.test.js`: getProviderConnections filtered, reorderInTx, CRUD
     - `usage.test.js`: saveRequestUsage, flushWriteQueue, getUsageStats,
       getChartData, getUsageHistory, getRecentLogs — **most comprehensive**
     - `requestDetails.test.js`: saveRequestDetail, getRequestDetails paginated,
       self-pruning trim
   - Acceptance: All repo tests pass with PostgreSQL adapter. Output shapes match
     SQLite exactly.
   - Risk: Medium (usage tests are complex due to the flush pipeline)

6. **Write concurrent correctness tests**
   - Description: Port/extend the existing `tests/unit/db-concurrent.test.js` to
     run against PostgreSQL:
     - 100 parallel `saveRequestUsage` → totalRequestsLifetime === 100
     - 200 parallel `saveRequestDetail` → all flushed
     - Mixed concurrent usage + details + connections → no data loss
   - Acceptance: Same invariants hold under PostgreSQL as under SQLite.
   - Risk: Medium (concurrency bugs may surface)

7. **Update CI pipeline**
   - Description: Add a PostgreSQL test job to `.github/workflows/ci.yml` (or
     equivalent). Use `services: postgres` in GitHub Actions, or spin up via
     `docker compose`. Run the integration test suite. Keep existing SQLite tests
     as the default fast path; PostgreSQL tests run on PRs that touch DB code.
     SQLite-only tests run on every commit.
   - Acceptance: CI runs both SQLite and PostgreSQL test suites. PostgreSQL tests
     pass on every PR that modifies DB code.
   - Risk: Low

8. **Create test database seeding script**
   - Description: `tests/integration/postgres/seed.js` — creates test data: 5
     provider connections, 10 API keys, 1000 usage history rows across 30 days,
     200 request details. Used for performance benchmarks and manual testing.
   - Acceptance: Running seed script creates representative test data in <5 seconds.
   - Risk: Low

## Effort Estimate

2-3 days (plus ongoing test maintenance)

## Risk Level

Low — well-defined test boundaries, existing test patterns to follow.

## Testing Requirements

- Unit tests: N/A (this domain creates the tests)
- Integration tests: Yes — this domain IS the integration tests
- Manual testing: `docker compose -f docker-compose.test.yml up` → run tests →
  verify all pass
