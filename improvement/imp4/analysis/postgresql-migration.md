# PostgreSQL Migration Analysis — imp4

**Question:** What needs to be done to add a PostgreSQL adapter/driver to 9Router?
**Context:** 9Router currently uses SQLite (file-based, single-process). To run in Kubernetes with multiple pods, we need PostgreSQL (server-based, multi-connection).

---

## Executive Summary

| Aspect | Effort | Risk | Notes |
|--------|--------|------|-------|
| **Adapter Implementation** | Small | Low | ~200 LOC, mirrors existing adapter interface |
| **Schema Migration** | Small | Low | All CREATE TABLE/INDEX statements work in PostgreSQL with minor type changes |
| **JSON → JSONB** | Small | Low | Drop-in replacement for most patterns |
| **Repository Refactor** | **MEDIUM** | **MEDIUM** | Sort in SQL instead of JS, optimize some queries |
| **usageRepo.js Rewrite** | **LARGE** | **HIGH** | Critical path, needs careful redesign |
| **Settings Caching** | Small | Low | Add Redis layer (optional) |
| **Connection Pooling** | Small | Low | Use `pg.Pool` |
| **K8s Deployment** | Medium | Low | StatefulSet for PostgreSQL, Deployment for app |
| **Total Estimate** | 4-6 weeks | Medium | |

---

## Part 1: PostgreSQL Adapter Implementation

### Adapter Interface (already defined)

The codebase has a clean adapter interface that all repos depend on:

```javascript
// Adapter must implement:
{
  driver: "string",           // identifier
  run(sql, params),           // INSERT/UPDATE/DELETE → { changes, lastInsertRowid }
  get(sql, params),           // SELECT single → row | undefined
  all(sql, params),           // SELECT multi → row[]
  exec(sql),                  // DDL/multi-statement → void
  transaction(fn),            // Atomic block → fn() result
  checkpoint(),               // (SQLite only, can be no-op for PG)
  close(),                    // Cleanup
  raw: db,                    // Underlying driver instance
  prepare(sql)                // (optional) cached prepared statement
}
```

### Recommended Library: `postgres` (postgres.js) or `pg` (node-postgres)

**`postgres` (Porsager)** is recommended:
- Faster than `pg`
- Built-in connection pooling
- Native async/await
- Better prepared statement handling
- Modern API

### Sketch: `src/lib/db/adapters/postgresAdapter.js`

```javascript
import postgres from "postgres";
import { PRAGMA_SQL } from "../schema.js";

export async function createPostgresAdapter(config) {
  // config: { connectionString, pool, ssl, ... }
  const sql = postgres(config.connectionString, {
    max: config.pool?.max || 20,  // pool size
    idle_timeout: 30,
    connect_timeout: 10,
    ssl: config.ssl || false,
  });

  await sql.unsafe(PRAGMA_SQL_POSTGRES); // Apply PG-specific settings

  return {
    driver: "postgres",
    raw: sql,

    // Convert ? placeholders to $1, $2, ... (PostgreSQL syntax)
    _convert(sql) {
      let i = 0;
      return sql.replace(/\?/g, () => `$${++i}`);
    },

    async run(query, params = []) {
      const pg = this._convert(query);
      const result = await sql.unsafe(pg, params);
      return { changes: result.count, lastInsertRowid: result[0]?.id || null };
    },

    async get(query, params = []) {
      const pg = this._convert(query);
      const rows = await sql.unsafe(pg, params);
      return rows[0];
    },

    async all(query, params = []) {
      const pg = this._convert(query);
      return await sql.unsafe(pg, params);
    },

    async exec(query) {
      // Multi-statement DDL
      await sql.unsafe(query);
    },

    async transaction(fn) {
      return await sql.begin(async (tx) => {
        // Wrap adapter methods to use tx
        const txAdapter = {
          ...this,
          run: (q, p) => this.run(q, p).catch(rollback),
          // ... etc
        };
        return await fn(txAdapter);
      });
    },

    async checkpoint() { /* no-op for PostgreSQL */ },
    async close() { await sql.end(); },
  };
}
```

**Estimated size:** ~200 lines

---

## Part 2: Driver.js Updates

### Add PostgreSQL to Adapter Selection

```javascript
// src/lib/db/driver.js

async function tryPostgres() {
  if (!process.env.POSTGRES_URL) return null;
  try {
    const { createPostgresAdapter } = await import("./adapters/postgresAdapter.js");
    return await createPostgresAdapter({
      connectionString: process.env.POSTGRES_URL,
      pool: { max: parseInt(process.env.POSTGRES_POOL_MAX || "20", 10) },
      ssl: process.env.POSTGRES_SSL === "true" ? "require" : false,
    });
  } catch (e) {
    console.warn(`[DB] postgres unavailable: ${e.message}`);
    return null;
  }
}

// In initAdapter():
let adapter = await tryPostgres();      // NEW: PostgreSQL first if configured
if (!adapter) adapter = await tryBunSqlite();
if (!adapter) adapter = await tryBetterSqlite();
if (!adapter) adapter = await tryNodeSqlite();
if (!adapter) adapter = await trySqlJs();
```

**Behavior:** If `POSTGRES_URL` env var is set, use PostgreSQL. Otherwise, fall back to SQLite (preserves single-user mode).

---

## Part 3: Schema Migration (PostgreSQL DDL)

### Issues to Address

| Issue | SQLite | PostgreSQL Fix |
|-------|--------|---------------|
| Boolean as INTEGER | `INTEGER DEFAULT 1` | `BOOLEAN DEFAULT TRUE` |
| Timestamp as TEXT | `TEXT NOT NULL` | `TIMESTAMPTZ NOT NULL` |
| JSON as TEXT | `TEXT` | `JSONB` |
| AUTOINCREMENT | `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL` or `UUID` |
| REAL for cost | `REAL` (floating point) | `NUMERIC(12,6)` (exact) |
| No FK constraints | Implied references | Add `FOREIGN KEY` |

### Recommended Approach: Add new file `src/lib/db/schema.postgres.js`

**DO NOT** modify `schema.js` (keep SQLite working). Add parallel schema file for PostgreSQL.

```javascript
// src/lib/db/schema.postgres.js
export const SCHEMA_VERSION = 1;

export const PRAGMA_SQL_POSTGRES = `
SET synchronous_commit = 'on';
SET shared_buffers = '256MB';
SET effective_cache_size = '1GB';
SET work_mem = '16MB';
SET timezone = 'UTC';
`;

export const TABLES_POSTGRES = {
  _meta: {
    columns: {
      key: "TEXT PRIMARY KEY",
      value: "TEXT NOT NULL",
    },
  },
  settings: {
    columns: {
      id: "INTEGER PRIMARY KEY CHECK (id = 1)",
      data: "JSONB NOT NULL",
    },
  },
  providerConnections: {
    columns: {
      id: "UUID PRIMARY KEY DEFAULT gen_random_uuid()",
      provider: "TEXT NOT NULL",
      authType: "TEXT NOT NULL",
      name: "TEXT",
      email: "TEXT",
      priority: "INTEGER",
      isActive: "BOOLEAN DEFAULT TRUE",
      data: "JSONB NOT NULL",
      createdAt: "TIMESTAMPTZ NOT NULL DEFAULT NOW()",
      updatedAt: "TIMESTAMPTZ NOT NULL DEFAULT NOW()",
    },
    indexes: [
      "CREATE INDEX idx_pc_provider ON providerConnections(provider)",
      "CREATE INDEX idx_pc_provider_active ON providerConnections(provider, isActive) WHERE isActive = TRUE",
      "CREATE INDEX idx_pc_priority ON providerConnections(provider, priority)",
    ],
  },
  // ... (similar for all other tables)

  usageHistory: {
    columns: {
      id: "BIGSERIAL PRIMARY KEY",
      timestamp: "TIMESTAMPTZ NOT NULL",
      provider: "TEXT",
      model: "TEXT",
      connectionId: "TEXT REFERENCES providerConnections(id) ON DELETE SET NULL", // ADDED FK
      apiKey: "TEXT",
      endpoint: "TEXT",
      promptTokens: "BIGINT DEFAULT 0",
      completionTokens: "BIGINT DEFAULT 0",
      cost: "NUMERIC(12,6) DEFAULT 0",
      status: "TEXT",
      tokens: "JSONB",
      meta: "JSONB",
    },
    indexes: [
      "CREATE INDEX idx_uh_ts ON usageHistory(timestamp DESC)",
      "CREATE INDEX idx_uh_provider ON usageHistory(provider)",
      "CREATE INDEX idx_uh_model ON usageHistory(model)",
      "CREATE INDEX idx_uh_conn ON usageHistory(connectionId)",
      "CREATE INDEX idx_uh_apiKey ON usageHistory(apiKey)",
      "CREATE INDEX idx_uh_apiKey_ts ON usageHistory(apiKey, timestamp DESC)",
    ],
  },
  usageDaily: {
    columns: {
      dateKey: "DATE PRIMARY KEY",
      data: "JSONB NOT NULL",
    },
  },
  // ...
};
```

**Estimated size:** ~250 lines (parallel to schema.js)

---

## Part 4: Repository Code Changes

### A. `?` → `$1, $2, ...` Placeholder Conversion

PostgreSQL uses `$1`, `$2` instead of `?`. Two options:

**Option 1: Convert in Adapter** (simpler)
```javascript
_convert(sql) {
  let i = 0;
  return sql.replace(/\?/g, () => `$${++i}`);
}
```

**Option 2: Convert in Code** (explicit, but more work)

**Recommendation:** Option 1 — less code change.

### B. `parseJson()` / `stringifyJson()` Behavior

PostgreSQL JSONB automatically returns/accepts objects. Two options:

**Option 1: Keep TEXT + parse in JS** (no code change)
- Add `data::text` in SELECT, cast in INSERT
- Adapter handles JSON serialization

**Option 2: Switch to JSONB** (faster queries)
- Update `parseJson`/`stringifyJson` to handle JSONB strings
- `pg` driver returns JSONB as already-parsed object

**Recommendation:** Option 2 for new code, Option 1 for migration (no rush).

### C. `db.transaction()` Returns

SQLite `db.transaction(fn)()` returns `fn()` result synchronously.
PostgreSQL `sql.begin(fn)` is async, returns `fn()` result as Promise.

**Impact:** All callers already use `await` (since repos are async), so no code change needed.

### D. `getUsageStats()` — Major Refactor Needed

**Current (SQLite):** 5+ queries, parse JSON, aggregate in JS
```javascript
const dayRows = db.all(`SELECT dateKey, data FROM usageDaily WHERE dateKey >= ?`, [cutoff]);
for (const dr of dayRows) {
  const day = parseJson(dr.data, {});  // JS JSON parse
  stats.totalRequests += day.requests || 0;  // JS aggregation
}
```

**Refactored (PostgreSQL with JSONB):**
```javascript
// Single query with SQL aggregation
const result = await db.get(`
  SELECT
    SUM((data->>'requests')::int) as totalRequests,
    SUM((data->>'promptTokens')::bigint) as totalPromptTokens,
    SUM((data->>'completionTokens')::bigint) as totalCompletionTokens,
    SUM((data->>'cost')::numeric) as totalCost
  FROM usageDaily
  WHERE dateKey >= $1
`, [cutoff]);
```

**Better (with TimescaleDB):** Use continuous aggregates
```sql
-- Pre-computed by TimescaleDB:
SELECT * FROM usage_daily_agg WHERE day >= $1
```

### E. `connectionsRepo.js` — Sort in SQL

**Current:** Sort in JS
```javascript
const rows = db.all(`SELECT * FROM providerConnections WHERE provider = ?`, [provider]);
list.sort((a, b) => (a.priority || 999) - (b.priority || 999));
```

**Refactor:** Sort in SQL
```javascript
const rows = db.all(`SELECT * FROM providerConnections WHERE provider = ? ORDER BY priority ASC NULLS LAST, updatedAt DESC`, [provider]);
```

---

## Part 5: usageRepo.js — The Hardest Part

### Current Bottleneck

```javascript
// Lines 292-308 in usageRepo.js — in db.transaction()
db.transaction(() => {
  // 1. Batch INSERT into usageHistory (up to 50)
  const insertStmt = db.prepare(`INSERT INTO usageHistory(...) VALUES(?, ?, ...)`);
  for (const e of entries) insertStmt.run(...);

  // 2. UPSERT into usageDaily (1 per day in batch)
  const dayStmt = db.prepare(`INSERT INTO usageDaily...ON CONFLICT...`);
  for (const [dk, day] of Object.entries(dayAgg)) dayStmt.run(dk, stringifyJson(day));

  // 3. UPSERT into _meta
  db.run(`INSERT INTO _meta...ON CONFLICT...`);
});
```

### Migration Option A: Minimal Change (PostgreSQL multi-row INSERT)

```javascript
await db.transaction(async (tx) => {
  // 1. Multi-row INSERT into usageHistory
  const placeholders = entries.map((_, i) =>
    `($${i*12+1}, $${i*12+2}, ..., $${i*12+12})`
  ).join(',');
  await tx.run(`
    INSERT INTO usageHistory(timestamp, provider, model, ...) VALUES ${placeholders}
  `, entries.flatMap(e => [e.timestamp, e.provider, e.model, ...]));

  // 2. UPSERT into usageDaily (unchanged)
  for (const [dk, day] of Object.entries(dayAgg)) {
    await tx.run(`INSERT INTO usageDaily...ON CONFLICT...`, [dk, JSON.stringify(day)]);
  }

  // 3. UPSERT into _meta
  await tx.run(`INSERT INTO _meta...`, [...]);
});
```

**Pros:** Minimal code change, works in PostgreSQL
**Cons:** Still doesn't use JSONB advantages, still in-JS aggregation

### Migration Option B: Use JSONB + SQL Aggregation (Recommended)

```javascript
// usageDaily — split JSON into proper columns for fast aggregation
async function updateUsageDaily(tx, dayAgg) {
  for (const [dk, day] of Object.entries(dayAgg)) {
    await tx.run(`
      INSERT INTO usageDaily (
        dateKey, totalRequests, totalPromptTokens, totalCompletionTokens, totalCost,
        byProvider, byModel, byApiKey, byAccount, byEndpoint, lastUpdated
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
      ON CONFLICT (dateKey) DO UPDATE SET
        totalRequests = usageDaily.totalRequests + EXCLUDED.totalRequests,
        totalPromptTokens = usageDaily.totalPromptTokens + EXCLUDED.totalPromptTokens,
        -- ... etc
        byProvider = usageDaily.byProvider || EXCLUDED.byProvider,
        -- ... etc
        lastUpdated = NOW()
    `, [dk, day.requests, day.promptTokens, day.completionTokens, day.cost,
        JSON.stringify(day.byProvider), JSON.stringify(day.byModel), ...]);
  }
}
```

**Pros:** Normalized columns enable SQL aggregation, faster queries
**Cons:** Larger schema change

### Migration Option C: TimescaleDB (Best for Production)

```sql
-- Convert usageHistory to a hypertable:
SELECT create_hypertable('usageHistory', 'timestamp');

-- Create continuous aggregate for daily stats:
CREATE MATERIALIZED VIEW usage_daily_agg
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 day', timestamp) AS day,
  apiKey,
  model,
  provider,
  SUM(prompt_tokens) as prompt_tokens,
  SUM(completion_tokens) as completion_tokens,
  SUM(cost) as cost,
  COUNT(*) as requests
FROM usageHistory
GROUP BY day, apiKey, model, provider;

-- Add refresh policy:
SELECT add_continuous_aggregate_policy('usage_daily_agg',
  start_offset => INTERVAL '1 month',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');
```

**Pros:** Automatic pre-aggregation, automatic partitioning, automatic compression
**Cons:** Requires TimescaleDB extension, major rewrite

**Recommendation:** Start with Option A, evolve to B, consider C for scale.

---

## Part 6: Data Migration Path

### One-Time Data Migration (SQLite → PostgreSQL)

```javascript
// scripts/migrate-sqlite-to-postgres.js
import sqliteAdapter from "./adapters/sqliteAdapter.js";
import postgresAdapter from "./adapters/postgresAdapter.js";

async function migrateData() {
  const sqlite = await sqliteAdapter();
  const postgres = await postgresAdapter();

  // 1. Read all rows from SQLite
  const tables = ["settings", "providerConnections", "providerNodes",
    "proxyPools", "apiKeys", "combos", "usageHistory", "usageDaily", "requestDetails"];

  for (const table of tables) {
    const rows = sqlite.all(`SELECT * FROM ${table}`);
    console.log(`Migrating ${rows.length} rows from ${table}...`);

    if (rows.length === 0) continue;

    // 2. Transform SQLite → PostgreSQL format
    const pgRows = rows.map(transformRow);  // Convert ISO TEXT → TIMESTAMPTZ, etc.

    // 3. Bulk INSERT into PostgreSQL
    await postgres.exec(`TRUNCATE ${table}`);
    for (const row of pgRows) {
      await postgres.run(`INSERT INTO ${table}...`, row);
    }
  }

  console.log("Migration complete!");
}
```

**Estimated time:** 30-60 min for a typical database

---

## Part 7: Kubernetes Deployment

### Required Resources

1. **PostgreSQL StatefulSet** (or external managed PostgreSQL)
   - Use managed service (RDS, Cloud SQL, Aiven) for production
   - OR self-hosted with PVC for dev

2. **9Router Deployment** (multiple replicas)
   - Environment: `POSTGRES_URL=postgresql://...`
   - Replica count: 2-3 (with load balancer)
   - Health check endpoint: `/api/health`

3. **Shared storage for SQLite fallback** (optional)
   - If users want both modes (SQLite for single-user, PG for K8s)
   - EmptyDir for ephemeral data

### Sample Kubernetes Manifests

```yaml
# postgres-statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
        - name: postgres
          image: postgres:16
          env:
            - name: POSTGRES_DB
              value: ninerouter
            - name: POSTGRES_USER
              valueFrom:
                secretKeyRef: { name: pg-secret, key: username }
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef: { name: pg-secret, key: password }
          ports:
            - containerPort: 5432
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 50Gi
```

```yaml
# 9router-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: 9router
spec:
  replicas: 3
  selector:
    matchLabels:
      app: 9router
  template:
    metadata:
      labels:
        app: 9router
    spec:
      containers:
        - name: 9router
          image: 9router:latest
          env:
            - name: POSTGRES_URL
              valueFrom:
                secretKeyRef: { name: pg-secret, key: url }
            - name: POSTGRES_POOL_MAX
              value: "20"
          ports:
            - containerPort: 20128
          livenessProbe:
            httpGet: { path: /api/health, port: 20128 }
            initialDelaySeconds: 30
            periodSeconds: 10
```

---

## Part 8: Testing Strategy

### Unit Tests
- Mock adapter interface
- Test each repo with both SQLite and PostgreSQL adapters
- Ensure consistent behavior

### Integration Tests
- Docker Compose with PostgreSQL container
- Test full request flow: API call → usage write → stats read

### Performance Tests
- Benchmark with 100, 1000, 10000 concurrent requests
- Compare SQLite (single connection) vs PostgreSQL (pool=20)
- Validate no event loop blocking with async adapter

### Migration Tests
- Export from SQLite, import to PostgreSQL
- Verify row counts, sums match
- Test roundtrip: PG → SQLite backup

---

## Part 9: Rollout Plan

### Phase 1: Adapter Implementation (Week 1)
- [ ] Create `postgresAdapter.js` with same interface
- [ ] Update `driver.js` to check POSTGRES_URL
- [ ] Add `schema.postgres.js` for DDL
- [ ] Unit tests for adapter

### Phase 2: Schema Migration (Week 1-2)
- [ ] Create PostgreSQL schema with proper types
- [ ] Add JSONB for JSON columns
- [ ] Add foreign keys
- [ ] Add migration script: SQLite → PostgreSQL

### Phase 3: Repository Refactor (Week 2-3)
- [ ] Update all repos to use async/await pattern (already done)
- [ ] Convert `?` to `$N` (or handle in adapter)
- [ ] Move sorting to SQL
- [ ] Test each repo against PostgreSQL

### Phase 4: usageRepo.js Rewrite (Week 3-4)
- [ ] Switch batch INSERT to multi-row INSERT
- [ ] Convert usageDaily JSON to JSONB
- [ ] Test aggregation queries
- [ ] Benchmark against SQLite baseline

### Phase 5: Settings + API Keys (Week 4)
- [ ] Add Redis cache for settings (optional)
- [ ] Consider key hashing (optional security)
- [ ] Test validateApiKey hot path

### Phase 6: K8s Deployment (Week 5)
- [ ] Create StatefulSet for PostgreSQL
- [ ] Create Deployment for 9Router
- [ ] Configure secrets for POSTGRES_URL
- [ ] Health checks + graceful shutdown
- [ ] Test failover scenarios

### Phase 7: Production Rollout (Week 6)
- [ ] Dual-write mode: SQLite + PostgreSQL (1 week)
- [ ] Switch read to PostgreSQL
- [ ] Switch write to PostgreSQL
- [ ] Remove SQLite fallback (optional)
- [ ] Monitor performance, fix issues

---

## Part 10: Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|-----------|
| Data loss during migration | High | Dual-write period, backup, verification |
| Performance regression | Medium | Benchmark before/after, optimize queries |
| Concurrent write conflicts | Medium | PostgreSQL handles natively, but test |
| Connection pool exhaustion | Medium | Set max=20 per pod, monitor |
| Long-running migrations | Low | Run during low-traffic window |
| Backward compatibility | Low | Keep SQLite adapter, env-based selection |

---

## Summary

To add PostgreSQL support for Kubernetes deployment:

1. **New file:** `src/lib/db/adapters/postgresAdapter.js` (~200 LOC)
2. **New file:** `src/lib/db/schema.postgres.js` (~250 LOC)
3. **Modified file:** `src/lib/db/driver.js` (~20 LOC added)
4. **Modified file:** `src/lib/db/repos/usageRepo.js` (refactor batch INSERT, ~50 LOC changed)
5. **New script:** `scripts/migrate-sqlite-to-postgres.js` (data migration)
6. **K8s manifests:** `postgres-statefulset.yaml`, `9router-deployment.yaml`

**Estimated effort:** 4-6 weeks for full migration with testing.

**Critical complexity:** usageRepo.js due to JSON manipulation patterns.

**Recommended first step:** Implement postgresAdapter.js with parallel schema, test with a few repos (settings, apiKeys), then tackle usageRepo.js last.
