# imp4: PostgreSQL Dual-Database Architecture — Implementation Guide

## Overview

9Router historically used SQLite as its embedded database. SQLite is excellent for single-process, file-based workloads, but it is fundamentally incompatible with the horizontal-scaling story we want for the 9Router app: it serializes writes, offers no connection pooling, cannot serve multi-pod deployments safely, and lacks the time-series and JSONB primitives we need for the `usageHistory` analytics path. This imp4 deliverable adds **PostgreSQL as a first-class, second adapter** alongside SQLite, with a single env-var switch (`POSTGRES_URL`) toggling between them. There is no migration in the application code path: the same repository modules run against both backends, and the driver layer picks the adapter at startup.

**Use SQLite when** you are running a single 9Router instance on a developer laptop, a small VPS, or a personal container. It needs zero infrastructure, the file lives next to your app data, and it is fast enough for sub-1000 req/day workloads. **Use PostgreSQL when** you are running more than one replica, when your `usageHistory` table grows past ~1M rows, when you need cross-pod consistency for the `settings` or `_meta` tables, or when you want native JSONB queries, atomic upserts, and proper connection pooling for the request hot path.

The implementation is intentionally additive: there are zero breaking changes for existing SQLite users. Existing deployments continue to work unmodified; setting `POSTGRES_URL` is the only signal to flip behavior.

## Quick Start (5 minutes)

### Option A: Local Development with Docker

Spin up a local PostgreSQL for testing without docker-compose:

```bash
docker run -d --name 9router-pg \
  -e POSTGRES_DB=9router \
  -e POSTGRES_USER=9router \
  -e POSTGRES_PASSWORD=devpassword \
  -p 5432:5432 \
  postgres:16-alpine

export POSTGRES_URL="postgres://9router:devpassword@localhost:5432/9router"
npm run dev
```

The app will detect `POSTGRES_URL` at startup, create the `postgresAdapter`, apply the schema, and log `[DB] Driver: postgres` once.

### Option B: Docker Compose

The repo ships a `docker-compose.test.yml` for tests and local dev:

```bash
docker compose -f docker-compose.test.yml up -d
export POSTGRES_URL="postgres://test:test@localhost:5432/9router_test"
npm test
```

The compose file uses `tmpfs` for the data directory (so it never persists across container recreation) and `trust` auth, which is appropriate for local dev only.

### Option C: Kubernetes

Apply the manifests in the order below. All manifests live under `k8s/`:

```bash
# 1. Namespace, ConfigMap, Secrets
kubectl apply -f k8s/9router-deployment.yaml
# 2. PostgreSQL StatefulSet + PVC
kubectl apply -f k8s/postgres-statefulset.yaml
# 3. Service topology (headless + ClusterIP + LoadBalancer + Ingress)
kubectl apply -f k8s/services.yaml

# Verify
kubectl -n 9router get pods
kubectl -n 9router logs -l app=postgres --tail=20
```

The 9Router pods are configured to read `POSTGRES_URL` from the `9router-secrets` Secret automatically.

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `POSTGRES_URL` | No (SQLite fallback) | - | PostgreSQL connection string. If set and non-empty, PostgreSQL is used; otherwise SQLite. |
| `POSTGRES_POOL_SIZE` | No | 20 | Max connections in pg pool per pod |
| `POSTGRES_SSL` | No | false | Enable SSL (`sslmode=require`-equivalent). Set `true` for managed databases. |
| `DATABASE_URL` | No | - | Alias for `POSTGRES_URL` (Heroku/Render convention) |
| `DATA_FILE` | No | `.data/9router.db` | SQLite path (used when `POSTGRES_URL` is empty) |
| `DATA_DIR` | No | `~/.9router` | Directory for SQLite file and backups |
| `JWT_SECRET` | Yes (prod) | auto-generated | JWT signing secret — share across pods |
| `NODE_ENV` | No | runtime default | Set `production` for deploy |

### Schema Selection Logic

Adapter selection is performed once at process start, in `src/lib/db/driver.js`:

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

The fallback chain after PostgreSQL reflects the runtime: on Bun, `tryBunSqlite` runs first; on Node, `tryBetterSqlite` runs first. If `POSTGRES_URL` was set and PostgreSQL fails AND no SQLite driver is loadable, the process exits with a hard error (`"PostgreSQL connection failed and no SQLite driver available"`).

## Architecture

### Adapter Interface

Both adapters (`postgresAdapter.js` and the four SQLite adapters under `adapters/`) implement the same 9-method surface:

```js
{
  driver:  "postgres" | "better-sqlite3" | "node:sqlite" | "bun:sqlite" | "sql.js",
  kind:    "postgres" | "sqlite",
  run(sql, params)             // execute, return { changes }
  get(sql, params)             // single row, {} if empty
  all(sql, params)             // all rows, [] if empty
  exec(sql, params)            // DDL/multistmt, return { lastInsertRowid, changes }
  transaction(fn)              // BEGIN; fn(tx); COMMIT/ROLLBACK
  prepare(sql)                 // returns { run, get, all } pinned
  checkpoint()                 // no-op for postgres
  close()                      // tear down pool / file handle
  raw                          // escape hatch to underlying driver
}
```

The contract is intentionally minimal: every repository module imports only these methods. Adding a new database (e.g. MySQL) means writing a new adapter that satisfies the same shape — no repository code changes.

### Type Mapping

| SQLite Type | PostgreSQL Type | Notes |
|-------------|-----------------|-------|
| `TEXT` (general) | `TEXT` | Direct mapping |
| `TEXT` (timestamp) | `TIMESTAMPTZ` | Timezone-aware, ISO string in/out |
| `TEXT` (JSON blob) | `JSONB` | Binary JSON, GIN-indexable |
| `INTEGER` (boolean) | `BOOLEAN DEFAULT TRUE` | Native boolean; no `0/1` coercion in adapter |
| `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` | Auto-increment 64-bit |
| `REAL` (cost/price) | `NUMERIC(12,6)` | Financial precision, no float drift |
| `TEXT` (UUID) | `TEXT` (kept) | We do not switch to native `UUID` to keep migration frictionless |
| `INTEGER` (counters) | `BIGINT DEFAULT 0` | `usageHistory`-style growing counters |

### Schema Differences

The PostgreSQL schema in `src/lib/db/schema.postgres.js` defines **11 base tables and 5 rollup tables**:

| Table | Kind | Notes |
|-------|------|-------|
| `_meta` | base | Versioning / key-value meta |
| `settings` | base | Single-row config (id=1), `data` JSONB |
| `providerConnections` | base | Provider credentials, JSONB blob |
| `providerNodes` | base | LLM node configs, JSONB blob |
| `proxyPools` | base | Proxy pool configs, JSONB blob |
| `apiKeys` | base | API keys (machineId, isActive) |
| `combos` | base | Model combos, `models` JSONB |
| `kv` | base | Generic key-value store, `value` JSONB |
| `usageHistory` | base | Append-only request usage log |
| `usageDaily` | base | Date → JSONB blob (legacy) |
| `requestDetails` | base | Per-request debug log |
| `usageDailyByProvider` | rollup | (date, provider) counters |
| `usageDailyByModel` | rollup | (date, model) counters |
| `usageDailyByApiKey` | rollup | (date, apiKeyId) counters |
| `usageDailyByAccount` | rollup | (date, accountId) counters |
| `usageDailyByEndpoint` | rollup | (date, endpoint) counters |

The **5 rollup tables** are new for PostgreSQL. They are populated via atomic upserts in `usageRepo._flushWriteQueue`, eliminating the read-modify-write race condition in the SQLite `usageDaily` JSON blob pattern. The same query paths read from these rollups instead of parsing the blob.

Key differences vs. the SQLite schema:
- `usageHistory.id` is `BIGSERIAL` (was `INTEGER AUTOINCREMENT`).
- `usageHistory.cost` is `NUMERIC(12,6)` (was `REAL`).
- `usageHistory.tokens` and `meta` are `JSONB` (was `TEXT`).
- `usageDaily.dateKey` is `DATE` (was `TEXT`).
- All `*At` columns are `TIMESTAMPTZ NOT NULL` (was `TEXT`).
- `isActive` flags are `BOOLEAN DEFAULT TRUE` (was `INTEGER 0/1`).

### Placeholder Translation

PostgreSQL uses `$1, $2, $3, ...` positional placeholders; SQLite uses `?`. To keep the call sites uniform, `postgresAdapter.js` performs a transparent translation in `_convert(sql)` (called on every query):

```js
function convertPlaceholders(sql) {
  let i = 0;
  return sql.replace(/\?/g, () => `$${++i}`);
}
```

The translation is cached per SQL string in a `Map` (`state.prepared`) so the hot path skips the regex on repeated calls. **All repository code can therefore keep writing `?` placeholders** — the adapter makes the substitution on the way in.

**Caveat**: the translation does not parse string literals. If you ever inline a literal containing `?` into a query, you must escape it or rewrite to use a parameter.

### JSON Handling

The two backends serialize JS objects very differently:

| Backend | On write | On read |
|---------|----------|---------|
| SQLite | Caller does `JSON.stringify(obj)` before `run()` | Caller does `JSON.parse(row.data)` after `get()`/`all()` |
| PostgreSQL | pg driver auto-serializes JS object → JSONB | pg driver auto-parses JSONB → JS object |

For repositories that want to be portable across both, the rule is: **always pass JS objects, never pre-stringify them**. The repository layer never calls `JSON.stringify` / `JSON.parse` directly — that responsibility was removed from the SQLite path during the imp4 migration. On SQLite, the data is stored as `TEXT` and read back as a string, but the `apiKeysRepo`/`settingsRepo` style code uses the helper `parseJson(row.data)` only where the row was originally written through the legacy path.

The `kv` table specifically uses `JSONB` to get native object storage on PostgreSQL; on SQLite, the same table uses `TEXT` and the caller treats it as opaque.

### Transaction Nesting

`adapter.transaction(fn)` pins every call inside `fn` to a single checked-out pg client. Nested `transaction()` calls inside `fn` are implemented as **SAVEPOINTs** rather than nested BEGINs (PostgreSQL throws "already in transaction" on nested BEGIN):

```js
async transaction(fn) {
  const sp = `sp_${Math.random().toString(36).slice(2)}`;
  await client.query(`SAVEPOINT ${sp}`);
  try {
    const r = await fn(this);
    await client.query(`RELEASE SAVEPOINT ${sp}`);
    return r;
  } catch (e) {
    try { await client.query(`ROLLBACK TO SAVEPOINT ${sp}`); } catch {}
    throw e;
  }
}
```

This matches the behavior of the `node:sqlite` adapter, which also uses `SAVEPOINT` for nested transactions. `better-sqlite3` uses real nested transactions on its C++ side; for cross-adapter parity, the PostgreSQL adapter and the SQL.js adapter both implement the savepoint protocol.

## Deployment

### Docker Compose

```bash
# 1. Start PostgreSQL
docker compose -f docker-compose.test.yml up -d

# 2. Wait for it to be ready (the helper script does this automatically)
./scripts/test-pg.sh

# 3. Run the app pointing at it
export POSTGRES_URL="postgres://test:test@localhost:5432/9router_test"
npm run dev
```

The `docker-compose.test.yml` service:
- Uses `postgres:16-alpine` (~80MB image)
- Trusts all local connections (`POSTGRES_HOST_AUTH_METHOD=trust`) — **dev/test only**
- Configures `max_connections=50`, `shared_buffers=128MB` to support a dev machine
- Mounts `/var/lib/postgresql/data` on `tmpfs` so the container leaves no residue

### Kubernetes

Apply the three manifests in order:

```bash
kubectl apply -f k8s/9router-deployment.yaml
kubectl apply -f k8s/postgres-statefulset.yaml
kubectl apply -f k8s/services.yaml
```

This creates:
- Namespace `9router`
- ConfigMap `9router-config` (NODE_ENV, PORT, HOSTNAME, DATA_DIR)
- Secret `9router-secrets` (POSTGRES_URL, JWT_SECRET) — **replace `changeme` with a real password before production use**
- Secret `postgres-credentials` (POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD)
- StatefulSet `postgres` (1 replica) with 20Gi PVC
- Deployment `9router` (3 replicas, HPA 2-10)
- Service topology: headless `postgres`, ClusterIP `postgres-cluster`, ClusterIP `9router`, LoadBalancer `9router-lb`, Ingress `9router-ingress`

#### Production Setup

For a production deployment, replace the bundled StatefulSet with a managed PostgreSQL (AWS RDS, Google Cloud SQL, Crunchy Bridge, Neon, etc.). The key changes:

1. **StatefulSet for PostgreSQL** with `20Gi` PVC and `standard` storage class. Increase to `100Gi+` for production. Use `rook-ceph` or your cloud's SSD-backed class.
2. **3-replica Deployment for the app** with `RollingUpdate` strategy (`maxSurge: 1`, `maxUnavailable: 0`) — zero-downtime deploys.
3. **HPA 2-10 replicas** scaling on 70% CPU / 80% memory. The `scaleUp` policy permits 100% growth per 30s, so a 3→10 scale-up completes in under 60s.
4. **Service topology**:
   - `postgres` headless Service (ClusterIP: None) for StatefulSet DNS
   - `postgres-cluster` ClusterIP for app-to-db access
   - `9router` ClusterIP for in-cluster HTTP traffic on port 80 → container 20128
   - `9router-lb` LoadBalancer for external access (ports 80, 443)
   - `9router-ingress` (nginx) for host-based routing — edit `host: 9router.example.com` to match your domain
5. **Probes**: `readinessProbe` and `livenessProbe` both hit `/api/health` over HTTP. Initial delay 10s for readiness, 30s for liveness.
6. **Resources**: app requests 250m CPU / 256Mi memory, limits 1 CPU / 1Gi. Postgres requests 250m / 512Mi, limits 1 CPU / 2Gi.

## Data Migration (SQLite → PostgreSQL)

### When to Migrate

You should move from SQLite to PostgreSQL when **any** of these become true:

- You deploy more than one 9Router pod and see write conflicts in the logs
- `usageHistory` exceeds ~1M rows and chart/dashboard queries get slow
- You want cross-pod consistency on `settings` and `_meta` (race-free atomic updates)
- You need to add read replicas for analytics dashboards
- You need to back up with `pg_dump` rather than copying a `.db` file
- You need a managed database with automated backups and point-in-time recovery

### Migration Steps

The migration script is `scripts/migrate-sqlite-to-postgres.js` (Wave 4). It runs as a one-shot CLI:

```bash
# 1. Backup the SQLite file (do not skip)
cp -r .data/9router.db .data/9router.db.bak.$(date +%Y%m%d)

# 2. Ensure PostgreSQL is running and the schema is applied
export POSTGRES_URL="postgres://9router:secret@postgres:5432/9router"
node -e 'import("./src/lib/db/driver.js").then(m => m.getAdapter())'

# 3. Run the migration
node scripts/migrate-sqlite-to-postgres.js \
  --sqlite .data/9router.db \
  --postgres "$POSTGRES_URL" \
  --batch 500

# 4. Verify row counts match
node scripts/verify-migration.js --sqlite .data/9router.db --postgres "$POSTGRES_URL"

# 5. Switch the env var in your deploy
#    Remove DATA_FILE for production pods; only POSTGRES_URL matters.
kubectl -n 9router set env deployment/9router POSTGRES_URL=...

# 6. Smoke-test the running app
curl -fsS http://9router.example.com/api/health
```

The script:
- Connects to both DBs using their respective adapters
- Iterates tables in dependency order (`_meta` → `settings` → `providerConnections` → ... → `usageHistory`)
- Uses multi-row `INSERT ... ON CONFLICT DO UPDATE` for upsert semantics
- Migrates the 5 rollup tables by re-aggregating from `usageHistory` (more accurate than copying the stale `usageDaily` JSON blob)
- Reports per-table row counts and total time

### Rollback Plan

If the migration fails or the new deployment misbehaves:

```bash
# 1. Revert the env var (unset POSTGRES_URL)
kubectl -n 9router set env deployment/9router POSTGRES_URL-

# 2. The app falls back to SQLite automatically — no restart of the
#    deployment needed, but rolling restart is safer:
kubectl -n 9router rollout restart deployment/9router

# 3. If you need to also revert the data, run the reverse script:
node scripts/migrate-postgres-to-sqlite.js \
  --postgres "$POSTGRES_URL" \
  --sqlite .data/9router.db

# 4. Restore from the .bak file as a last resort:
cp .data/9router.db.bak.$(date +%Y%m%d) .data/9router.db
```

Because the adapter selection is purely env-driven, **rollback is "unset the env var and restart"** — no code changes, no schema changes.

## Testing

### Local Test

```bash
# Start PostgreSQL and run the test suite
./scripts/test-pg.sh
POSTGRES_URL="postgres://test:test@localhost:5432/9router_test" npm test
```

### Integration Test

The integration suite lives in `tests/integration/postgres/` and covers:

- Adapter interface contract (run/get/all/exec/transaction)
- Placeholder translation (`?` → `$1, $2, ...`)
- JSONB round-trip (object in, object out, no manual `JSON.stringify`)
- Transaction commit and rollback
- Savepoint nesting
- Pool exhaustion behavior

```bash
# Run only the integration tests
POSTGRES_URL="postgres://test:test@localhost:5432/9router_test" \
  npx vitest run tests/integration/postgres
```

### Performance Test

For load testing the PostgreSQL path:

```bash
# k6 — 100 concurrent users, 60s sustained
k6 run --vus 100 --duration 60s \
  -e POSTGRES_URL="$POSTGRES_URL" \
  tests/load/usage-hot-path.js

# autocannon — raw HTTP against the chat completions endpoint
autocannon -c 50 -d 60 \
  -H "Authorization: Bearer $TEST_API_KEY" \
  -H "Content-Type: application/json" \
  -m POST -b '{"model":"kr/claude-sonnet-4.5","messages":[{"role":"user","content":"ping"}]}' \
  http://localhost:20128/v1/chat/completions
```

Compare against the same workload against SQLite to verify the rollout does not regress.

## Monitoring

### Key Metrics

Track these in your observability stack (Prometheus, Datadog, etc.):

| Metric | Source | Healthy Range |
|--------|--------|---------------|
| `pg_pool_total_count` | `pool.totalCount` | < `max` (20) sustained |
| `pg_pool_idle_count` | `pool.idleCount` | > 0 (room to grow) |
| `pg_pool_waiting_count` | `pool.waitingCount` | 0 (no contention) |
| Query latency p95 | middleware | < 50ms for repos, < 200ms for `getUsageStats` |
| `usageHistory` write rate | app metric | matches request rate |
| `usageDailyBy*` upsert rate | app metric | ~3x write rate (3 rollups per write) |
| Pool error events | `pool.on("error")` | 0 |

### Health Checks

- `GET /api/health` — liveness, returns 200 if the process is responsive
- `GET /api/health/db` — readiness, runs `SELECT 1` against the active adapter
- `GET /readyz` — alias for `/api/health/db`

The Kubernetes manifests wire `readinessProbe` and `livenessProbe` to `/api/health` over HTTP on port 20128. The PostgreSQL StatefulSet uses `pg_isready` exec probes for both liveness and readiness.

### Logs

What to look for in `kubectl logs -l app=9router`:

- `[DB] Driver: postgres` — confirms the adapter selected at startup (logged once)
- `[DB] pg pool error:` — pool-level failures (idle client dropped, network blip)
- `[DB] PostgreSQL init failed, falling back to SQLite` — the URL was set but connection failed; check DNS, network policy, and credentials
- Slow query warnings — set `POSTGRES_LOG_SLOW_QUERIES_MS=100` to log queries over 100ms

## Troubleshooting

### "PostgreSQL connection failed and no SQLite driver available"

**Cause**: `POSTGRES_URL` is set, the initial connection failed, and no SQLite driver could be loaded as a fallback.

**Fixes**:
1. Verify the connection string is correct: `psql "$POSTGRES_URL"` from a shell that has `psql` installed
2. Check DNS / network: `nslookup postgres.9router.svc.cluster.local` from inside a pod
3. If you intended to use PostgreSQL, fix the URL and restart
4. If you actually wanted SQLite fallback, unset `POSTGRES_URL` and ensure one of `bun:sqlite`/`better-sqlite3`/`node:sqlite`/`sql.js` is available

### "relation does not exist"

**Cause**: PostgreSQL was reachable but the schema was not applied (e.g., a fresh DB with no DDL yet, or a manual `psql` session that bypassed `driver.js`).

**Fix**:
```bash
# Re-apply the schema manually
node -e 'import("./src/lib/db/schema.postgres.js").then(m => console.log(m.getPostgresSchema()))' \
  | psql "$POSTGRES_URL"
```

If you see this on app startup, the schema is normally applied via `adapter.exec(schema)` inside `tryPostgres()` — that path failed silently and the app started anyway. Check the logs for the `tryPostgres` error message.

### "too many connections"

**Cause**: The pool is exhausted. This typically means either (a) `POSTGRES_POOL_SIZE` is too low for the replica count, or (b) connections are leaking (not being released back to the pool).

**Fixes**:
1. Increase `POSTGRES_POOL_SIZE` (default 20). For 3 app pods × 20 connections = 60; raise Postgres `max_connections` accordingly.
2. Check for uncommitted transactions: `SELECT * FROM pg_stat_activity WHERE state = 'idle in transaction'`
3. Look for slow queries: `SELECT pid, query, now() - query_start AS duration FROM pg_stat_activity WHERE state <> 'idle' ORDER BY duration DESC`
4. Verify the application code path does not call `pool.connect()` without a matching `client.release()`

### Placeholder errors

**Symptom**: `error: syntax error at or near "$"` or `error: insufficient parameters`.

**Cause**: A query was sent to PostgreSQL with `?` placeholders that were not translated, or with mismatched parameter count.

**Fix**: All queries must go through the adapter (`run`/`get`/`all`/`exec`/`transaction`/`prepare`). Direct `pool.query()` calls must use `$1, $2, ...` themselves. Check for any code path that calls `adapter.raw.query(...)` with `?` placeholders.

### JSONB type errors

**Symptom**: `error: invalid input syntax for type json` or `error: cannot cast text to jsonb`.

**Cause**: A string was passed where a JSONB column expected a JS object (or vice versa).

**Fix**: Always pass JS objects to JSONB columns. If you have legacy data that was stored as `JSON.stringify(...)` text, run a one-time migration:

```sql
UPDATE settings
SET data = (data #>> '{}')::jsonb
WHERE jsonb_typeof(data) = 'string';
```

The driver assumes the column is a real JSONB; storing escaped text inside it will round-trip as a string, not an object.

## Files Reference

### Created

- `src/lib/db/adapters/postgresAdapter.js` — pg-based adapter implementing the 9-method interface, with placeholder translation, savepoint nesting, and prepared-statement caching
- `src/lib/db/schema.postgres.js` — DDL for 11 base tables + 5 rollup tables, with 19+ indexes
- `k8s/9router-deployment.yaml` — Namespace, ConfigMap, Secrets, Deployment, HPA, PVCs, Service
- `k8s/postgres-statefulset.yaml` — Namespace, Secret, headless Service, ClusterIP, StatefulSet with 20Gi PVC
- `k8s/services.yaml` — Headless Service, ClusterIP, LoadBalancer, Ingress
- `docker-compose.test.yml` — Local dev/test PostgreSQL with `tmpfs` storage
- `scripts/test-pg.sh` — Helper that brings up the test compose stack and exports `POSTGRES_URL`
- `scripts/migrate-sqlite-to-postgres.js` (Wave 4) — one-shot migration CLI

### Modified

- `src/lib/db/driver.js` — added `tryPostgres()` ahead of the SQLite fallback chain
- `src/lib/db/repos/usageRepo.js` — multi-row INSERT for `usageHistory`, atomic upserts to the 5 rollup tables
- `src/lib/db/repos/requestDetailsRepo.js` — async API surface (all repo methods are now async-safe)
- `src/lib/db/repos/settingsRepo.js` — driver branch that uses `jsonb_set` for atomic merge on PostgreSQL
- `src/lib/db/repos/apiKeysRepo.js` — `toDbBool` helper for cross-adapter boolean handling
- `package.json` — added `pg@8.21.0` (node-postgres) dependency

## Performance Considerations

### Indexes

The PostgreSQL schema defines **19 indexes** on the hot read paths:

- `usageHistory` — 10 indexes including composite `(provider, timestamp DESC)`, `(model, timestamp DESC)`, `(apiKey, timestamp DESC)`. All timestamp indexes are `DESC` for `ORDER BY timestamp DESC LIMIT N` queries.
- `providerConnections` — 3 indexes: `(provider)`, `(provider, isActive)`, `(provider, priority)`
- `proxyPools` — 2 indexes: `(isActive)`, `(testStatus)`
- `requestDetails` — 4 indexes: `(timestamp DESC)`, `(provider)`, `(model)`, `(connectionId)`
- Single indexes on `combos(name)`, `kv(scope)`, `providerNodes(type)`, `apiKeys(key)`

The `DESC` direction matches the query order, which avoids a sort step in PostgreSQL's planner.

### Rollup Tables

The 5 rollup tables (`usageDailyByProvider`, `usageDailyByModel`, `usageDailyByApiKey`, `usageDailyByAccount`, `usageDailyByEndpoint`) replace the SQLite-era "JSON blob per day" pattern. The trade-offs:

- **Before**: `usageDaily` was a single row per date with a `data` JSON blob. `getUsageStats` had to fetch the full blob and aggregate in JS. Concurrent writers raced to rewrite the blob (last-write-wins).
- **After**: 5 normalized tables. `getUsageStats` does `SELECT provider, SUM(requestCount), SUM(cost) FROM usageDailyByProvider WHERE date BETWEEN $1 AND $2 GROUP BY provider`. Inserts are atomic `INSERT ... ON CONFLICT (date, provider) DO UPDATE SET requestCount = requestCount + $3, cost = cost + $4`. No read-modify-write, no race.

The rollups are populated from `usageHistory` at the same time as the history insert. The cost is 3 additional upserts per request (history + 3 rollups hit per typical request — provider, model, apiKey). The benefit is O(1) lookups for dashboard queries.

### Connection Pooling

Pool configuration (in `postgresAdapter.js`):

```js
new Pool({
  connectionString,
  max: parseInt(process.env.POSTGRES_POOL_SIZE || "20", 10),
  idleTimeoutMillis: 30000,
  connectionTimeoutMillis: 10000,
  ssl: process.env.POSTGRES_SSL === "true" ? { rejectUnauthorized: false } : false,
})
```

- `max: 20` — supports 3 app pods × 20 = 60 connections; raise Postgres `max_connections` accordingly
- `idleTimeoutMillis: 30000` — clients idle for 30s are recycled
- `connectionTimeoutMillis: 10000` — fail fast (10s) on network issues, do not hang requests
- The pool itself is a `globalThis` singleton so Next.js dev hot-reload does not leak connections

### Query Patterns

| Path | Type | Notes |
|------|------|-------|
| API request → `usageHistory` insert | Append-heavy | One row per request, BIGSERIAL id |
| `usageHistory` → rollup upserts | Append-heavy | 3 `INSERT ... ON CONFLICT` per request |
| `getUsageStats` | Read-heavy | Aggregates from rollup tables, no full scan |
| `getChartData` | Read-heavy | Time-range query on rollup tables |
| `settings.get/set` | Mixed | One row, JSONB merge on set |
| `apiKeys.validateApiKey` | Read-heavy | Single row lookup by `key` column (indexed) |
| `providerConnections.list` | Read-heavy | Index on `(provider, isActive)` |

The hot path per request is: read `apiKeys` (1 query) → resolve `providerConnections` (1-3 queries) → proxy the request → write `usageHistory` + 3 rollups (4 queries) → return. Total: ~7-8 queries per request, all under 10ms p95 on a tuned Postgres.

## Security

### Connection Security

- All connections should use TLS in production. Set `POSTGRES_SSL=true` for managed databases; the adapter passes `{ rejectUnauthorized: false }` to `pg` (acceptable for self-signed internal certs, replace with the CA bundle for stricter validation).
- For local dev, plaintext on `localhost` is acceptable.
- Database should never be exposed on a public IP. In K8s, the Service is `ClusterIP` only; in cloud, restrict the security group to the app subnet.

### SQL Injection

All repository code uses parameterized queries via `?` placeholders, which the adapter translates to `$1, $2, ...`. There is no string concatenation into SQL anywhere in the adapter or repository layer. The `_convert(sql)` function in `postgresAdapter.js` only rewrites `?` to `$N` — it does not interpret or escape user input.

**Note**: string literals containing `?` characters are not skipped by `_convert`. Always use parameter binding for any user-controlled value, including table/column names that could theoretically contain `?` (none exist in the current schema, but be careful when adding dynamic SQL).

### Credentials

- The PostgreSQL password is stored in `k8s/postgres-statefulset.yaml` as a `Secret` (`postgres-credentials`). Replace `changeme` before applying to a real cluster.
- The 9Router app reads `POSTGRES_URL` from `9router-secrets`.
- Use sealed-secrets, external-secrets, or your cloud's secret manager (AWS Secrets Manager, GCP Secret Manager) in production. Do not commit secrets to git.
- The `JWT_SECRET` is also a Secret; share it across all pods or JWT validation will fail intermittently.

## Future Work

- **Read replicas**: Wire the read pool to a replica for analytics queries. The current adapter is single-pool; a future enhancement could add `getReadAdapter()` for `getUsageStats` paths.
- **Partitioning `usageHistory` by month**: Once the table exceeds ~10M rows, switch to `PARTITION BY RANGE (timestamp)`. The schema is already keyed on `timestamp`, so this is a clean migration.
- **Materialized views for analytics**: Pre-compute monthly/weekly rollups into materialized views refreshed every 5 minutes. Useful for the "spending trends" dashboard.
- **Connection pool tuning per-instance**: Currently `POSTGRES_POOL_SIZE=20` is global. A future enhancement could read the HPA replica count and divide Postgres `max_connections` by replica count, raising a soft alert if the math no longer works.
- **Logical replication for zero-downtime migration**: When we eventually move from RDS-A to RDS-B, logical replication slots + a switchover window would let us migrate with no app restart.
- **Connection-level metrics in OpenTelemetry**: Expose `pool.totalCount` / `pool.idleCount` / `pool.waitingCount` as OTel gauges for Grafana dashboards.
- **Prepared statement namespacing**: pg supports named prepared statements; the current `state.prepared` Map caches the converted SQL but re-prepares on the wire each time. A future enhancement could use `cursor()` with a unique name per SQL string.
