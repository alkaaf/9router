# imp4 Strategy — PostgreSQL Migration with Dual-Database Support

**Project**: 9Router Database Migration to PostgreSQL
**Status**: Strategy complete
**Date**: 2026-06-03
**Architecture**: Choose-able by env — PostgreSQL when `POSTGRES_URL` is set, SQLite otherwise

---

## 1. Executive Summary

### Problem
9Router uses SQLite as its database, which is file-based and single-process. For Kubernetes deployment with multiple replicas/pods, SQLite cannot support:
- Concurrent writes from multiple pods
- Connection pooling
- Proper time-series data (usageHistory)
- JSONB for fast JSON querying
- Horizontal scaling

### Solution
Add PostgreSQL as a **second-class adapter** alongside SQLite. The system chooses the database at runtime based on the `POSTGRES_URL` environment variable:
- `POSTGRES_URL` is set and non-empty → use PostgreSQL
- `POSTGRES_URL` is empty/undefined → use SQLite (existing behavior preserved)

### Key Principles
1. **Zero breaking changes** — existing SQLite deployments work without any configuration changes
2. **Same interface** — both adapters implement identical API surface
3. **Incremental migration** — each repository can be migrated independently
4. **Testable locally** — Docker Compose for PostgreSQL testing

---

## 2. Architecture Design

### 2.1 Adapter Selection Flow

```
Application Start
       │
       ▼
  check POSTGRES_URL
       │
       ├── empty/undefined ──→ tryBetterSqlite() ──→ SQLite (default)
       │                            ├── tryNodeSqlite() (Node ≥22.5)
       │                            └── trySqlJs() (fallback)
       │
       └── has value ──→ tryPostgres() ──→ PostgreSQL (primary)
                            │
                            └── failure ──→ tryBetterSqlite() ──→ SQLite (fallback)
```

### 2.2 Adapter Interface Specification

Both adapters MUST implement this interface:

| Method | SQLite | PostgreSQL | Notes |
|--------|--------|------------|-------|
| `run(sql, params)` | ✅ | ✅ | Execute statement, no return |
| `get(sql, params)` | ✅ | ✅ | Single row, `{}` if empty |
| `all(sql, params)` | ✅ | ✅ | All rows, `[]` if empty |
| `exec(sql, params)` | ✅ | ✅ | Run + return `{lastInsertRowId, changes}` |
| `transaction(fn)` | ✅ | ✅ | Execute `fn` in transaction |
| `checkpoint()` | ✅ | N/A | SQLite WAL checkpoint only — no-op for PG |
| `close()` | ✅ | ✅ | Clean up connections |
| `raw` | ✅ | ✅ | Raw driver access (for special ops) |
| `prepare(sql)` | ✅ | ✅ | Return prepared statement |

### 2.3 Key Behavioral Differences

#### Placeholder Translation
- **SQLite**: `?` positional placeholders
- **PostgreSQL**: `$1, $2, $3, ...` positional placeholders

**Approach**: The `postgresAdapter.js` handles this internally via `_convert(sql)`. Callers use `?` consistently, adapter translates on each call.

#### JSON Handling
- **SQLite**: Store as TEXT, use `stringifyJson()`/`parseJson()` helpers from `helpers/jsonCol.js`
- **PostgreSQL**: Store as JSONB, pass JavaScript objects directly (pg driver handles serialization)

**Approach**: Callers always pass JS objects. Adapter handles serialization:
- SQLite: `JSON.stringify(obj)` before `run()` / `JSON.parse(str)` after `get()`/`all()`
- PostgreSQL: Pass object directly, pg driver handles JSONB serialization

#### Boolean Handling
- **SQLite**: `BOOLEAN` stored as `INTEGER` (0/1), manual conversion in `rowToKey()` etc.
- **PostgreSQL**: Native `BOOLEAN` type

**Approach**: Repository code already uses JS booleans with `=== 1 || === true` checks (e.g., `apiKeysRepo.js:11`). PostgreSQL adapter returns native boolean.

#### Timestamps
- **SQLite**: ISO string (`TEXT`)
- **PostgreSQL**: `TIMESTAMPTZ`

**Approach**: Repository code uses ISO strings (e.g., `getLocalDateKey()` in `usageRepo.js:37-40`). PostgreSQL adapter stores as `TIMESTAMPTZ`.

#### `changes`/`lastInsertRowid` Coercion
- **SQLite**: `bun:sqlite` and `node:sqlite` explicitly coerce to `Number` (e.g., `bunSqliteAdapter.js:42`, `nodeSqliteAdapter.js:55`)
- **PostgreSQL**: `pg` driver returns these as strings by default

**Approach**: PostgreSQL adapter coerces `result.count` (Number) and `rows[0]?.id` for consistency.

### 2.4 Connection Pooling (PostgreSQL)

- Pool size: 20 connections per pod (configurable via `POSTGRES_POOL_SIZE`)
- `idleTimeoutMillis`: 30000
- `connectionTimeoutMillis`: 10000
- Singleton pattern: preserved via `global._dbAdapter` for Next.js hot-reload

### 2.5 Singleton Preservation

For Next.js hot-reload, the adapter uses a singleton pattern via `global._dbAdapter` (see `driver.js:4-5`).

### 2.6 Current SQLite Adapters

| Adapter | Runtime | Type | Notes |
|---------|---------|------|-------|
| `bunSqliteAdapter` | Bun only | Async factory, sync ops | Native `bun:sqlite`, fastest under Bun |
| `betterSqliteAdapter` | Node only | Sync | `better-sqlite3` C++ binding, fastest on Node |
| `nodeSqliteAdapter` | Node ≥22.5 | Async factory, sync ops | Built-in `node:sqlite`, SAVEPOINT for transactions |
| `sqljsAdapter` | Any (fallback) | Async factory, sync ops | WASM-based, in-memory with disk persistence |

### 2.7 Security Considerations

- **API keys** stored in plaintext — require TLS to PostgreSQL (`sslmode=require` minimum), encrypted at-rest
- **Settings JSON** contains OIDC client secrets, outbound proxy URL — same encryption requirements
- **Connection string** in env / K8s Secret — use sealed/external secret store, never ConfigMap
- **Cross-pod cache invalidation** for settings — in-process cache is per-pod; consider PostgreSQL `LISTEN/NOTIFY` or Redis pub/sub for faster propagation

---

## 3. Adapter Implementation Plan

### 3.1 Files to Create

| File | Lines | Effort | Risk |
|------|-------|--------|------|
| `src/lib/db/adapters/postgresAdapter.js` | ~250 | Small | Low |
| `src/lib/db/schema.postgres.js` | ~250 | Small | Low |
| `src/lib/db/driver.js` (modify) | +30 | Trivial | Low |

### 3.2 Files to Modify (Repositories)

The actual codebase has **11 repositories**, not the 11 named in preliminary analysis. Here is the accurate breakdown:

#### Easy Repos (10) — Remove JSON parsing only

| Repository | LOC | Changes | Notes |
|------------|-----|---------|-------|
| `settingsRepo.js` | 118 | Remove `parseJson`/`stringifyJson` calls | `row.data` returns parsed object from JSONB |
| `apiKeysRepo.js` | 75 | Remove `parseJson`/`stringifyJson` calls | `rowToKey` already handles boolean coercion |
| `connectionsRepo.js` | 226 | Remove `parseJson`/`stringifyJson` calls | Sort in SQL instead of JS (`ORDER BY priority ASC NULLS LAST`) |
| `proxyPoolsRepo.js` | 103 | Remove `parseJson`/`stringifyJson` calls | |
| `requestDetailsRepo.js` | 200 | Remove `parseJson`/`stringifyJson` calls | Self-pruning logic stays the same |
| `aliasRepo.js` | 62 | Remove `parseJson`/`stringifyJson` calls | |
| `combosRepo.js` | 73 | Remove `parseJson`/`stringifyJson` calls | |
| `disabledModelsRepo.js` | 56 | Remove `parseJson`/`stringifyJson` calls | |
| `nodesRepo.js` | 95 | Remove `parseJson`/`stringifyJson` calls | |
| `pricingRepo.js` | 111 | Remove `parseJson`/`stringifyJson` calls | |

#### Hard Repo (1) — Major Rewrite

| Repository | LOC | Changes | Risk |
|------------|-----|---------|------|
| **`usageRepo.js`** | **913** | **Rewrite `_flushWriteQueue` for PostgreSQL** | **HIGH** |

`usageRepo.js` is the critical path:
- 3 DB writes per flush (1 history row + 1 daily upsert + 1 counter update)
- JSON aggregation in JS (`aggregateEntryToDay`) — must become SQL-based
- `usageDaily` JSON blob rewrite pattern is hostile to PostgreSQL (last-write-wins race)
- SSE `statsEmitter` is process-local — must consider cross-pod pub/sub
- `getUsageStats` has a full-scan overlay for `lastUsed` timestamps — must use window functions

### 3.3 Schema Tables (10 tables)

| Table | SQLite PK | PostgreSQL PK | Key Changes |
|-------|-----------|---------------|-------------|
| `_meta` | — (composite via `scope, key`) | Same | No change |
| `settings` | `id INTEGER CHECK (id=1)` | Same | `data` → JSONB |
| `providerConnections` | `id TEXT` | `id UUID DEFAULT gen_random_uuid()` | `isActive` → BOOLEAN, `data` → JSONB, timestamps → TIMESTAMPTZ |
| `providerNodes` | `id TEXT` | Same | `data` → JSONB, timestamps → TIMESTAMPTZ |
| `proxyPools` | `id TEXT` | Same | `isActive` → BOOLEAN, `data` → JSONB, timestamps → TIMESTAMPTZ |
| `apiKeys` | `id TEXT` | Same | `isActive` → BOOLEAN, timestamps → TIMESTAMPTZ |
| `combos` | `id TEXT` | Same | `models` → JSONB, timestamps → TIMESTAMPTZ |
| `kv` | Composite (`scope, key`) | Same | No change |
| `usageHistory` | `id INTEGER AUTOINCREMENT` | `id BIGSERIAL` | `cost` → NUMERIC(12,6), timestamps → TIMESTAMPTZ, `tokens`/`meta` → JSONB |
| `usageDaily` | `dateKey TEXT` | `dateKey DATE` | `data` → JSONB |
| `requestDetails` | `id TEXT` | Same | `data` → JSONB, timestamps → TIMESTAMPTZ |

### 3.4 Type Mapping: SQLite → PostgreSQL

| SQLite Type | PostgreSQL Type | Notes |
|-------------|-----------------|-------|
| `TEXT` (general) | `TEXT` | — |
| `TEXT` (timestamp) | `TIMESTAMPTZ` | Timezone-aware |
| `TEXT` (JSON) | `JSONB` | Binary JSON with indexing |
| `INTEGER` (boolean) | `BOOLEAN DEFAULT TRUE` | Native boolean |
| `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` | Auto-increment |
| `REAL` (cost) | `NUMERIC(12,6)` | Financial precision |
| `TEXT` (UUID) | `UUID DEFAULT gen_random_uuid()` | Or keep TEXT for zero-touch migration |

---

## 4. Migration Phases (6 phases)

### Phase 1: Adapter Foundation (Week 1)
- [ ] `postgresAdapter.js` — implement full interface with placeholder translation, JSON serialization, connection pooling
- [ ] `schema.postgres.js` — all 10 tables, indexes with proper PostgreSQL types
- [ ] `driver.js` update — add `tryPostgres()` before SQLite fallback chain
- [ ] Unit tests for adapter interface (both SQLite and PostgreSQL)
- [ ] Docker Compose for PostgreSQL testing

**Deliverable**: Both adapters work, can switch via env var

### Phase 2: Easy Repository Migration (Week 1-2)
Migrate 10 repositories that don't need SQL changes (just remove JSON parsing):
- [ ] `settingsRepo.js`, `apiKeysRepo.js`, `connectionsRepo.js`, `proxyPoolsRepo.js`
- [ ] `requestDetailsRepo.js`, `aliasRepo.js`, `combosRepo.js`
- [ ] `disabledModelsRepo.js`, `nodesRepo.js`, `pricingRepo.js`

**Deliverable**: 10/11 repositories work with both SQLite and PostgreSQL

### Phase 3: usageRepo.js Rewrite (Week 2-3) — CRITICAL
- [ ] Convert `_flushWriteQueue` to use multi-row INSERT for `usageHistory`
- [ ] Convert `usageDaily` JSON blob to JSONB with atomic `jsonb_set` or normalized aggregation tables
- [ ] Convert `_meta` counter to atomic `UPDATE ... SET n = n + $1`
- [ ] Make flush truly async (currently synchronous transaction)
- [ ] `getUsageStats` lastUsed overlay → `DISTINCT ON` or window function
- [ ] Consider cross-pod SSE pub/sub (Redis or PostgreSQL LISTEN/NOTIFY)

**Deliverable**: usageRepo.js works with PostgreSQL, non-blocking writes

### Phase 4: K8s Deployment Manifests (Week 3)
- [ ] PostgreSQL StatefulSet or external managed PostgreSQL
- [ ] 9Router Deployment (2-3 replicas)
- [ ] Secrets management (POSTGRES_URL in K8s Secret)
- [ ] Health checks and readiness probes

**Deliverable**: K8s manifests ready for deployment

### Phase 5: Data Migration Tool (Week 4)
- [ ] `scripts/migrate-sqlite-to-postgres.js` — one-time data migration
- [ ] Schema version tracking
- [ ] Rollback script

**Deliverable**: One-click migration from SQLite to PostgreSQL

### Phase 6: Testing & Rollout (Week 4-5)
- [ ] Integration tests with PostgreSQL (via Docker)
- [ ] Load testing (compare SQLite vs PostgreSQL)
- [ ] Dual-write mode (1 week monitoring)
- [ ] Switch to PostgreSQL-only
- [ ] Monitor and optimize

**Deliverable**: Production-ready PostgreSQL support

---

## 5. Testing Strategy

### Unit Tests
```
tests/unit/db/
├── adapter.interface.test.js    # Test all adapters against interface spec
├── postgresAdapter.test.js      # PostgreSQL-specific tests
├── driver.test.js               # Adapter selection logic
└── placeholderTranslate.test.js # ? → $N translation
```

### Integration Tests
```
tests/integration/postgres/
├── setup.js
├── settings.test.js
├── apiKeys.test.js
├── connections.test.js
├── usage.test.js               # Critical path
└── requestDetails.test.js
```

### Docker Compose for Testing
```yaml
version: '3.8'
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: 9router_test
      POSTGRES_USER: test
      POSTGRES_PASSWORD: test
    ports:
      - "5432:5432"
    tmpfs:
      - /var/lib/postgresql/data
```

### Test Matrix
| Scenario | SQLite | PostgreSQL |
|----------|--------|------------|
| CRUD operations | ✅ | ✅ |
| Transaction rollback | ✅ | ✅ |
| Batch insert (50 items) | ✅ | ✅ |
| JSON storage/retrieval | ✅ | ✅ |
| Concurrent writes | ❌ (known issue) | ✅ |
| Large dataset (100k rows) | ⚠️ slow | ✅ fast |
| Hot-reload (Next.js) | ✅ | ✅ |
| 100 parallel `saveRequestUsage` | ✅ (WAL serialized) | ✅ (row-level locks) |

---

## 6. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| PostgreSQL connection drops | Medium | High | Connection retry logic, pool config |
| Data type mismatch (JSONB) | Low | Medium | Comprehensive type mapping tests |
| usageRepo.js rewrite breaks | Medium | High | Incremental migration, feature flags |
| SQL translation bugs | Medium | Medium | Adapter-level translation, unit tests |
| Cross-pod settings race condition | Medium | Medium | Atomic `UPDATE ... SET data = data || $1::jsonb RETURNING` |
| Docker not available in CI | Low | Low | Skip PostgreSQL tests, SQLite-only CI |
| Performance regression | Low | High | Benchmark SQLite vs PostgreSQL |
| Plaintext API keys exposure | Medium | High | TLS required, encrypted at-rest, restricted DB role |

### Rollback Procedure
1. Unset `POSTGRES_URL` → automatically falls back to SQLite
2. Run `migrate-postgres-to-sqlite.js` if data was written to PostgreSQL
3. Revert deployment to previous version

---

## 7. Implementation Order

### Week 1: Foundation
1. `postgresAdapter.js` — core adapter implementation with `_convert(sql)` placeholder translation
2. `schema.postgres.js` — PostgreSQL DDL for all 10 tables
3. `driver.js` — env-based adapter selection with `tryPostgres()` before SQLite chain
4. Adapter interface unit tests

### Week 2: Repository Migration (Part 1)
5. Settings repo (simplest, no JSON parsing needed)
6. API keys repo (validateApiKey hot path)
7. Connections repo (transaction-heavy, sort in SQL)
8. Proxy pools, request details, alias, combos

### Week 3: Repository Migration (Part 2) + usageRepo
9. Disabled models, nodes, pricing
10. **usageRepo.js major rewrite** (the big one)

### Week 4: K8s + Data Migration
11. K8s manifests
12. Data migration script
13. Integration tests

### Week 5: Testing + Rollout
14. Load testing
15. Dual-write monitoring
16. Production rollout

---

## 8. Dependencies

### New npm packages
```
pg                  # PostgreSQL client for Node.js
```

### Docker (for testing)
```
postgres:16-alpine  # PostgreSQL 16
```

---

## 9. Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `POSTGRES_URL` | No | (empty) | PostgreSQL connection string. If set, PostgreSQL is used. |
| `POSTGRES_POOL_SIZE` | No | 20 | Max connections in pool |
| `POSTGRES_SSL` | No | false | Enable SSL for PostgreSQL |

### Example Configurations

**Kubernetes (PostgreSQL)**:
```env
POSTGRES_URL=postgres://9router:secret@postgres:5432/9router
POSTGRES_POOL_SIZE=10
```

**Local Development (SQLite fallback)**:
```env
# POSTGRES_URL not set → SQLite used automatically
```

**Local Development (PostgreSQL via Docker)**:
```env
POSTGRES_URL=postgres://test:test@localhost:5432/9router_test
```

---

## 10. Success Criteria

1. ✅ Both adapters implement identical interface
2. ✅ Switching between SQLite and PostgreSQL requires only env var change
3. ✅ All 11 repositories work with both databases
4. ✅ usageRepo.js is non-blocking with PostgreSQL
5. ✅ Integration tests pass with PostgreSQL (via Docker)
6. ✅ K8s manifests deploy 9Router with 2+ replicas
7. ✅ Data migration preserves all records
8. ✅ No performance regression vs SQLite
9. ✅ Zero breaking changes to existing SQLite deployments

---

## 11. Key Insights from Analysis

### usageRepo.js Complexity
- 3 DB writes per API call, including one that rewrites an unbounded JSON blob
- `usageHistory` is append-only and unbounded — needs time-series features
- `getUsageStats` has a full-scan overlay for `lastUsed` timestamps
- SSE `statsEmitter` is process-local — cross-pod pub/sub needed for K8s
- Tests enforce strict invariants (no count loss under 100 parallel writes)

### Settings Concurrency Bug
- Current `db.transaction` + JS-side merge is racy across pods
- Two pods each read the same baseline, both merge, both write → one's update is lost
- Fix: `UPDATE settings SET data = data || $1::jsonb WHERE id = 1 RETURNING data`

### API Key Security
- Keys stored in plaintext — require TLS, encrypted at-rest, restricted DB role
- No hashing possible without breaking every existing key (defer to v2)
- `machineId` recorded on create but not enforced at validation time

---

*Strategy complete — ready for domain breakdown*
