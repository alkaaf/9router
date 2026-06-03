# imp4 — Kubernetes + PostgreSQL Migration Analysis

**Project**: 9Router Database Migration to PostgreSQL for Kubernetes Deployment
**Status**: Analysis complete ✅
**Date**: 2026-06-03
**Analyst**: Claude (with team agents)
**Output**: 8 analysis documents, 2892 lines total

---

## Problem Statement

9Router currently uses SQLite (file-based, single-process). For Kubernetes deployment with multiple replicas/pods, we need PostgreSQL:
- Support concurrent writes from multiple pods
- Support connection pooling
- Support proper time-series data (usageHistory)
- Support JSONB for fast JSON querying
- Support horizontal scaling

---

## Analysis Documents

| # | Document | Lines | Content |
|---|----------|-------|---------|
| 1 | `schema-catalog.md` | 490 | All 12 tables, 19 indexes, column types, PostgreSQL mappings |
| 2 | `usage-tracking-deep-dive.md` | 534 | Complete analysis of saveRequestUsage(), daily aggregation, SSE integration |
| 3 | `adapter-system.md` | 237 | 4 SQLite adapters, PRAGMA settings, adapter API surface |
| 4 | `repository-operations.md` | 330 | All SQL queries across 11 repos, SQLite-specific syntax |
| 5 | `read-patterns.md` | 281 | Query patterns, index usage, performance issues |
| 6 | `keys-and-settings.md` | 198 | API key storage (plaintext!), settings caching, auth flow |
| 7 | `write-patterns.md` | 132 | Write hotspots, transaction patterns, concurrent write issues |
| 8 | `postgresql-migration.md` | **690** | **Migration plan: adapter implementation, schema changes, K8s manifests** |

---

## Key Findings

### Database Architecture
- **12 tables**, 19 indexes
- **4 SQLite adapters**: better-sqlite3 (default), node:sqlite (Node 22.5+), bun:sqlite, sql.js (WASM fallback)
- **Singleton pattern** via `global._dbAdapter` for Next.js hot-reload survival
- **PRAGMA settings**: WAL mode, busy_timeout=5000ms, foreign_keys=ON

### Critical Bottleneck: usageRepo.js (914 lines!)
- `saveRequestUsage()` does **3 writes per flush** in one transaction:
  1. `INSERT INTO usageHistory` (batch up to 50)
  2. `INSERT OR REPLACE INTO usageDaily` (JSON blob)
  3. `UPSERT INTO _meta`
- **All writes are synchronous** — blocks event loop
- **JSON blob aggregation** done in JavaScript, not SQL
- Write queue with 1-second debounce and 50-entry batch limit

### API Key Security Concern
- **Plaintext storage**: Full API key string stored in DB (`apiKeys.key TEXT UNIQUE NOT NULL`)
- `validateApiKey()` is hot path (every authenticated request)
- No key hashing — security risk for shared PostgreSQL

### PostgreSQL Migration Effort

| Component | Effort | Risk |
|-----------|--------|------|
| `postgresAdapter.js` | Small (~200 LOC) | Low |
| `schema.postgres.js` | Small (~250 LOC) | Low |
| driver.js update | Trivial (~20 LOC) | Low |
| usageRepo.js rewrite | **LARGE** | **HIGH** |
| K8s manifests | Medium | Low |
| **Total estimate** | **4-6 weeks** | Medium |

---

## PostgreSQL Adapter Plan

### New Files Needed
1. `src/lib/db/adapters/postgresAdapter.js` — implements same interface (run, get, all, exec, transaction, close)
2. `src/lib/db/schema.postgres.js` — PostgreSQL DDL with proper types (JSONB, TIMESTAMPTZ, BOOLEAN, BIGSERIAL)
3. `scripts/migrate-sqlite-to-postgres.js` — one-time data migration

### driver.js Change
```javascript
// If POSTGRES_URL env var is set, use PostgreSQL
let adapter = await tryPostgres();
if (!adapter) adapter = await tryBetterSqlite(); // fallback to SQLite
```

### Schema Changes for PostgreSQL
- `INTEGER` boolean → `BOOLEAN DEFAULT TRUE`
- `TEXT` timestamp → `TIMESTAMPTZ`
- `TEXT` JSON → `JSONB` (with GIN index)
- `INTEGER PRIMARY KEY AUTOINCREMENT` → `BIGSERIAL`
- `REAL` cost → `NUMERIC(12,6)` (financial precision)
- Add foreign key constraints (optional)

### Code Changes Required
1. `?` placeholder → `$1, $2, ...` (handle in adapter or code)
2. `parseJson()`/`stringifyJson()` → native JSONB handling
3. Batch INSERT → multi-row INSERT syntax
4. Sort in JS → `ORDER BY` in SQL (for PostgreSQL optimization)
5. `usageRepo.js`: switch JSON aggregation to SQL GROUP BY (or TimescaleDB)

---

## Kubernetes Deployment

### Resources Needed
1. **PostgreSQL**: Managed service (RDS, Cloud SQL) or StatefulSet
2. **9Router**: Deployment with 2-3 replicas, load balancer
3. **Secrets**: POSTGRES_URL from env/Secret

### Sample Architecture
```
┌─────────────────────────────────────────────────┐
│              Kubernetes Cluster                   │
├─────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ 9Router  │  │ 9Router  │  │ 9Router  │   │
│  │   Pod 1   │  │   Pod 2   │  │   Pod 3   │   │
│  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘   │
│        │                │                │         │
│        └────────────────┼────────────────┘         │
│                         │                          │
│                  ┌──────▼──────┐                  │
│                  │  pg-pool    │                  │
│                  │  (20 conn)  │                  │
│                  └──────┬──────┘                  │
│                         │                          │
│                  ┌──────▼──────┐                  │
│                  │ PostgreSQL   │                  │
│                  │  (RDS/Cloud) │                  │
│                  └─────────────┘                  │
└─────────────────────────────────────────────────┘
```

---

## Rollout Plan

### Phase 1: Adapter + Schema (Week 1-2)
- [x] Create postgresAdapter.js
- [x] Create schema.postgres.js
- [x] Update driver.js
- [ ] Unit tests

### Phase 2: Repository Migration (Week 2-3)
- [ ] Migrate settings, apiKeys, connections repos
- [ ] Test validateApiKey hot path
- [ ] Benchmark against SQLite

### Phase 3: usageRepo.js (Week 3-4)
- [ ] Refactor batch INSERT
- [ ] Convert JSON to JSONB or normalized columns
- [ ] Test aggregation queries
- [ ] Consider TimescaleDB continuous aggregates

### Phase 4: K8s Deployment (Week 5)
- [ ] Create PostgreSQL StatefulSet or use managed service
- [ ] Create 9Router Deployment with replicas=3
- [ ] Configure secrets and health checks

### Phase 5: Migration & Rollout (Week 6)
- [ ] Run migration script (SQLite → PostgreSQL)
- [ ] Dual-write mode (1 week)
- [ ] Switch to PostgreSQL-only
- [ ] Monitor and optimize

---

## Files Reference

| File | Purpose |
|-------|---------|
| `src/lib/db/driver.js` | Adapter selection — add PostgreSQL option |
| `src/lib/db/schema.js` | SQLite schema — KEEP (backward compat) |
| `src/lib/db/schema.postgres.js` | PostgreSQL schema — NEW |
| `src/lib/db/adapters/*.js` | Add postgresAdapter.js — NEW |
| `src/lib/db/repos/usageRepo.js` | Rewrite batch INSERT — MODIFY |
| `src/lib/db/repos/settingsRepo.js` | Add caching — OPTIONAL |
| `scripts/migrate-sqlite-to-postgres.js` | Data migration — NEW |

---

## Related Improvements

- **imp3** (performa blocking sync): Found that usageRepo writes are synchronous
- **imp2** (rate limiting): Uses same database pattern
- **imp1** (per-key usage): Depends on usageHistory table

---

*Analysis complete — ready for implementation planning*
