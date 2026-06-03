# Repository Operations — imp4 Analysis

> Generated: 2026-06-03 · Source: `src/lib/db/repos/`, `src/lib/db/helpers/`, `src/lib/usageDb.js`
> Purpose: Reference for imp4 (K8s/PostgreSQL migration) — identify all SQL emitted by the repository layer, classify per query, and surface PostgreSQL migration concerns.

---

## 1. Repository Files Inventory

| # | File | LOC | Tables Touched | CRUD | Notes |
|---|------|----:|----------------|------|-------|
| 1 | `src/lib/db/repos/settingsRepo.js` | 118 | `settings` | R, U (upsert) | Single-row (`id=1`) JSON blob + in-memory cache (5s TTL). |
| 2 | `src/lib/db/repos/connectionsRepo.js` | 226 | `providerConnections` | R, C, U, D | OAuth refresh race safety via txn; `reorderInTx` rewrites priorities. |
| 3 | `src/lib/db/repos/nodesRepo.js` | 95 | `providerNodes` | R, C, U, D | EAV pattern: fixed columns + JSON `data` blob. |
| 4 | `src/lib/db/repos/proxyPoolsRepo.js` | 103 | `proxyPools` | R, C, U, D | Same EAV pattern; status filter. |
| 5 | `src/lib/db/repos/apiKeysRepo.js` | 75 | `apiKeys` | R, C, U, D | Hot path: `validateApiKey` (auth gate). |
| 6 | `src/lib/db/repos/combosRepo.js` | 73 | `combos` | R, C, U, D | Models array stored as JSON string. |
| 7 | `src/lib/db/repos/aliasRepo.js` | 62 | `kv` (scope: `modelAliases`, `customModels`, `mitmAlias`) | R, C, D | Uses generic `kvStore` helper. |
| 8 | `src/lib/db/repos/pricingRepo.js` | 111 | `kv` (scope: `pricing`) | R, U, D | Read-modify-write per provider inside txn. |
| 9 | `src/lib/db/repos/disabledModelsRepo.js` | 56 | `kv` (scope: `disabledModels`) | R, C, U, D | Array semantics inside JSON value. |
| 10 | `src/lib/db/repos/usageRepo.js` | 913 | `usageHistory`, `usageDaily`, `_meta` | R, C, U (lifetime counter) | **Largest/most complex file.** Write queue + batch flush + 24h/today/7d/30d/60d rollups. |
| 11 | `src/lib/db/repos/requestDetailsRepo.js` | 200 | `requestDetails` | R, C, D (trim) | Observability buffer; FIFO trim when `COUNT(*) > maxRecords`. |
| 12 | `src/lib/usageDb.js` | 7 | (shim) | — | Re-exports from `@/lib/db/index.js`. No SQL of its own. |

### Helper modules that emit SQL on behalf of repos

| File | SQL Emitted | Used By |
|------|-------------|---------|
| `src/lib/db/helpers/kvStore.js` | `SELECT`/`INSERT … ON CONFLICT DO UPDATE`/`DELETE` against `kv` | `aliasRepo`, `pricingRepo`, `disabledModelsRepo` |
| `src/lib/db/helpers/metaStore.js` | `SELECT`/`INSERT … ON CONFLICT DO UPDATE` against `_meta` | `usageRepo` (lifetime counter) |

### Schema (current SQLite definition lives in `src/lib/db/schema.js`)

Tables: `_meta`, `settings`, `providerConnections`, `providerNodes`, `proxyPools`, `apiKeys`, `combos`, `kv`, `usageHistory`, `usageDaily`, `requestDetails`.

Most columns are `TEXT` (UUIDs + ISO timestamps + JSON blobs). `INTEGER` used for `id` (autoincrement), `priority`, `isActive`, `promptTokens`, `completionTokens`, `cost`. `data` and `tokens` columns are **JSON-as-TEXT** with helpers `parseJson`/`stringifyJson` in `src/lib/db/helpers/jsonCol.js`.

### Helper pattern across ALL repos

- `parseJson(text, default)` from `helpers/jsonCol.js` — safely parse JSON TEXT columns
- `stringifyJson(obj)` from `helpers/jsonCol.js` — serialize to JSON TEXT
- `rowToX(row)` functions — map DB row to JS object (parse JSON `data` column)
- `XToRow(obj)` functions — map JS object to DB row (stringify JSON `data`)
- `upsert(db, obj)` internal helpers — `INSERT … ON CONFLICT(id) DO UPDATE` pattern
- `db.transaction(() => { ... })` — synchronous callback-style txn (SQLite-idiomatic)

---

## 2. Per-Repository SQL Map

### 2.1 `src/lib/db/repos/settingsRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 47 | `SELECT data FROM settings WHERE id = 1` | SELECT | No | Backed by 5 s in-memory cache. |
| 2 | 88 | `SELECT data FROM settings WHERE id = 1` | SELECT | Yes | Re-reads inside txn for atomic merge. |
| 3 | 92 | `INSERT INTO settings(id, data) VALUES(1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data` | INSERT/UPSERT | Yes | `ON CONFLICT(id)` is single-column PK upsert. |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** `ON CONFLICT(id) DO UPDATE` (SQLite 3.24+; PG uses `ON CONFLICT (id) DO UPDATE`).

---

### 2.2 `src/lib/db/repos/connectionsRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 65 | `SELECT * FROM providerConnections${where.length ? ` WHERE ${where.join(" AND ")}` : ""}` | SELECT | No | Dynamic WHERE assembled from `provider` / `isActive` filters. Sorted in JS by `(a.priority||999)-(b.priority||999)`. |
| 2 | 74 | `SELECT * FROM providerConnections WHERE id = ?` | SELECT | No | By-id lookup. |
| 3 | 80 | `SELECT * FROM providerConnections WHERE provider = ?` | SELECT | Yes (`reorderInTx`) | Sorted by `priority ASC, updatedAt DESC` in JS. |
| 4 | 49-55 | `INSERT INTO providerConnections(id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET provider=excluded.provider, authType=excluded.authType, name=excluded.name, email=excluded.email, priority=excluded.priority, isActive=excluded.isActive, data=excluded.data, updatedAt=excluded.updatedAt` | UPSERT | Yes | Helper `upsert(db, c)`. |
| 5 | 87 | `UPDATE providerConnections SET priority = ? WHERE id = ?` | UPDATE | Yes (`reorderInTx`) | One row per connection in a loop (N+1). |
| 6 | 97 | `SELECT * FROM providerConnections WHERE provider = ?` | SELECT | Yes (`createProviderConnection`) | Pre-dedup scan for OAuth/apikey. |
| 7 | 161 | `SELECT * FROM providerConnections WHERE id = ?` | SELECT | Yes (`updateProviderConnection`) | Read-merge-write pattern. |
| 8 | 176 | `SELECT provider FROM providerConnections WHERE id = ?` | SELECT | Yes (`deleteProviderConnection`) | Capture provider for re-order. |
| 9 | 178 | `DELETE FROM providerConnections WHERE id = ?` | DELETE | Yes | |
| 10 | 187 | `SELECT COUNT(*) AS n FROM providerConnections WHERE provider = ?` | SELECT (aggregate) | No (`deleteProviderConnectionsByProvider`) | `COUNT(*)` aggregation. |
| 11 | 188 | `DELETE FROM providerConnections WHERE provider = ?` | DELETE | No | |
| 12 | 208 | `SELECT * FROM providerConnections` | SELECT | Yes (`cleanupProviderConnections`) | Full table scan. |

- **Joins/aggregations:** `COUNT(*)` (line 187). No `GROUP BY`/`JOIN`/subqueries.
- **SQLite-specific syntax:** `ON CONFLICT(id) DO UPDATE … excluded.col`.
- **Migration concerns:** Bulk re-priority update is N+1 (lines 86-88) — fine on SQLite, may want a CTE/array update on PG.

---

### 2.3 `src/lib/db/repos/nodesRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 46 | `SELECT * FROM providerNodes${where.length ? ` WHERE ${where.join(" AND ")}` : ""}` | SELECT | No | Filter: `type`. |
| 2 | 52 | `SELECT * FROM providerNodes WHERE id = ?` | SELECT | No | |
| 3 | 33-37 | `INSERT INTO providerNodes(id, type, name, data, createdAt, updatedAt) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET type=excluded.type, name=excluded.name, data=excluded.data, updatedAt=excluded.updatedAt` | UPSERT | Inside `upsert()` — used by `createProviderNode` (no explicit txn) and `updateProviderNode` (txn) | |
| 4 | 76 | `SELECT * FROM providerNodes WHERE id = ?` | SELECT | Yes (`updateProviderNode`) | |
| 5 | 89 | `SELECT * FROM providerNodes WHERE id = ?` | SELECT | Yes (`deleteProviderNode`) | |
| 6 | 92 | `DELETE FROM providerNodes WHERE id = ?` | DELETE | Yes | |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** UPSERT (`ON CONFLICT`).

---

### 2.4 `src/lib/db/repos/proxyPoolsRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 48 | `SELECT * FROM proxyPools${where.length ? ` WHERE ${where.join(" AND ")}` : ""}` | SELECT | No | Filters: `isActive`, `testStatus`. JS-sorted by `updatedAt DESC`. |
| 2 | 56 | `SELECT * FROM proxyPools WHERE id = ?` | SELECT | No | |
| 3 | 33-38 | `INSERT INTO proxyPools(id, isActive, testStatus, data, createdAt, updatedAt) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET isActive=excluded.isActive, testStatus=excluded.testStatus, data=excluded.data, updatedAt=excluded.updatedAt` | UPSERT | Inside `upsert()` | |
| 4 | 84 | `SELECT * FROM proxyPools WHERE id = ?` | SELECT | Yes | |
| 5 | 97 | `SELECT * FROM proxyPools WHERE id = ?` | SELECT | Yes | |
| 6 | 100 | `DELETE FROM proxyPools WHERE id = ?` | DELETE | Yes | |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** UPSERT.

---

### 2.5 `src/lib/db/repos/apiKeysRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 18 | `SELECT * FROM apiKeys ORDER BY createdAt ASC` | SELECT | No | |
| 2 | 24 | `SELECT * FROM apiKeys WHERE id = ?` | SELECT | No | |
| 3 | 42 | `INSERT INTO apiKeys(id, key, name, machineId, isActive, createdAt) VALUES(?, ?, ?, ?, ?, ?)` | INSERT | No | |
| 4 | 52 | `SELECT * FROM apiKeys WHERE id = ?` | SELECT | Yes | |
| 5 | 56 | `UPDATE apiKeys SET key = ?, name = ?, machineId = ?, isActive = ? WHERE id = ?` | UPDATE | Yes | |
| 6 | 66 | `DELETE FROM apiKeys WHERE id = ?` | DELETE | No | Reads `res.changes` (better-sqlite3 shape). |
| 7 | 72 | `SELECT isActive FROM apiKeys WHERE key = ?` | SELECT | No | **Hot path** (auth gate). |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** None exotic. `res.changes` is a better-sqlite3 return shape; adapter layer must normalize for PG (`rowCount`).

---

### 2.6 `src/lib/db/repos/combosRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 19 | `SELECT * FROM combos ORDER BY createdAt ASC` | SELECT | No | |
| 2 | 25 | `SELECT * FROM combos WHERE id = ?` | SELECT | No | |
| 3 | 31 | `SELECT * FROM combos WHERE name = ?` | SELECT | No | |
| 4 | 47 | `INSERT INTO combos(id, name, kind, models, createdAt, updatedAt) VALUES(?, ?, ?, ?, ?, ?)` | INSERT | No | `models` stored as JSON string. |
| 5 | 57 | `SELECT * FROM combos WHERE id = ?` | SELECT | Yes | |
| 6 | 61 | `UPDATE combos SET name = ?, kind = ?, models = ?, updatedAt = ? WHERE id = ?` | UPDATE | Yes | |
| 7 | 71 | `DELETE FROM combos WHERE id = ?` | DELETE | No | Uses `res.changes`. |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** None.

---

### 2.7 `src/lib/db/repos/aliasRepo.js`

This repo issues **no direct SQL** — it goes through `kvStore` helpers. Helper-emitted SQL is recorded under §3.1.

| API | KV operations | Scope |
|-----|---------------|-------|
| `getModelAliases` | `kv.getAll()` | `modelAliases` |
| `setModelAlias` | `kv.set()` | `modelAliases` |
| `deleteModelAlias` | `kv.remove()` | `modelAliases` |
| `getCustomModels` | `kv.getAll()` | `customModels` |
| `addCustomModel` (lines 36-43) | `SELECT 1 FROM kv WHERE scope = 'customModels' AND key = ?` + `INSERT INTO kv(scope, key, value) VALUES('customModels', ?, ?)` | `customModels` — wrapped in txn for dedup race |
| `deleteCustomModel` | `kv.remove()` | `customModels` |
| `getMitmAlias` | `kv.get` / `kv.getAll` | `mitmAlias` |
| `setMitmAliasAll` | `kv.set` | `mitmAlias` |

Direct SQL in `aliasRepo.js`:

| # | Line | SQL (verbatim) | Type | Inside Txn? |
|---|-----:|----------------|------|:-----------:|
| 1 | 38 | `SELECT 1 FROM kv WHERE scope = 'customModels' AND key = ?` | SELECT | Yes |
| 2 | 41 | `INSERT INTO kv(scope, key, value) VALUES('customModels', ?, ?)` | INSERT | Yes |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** None.

---

### 2.8 `src/lib/db/repos/pricingRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 67 | `SELECT value FROM kv WHERE scope = 'pricing' AND key = ?` | SELECT | Yes (`updatePricing`) | One read per provider in the loop. |
| 2 | 74 | `INSERT INTO kv(scope, key, value) VALUES('pricing', ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` | UPSERT | Yes | Composite PK `(scope, key)`. |
| 3 | 88 | `DELETE FROM kv WHERE scope = 'pricing' AND key = ?` | DELETE | Yes (`resetPricing`, provider-only branch) | |
| 4 | 91 | `SELECT value FROM kv WHERE scope = 'pricing' AND key = ?` | SELECT | Yes | |
| 5 | 95 | `DELETE FROM kv WHERE scope = 'pricing' AND key = ?` | DELETE | Yes | When merged object becomes empty. |
| 6 | 98 | `INSERT INTO kv(scope, key, value) VALUES('pricing', ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` | UPSERT | Yes | |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** Composite-PK `ON CONFLICT(scope, key)`.

---

### 2.9 `src/lib/db/repos/disabledModelsRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 8 | `SELECT key, value FROM kv WHERE scope = ?` | SELECT | No (`getDisabledModels`) | `scope='disabledModels'`. |
| 2 | 16 | `SELECT value FROM kv WHERE scope = ? AND key = ?` | SELECT | No | |
| 3 | 25 | `SELECT value FROM kv WHERE scope = ? AND key = ?` | SELECT | Yes (`disableModels`) | |
| 4 | 29 | `INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` | UPSERT | Yes | |
| 5 | 40 | `DELETE FROM kv WHERE scope = ? AND key = ?` | DELETE | Yes (`enableModels`, full clear) | |
| 6 | 43 | `SELECT value FROM kv WHERE scope = ? AND key = ?` | SELECT | Yes | |
| 7 | 48 | `DELETE FROM kv WHERE scope = ? AND key = ?` | DELETE | Yes | Empty-after-removal cleanup. |
| 8 | 51 | `INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` | UPSERT | Yes | |

- **Joins/aggregations:** None.
- **SQLite-specific syntax:** Composite-PK UPSERT.

---

### 2.10 `src/lib/db/repos/usageRepo.js` — most complex

#### 2.10.1 Write path (`saveRequestUsage` → `_flushWriteQueue` / `flushOnShutdown`)

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 294 | `INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)` | INSERT | Yes | Prepared once, run N times. |
| 2 | 299 | `INSERT INTO usageDaily(dateKey, data) VALUES(?, ?) ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data` | UPSERT | Yes | One per `dateKey` touched in batch. |
| 3 | 305 | `SELECT value FROM _meta WHERE key = 'totalRequestsLifetime'` | SELECT | Yes | Read-modify-write counter. |
| 4 | 307 | `INSERT INTO _meta(key, value) VALUES('totalRequestsLifetime', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value` | UPSERT | Yes | |
| 5 | 899 | `INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)` | INSERT | Yes (`flushOnShutdown`) | Mirror of #1. |
| 6 | 901 | `INSERT INTO usageDaily(dateKey, data) VALUES(?, ?) ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data` | UPSERT | Yes | Mirror of #2. |
| 7 | 903 | `SELECT value FROM _meta WHERE key = 'totalRequestsLifetime'` | SELECT | Yes | |
| 8 | 905 | `INSERT INTO _meta(key, value) VALUES('totalRequestsLifetime', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value` | UPSERT | Yes | |

#### 2.10.2 Read path — recent ring / live activity

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 9 | 114 | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint, cost, status, tokens FROM usageHistory ORDER BY id DESC LIMIT ?` | SELECT | No (`ensureRingInitialized`) | Populates 50-entry ring buffer. |
| 10 | 349 | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint, cost, status, tokens FROM usageHistory ${where} ORDER BY id ASC` | SELECT | No (`getUsageHistory`) | Dynamic WHERE: `provider`, `model`, `apiKey`, `timestamp >= ?`, `timestamp <= ?`. |
| 11 | 397 | `SELECT timestamp, provider, model, tokens, status FROM usageHistory ORDER BY id DESC LIMIT 100` | SELECT | No (`getUsageStats`) | Recent-requests overlay. |
| 12 | 456 | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ? AND timestamp <= ?` | SELECT | No | last10Minutes 10-min window. |
| 13 | 600 | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint FROM usageHistory WHERE timestamp >= ? AND apiKey = ?` | SELECT | No | `lastUsed` overlay (filtered). |
| 13b | 601 | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint FROM usageHistory WHERE timestamp >= ?` | SELECT | No | `lastUsed` overlay (unfiltered). |
| 14 | 637 | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, tokens FROM usageHistory WHERE timestamp >= ?${filterApiKey ? " AND apiKey = ?" : ""}` | SELECT | No | 24h/today live path. |
| 15 | 814 | `SELECT timestamp, provider, model, connectionId, promptTokens, completionTokens, status, tokens FROM usageHistory ORDER BY id DESC LIMIT ?` | SELECT | No (`getRecentLogs`) | |

#### 2.10.3 Read path — daily rollup (`usageDaily`)

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 16 | 360 | `SELECT dateKey, data FROM usageDaily` | SELECT | No (`loadDaysInRange` no-arg) | All days. |
| 17 | 365 | `SELECT dateKey, data FROM usageDaily WHERE dateKey >= ?` | SELECT | No | Filtered cutoff. |
| 18 | 735 | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ?${apiKeyFilter}` | SELECT | No (`getChartData` "today" branch) | |
| 19 | 758 | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ?${filterApiKey ? " AND apiKey = ?" : ""}` | SELECT | No (`getChartData` "24h" branch) | |
| 20 | 776 | (`loadDaysInRange(db, bucketCount)` — same as #17) | SELECT | No (`getChartData` 7d/30d/60d) | |

- **Joins/aggregations:** **None as SQL.** All aggregation (byProvider, byModel, byAccount, byApiKey, byEndpoint, last10Minutes) is performed in JavaScript after the read, using the JSON `usageDaily.data` blob. The counter on `_meta.totalRequestsLifetime` is the only true aggregate.
- **SQLite-specific syntax:** UPSERT on composite & single PK; prepared statements reused; `PRAGMA` settings from `schema.js` (`journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`).
- **Migration concerns:** Largest file — needs careful translation. `usageDaily` JSON-blob aggregation is the biggest design question: keep blob + JS aggregation, or normalize into rollup tables and use PG `SUM/GROUP BY` (more efficient at scale, more code to write).

---

### 2.11 `src/lib/db/repos/requestDetailsRepo.js`

| # | Line | SQL (verbatim) | Type | Inside Txn? | Notes |
|---|-----:|----------------|------|:-----------:|-------|
| 1 | 104 | `INSERT INTO requestDetails(id, timestamp, provider, model, connectionId, status, data) VALUES(?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET timestamp = excluded.timestamp, provider = excluded.provider, model = excluded.model, connectionId = excluded.connectionId, status = excluded.status, data = excluded.data` | UPSERT | Yes (`flushToDatabase` txn, per row) | Whole record serialized as JSON in `data`; columns 2-6 are denormalized for filter indexes. |
| 2 | 109 | `SELECT COUNT(*) as c FROM requestDetails` | SELECT (aggregate) | Yes | `COUNT(*)` used for trim threshold. |
| 3 | 112 | `DELETE FROM requestDetails WHERE id IN (SELECT id FROM requestDetails ORDER BY timestamp ASC LIMIT ?)` | DELETE w/ subquery | Yes | **Subquery + `IN`** — trim oldest rows to respect `maxRecords`. |
| 4 | 157 | `SELECT COUNT(*) as c FROM requestDetails ${where}` | SELECT (aggregate) | No (`getRequestDetails` pagination) | `COUNT(*)` for `totalItems`. |
| 5 | 166 | `SELECT data FROM requestDetails ${where} ORDER BY timestamp DESC LIMIT ? OFFSET ?` | SELECT | No | Pagination. |
| 6 | 179 | `SELECT data FROM requestDetails WHERE id = ?` | SELECT | No | |

- **Joins/aggregations:** `COUNT(*)` (×2), subquery (`SELECT id … ORDER BY timestamp ASC LIMIT ?` inside `IN`).
- **SQLite-specific syntax:** UPSERT; subquery inside `IN`.
- **Migration concerns:** Subquery in `IN` is portable. Denormalized columns (provider, model, connectionId, status) intentionally duplicated from `data` JSON for indexed filtering — keep this pattern (or move to JSONB-indexed PG queries).

---

### 2.12 `src/lib/usageDb.js`

Plain re-export shim. **No SQL.**

```js
export {
  statsEmitter, trackPendingRequest, getActiveRequests,
  saveRequestUsage, getUsageHistory, getUsageStats, getChartData,
  appendRequestLog, getRecentLogs,
  saveRequestDetail, getRequestDetails, getRequestDetailById,
} from "@/lib/db/index.js";
```

---

## 3. Helper-emitted SQL (referenced by repos)

### 3.1 `src/lib/db/helpers/kvStore.js`

| API | Line | SQL (verbatim) |
|-----|-----:|----------------|
| `get(key)` | 8 | `SELECT value FROM kv WHERE scope = ? AND key = ?` |
| `getAll()` | 13 | `SELECT key, value FROM kv WHERE scope = ?` |
| `set(key, value)` | 20 | `INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` |
| `setMany(obj)` | 26 | Same as `set` inside `db.transaction(() => …)` |
| `remove(key)` | 32 | `DELETE FROM kv WHERE scope = ? AND key = ?` |
| `clear()` | 36 | `DELETE FROM kv WHERE scope = ?` |

### 3.2 `src/lib/db/helpers/metaStore.js`

| API | Line | SQL (verbatim) |
|-----|-----:|----------------|
| `getMeta` | 5 | `SELECT value FROM _meta WHERE key = ?` |
| `setMeta` | 11 | `INSERT INTO _meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value` |
| `getMetaSync` | 16 | `SELECT value FROM _meta WHERE key = ?` (sync variant for migrations) |
| `setMetaSync` | 21 | `INSERT INTO _meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value` (sync variant) |

### 3.3 Adapter abstraction (`src/lib/db/driver.js`)

`getAdapter()` is awaited in every repo; returns an object exposing `get(sql, params) → row`, `all(sql, params) → rows[]`, `run(sql, params) → { changes, lastInsertRowid }`, `prepare(sql)`, `transaction(fn)`. The four driver implementations live in `src/lib/db/adapters/`:

- `bunSqliteAdapter.js` (Bun runtime)
- `betterSqliteAdapter.js` (Node, preferred)
- `nodeSqliteAdapter.js` (Node ≥22.5 built-in)
- `sqljsAdapter.js` (fallback)

All four emit the same logical SQL surface — they only differ in connection + driver nuances. The driver abstraction is the **natural seam for a PG port** (add a `pgAdapter.js`).

---

## 4. Most Complex / Critical Queries

These queries are the highest-risk surface for the K8s/PG migration because of size, frequency, or implicit assumptions baked into the data model.

1. **`usageRepo.js:294-307` (write batch txn)** — three operations inside a single transaction:
   - Bulk `INSERT` into `usageHistory` (N rows, up to 50 per batch).
   - `UPSERT` into `usageDaily` with full JSON-replacement of the day's data blob (last-writer-wins risk under concurrency).
   - Atomic read-modify-write of `_meta.totalRequestsLifetime` (`SELECT` + `UPSERT`).
   - **Why critical:** This is the hot path of the observability feature. Lost updates would silently misreport lifetime usage. PG migration must keep this atomic (use `BEGIN`/`COMMIT` + advisory lock or `SELECT … FOR UPDATE` on the counter row).

2. **`usageRepo.js:600-625` (`getUsageStats` overlay)** — full-history scan with optional `apiKey` filter to update `lastUsed` timestamps. With 60-day window this can be tens of thousands of rows. Currently unindexed `ORDER BY timestamp` after a `WHERE timestamp >= ?` (idx exists). On PG the same query plan should hold; verify the B-tree on `(apiKey, timestamp)` covers both branches.

3. **`usageRepo.js:776-799` (`getChartData` 7d/30d/60d)`** — load all `usageDaily` rows in the window, then JS-aggregate per bucket. The aggregation is O(days × keys). The decision to keep it in JS (vs SQL `SUM/GROUP BY`) is a deliberate trade-off and should be revisited for PG: a normalized rollup table + SQL aggregation would scale better, but adds write amplification.

4. **`usageRepo.js:455-468` (`last10Minutes` 10-bucket window)`** — pulls last 10 minutes of rows and bins them in JS. With 50 rows/min sustained traffic that's ~500 rows, fine. On PG the same plan is fine, but consider `date_trunc('minute', timestamp)` if scale grows.

5. **`connectionsRepo.js:79-89` (`reorderInTx`)`** — N+1 update loop. On PG acceptable for tens of rows; for thousands consider `UPDATE … FROM (VALUES (...)) AS v(priority, id) WHERE …`.

6. **`requestDetailsRepo.js:112-114` (trim)`** — `DELETE … WHERE id IN (SELECT id … ORDER BY timestamp ASC LIMIT ?)`. The subquery is correct, but on PG the planner may not push the `LIMIT` efficiently. Test with realistic row counts and consider `DELETE … USING (SELECT id … LIMIT N) sub WHERE requestDetails.id = sub.id`.

7. **`settingsRepo.js:84-99` (`updateSettings`)`** — atomic read-merge-write in a single txn. Same upsert pattern. PG equivalent is identical with `ON CONFLICT (id) DO UPDATE`.

8. **All `kv` table writes** — `INSERT … ON CONFLICT(scope, key) DO UPDATE`. The composite-PK upsert is a candidate for "upsert" boilerplate. On PG: identical syntax; ensure the `kv` table has a real composite PK (PG enforces it; SQLite tolerates a "PRIMARY KEY (a, b)" clause inside `CREATE TABLE`).

9. **Adapter return shape** — every `db.run` consumer expects `{ changes, lastInsertRowid }`. PG adapter must return `{ rowCount, … }` (driver normalizes).

10. **PRAGMAs** (`schema.js:4-12`):
    ```sql
    PRAGMA journal_mode = WAL;
    PRAGMA synchronous = NORMAL;
    PRAGMA temp_store = MEMORY;
    PRAGMA mmap_size = 30000000;
    PRAGMA cache_size = -64000;
    PRAGMA foreign_keys = ON;
    PRAGMA busy_timeout = 5000;
    ```
    None are valid in PG; they live in `PRAGMA_SQL` which is currently executed unconditionally on every adapter init. The PG adapter must skip this block.

---

## 5. PostgreSQL Migration Concerns

### 5.1 SQL syntax that needs translation

| SQLite construct | Where it appears | PG equivalent |
|------------------|------------------|---------------|
| `ON CONFLICT(id) DO UPDATE …` | `settingsRepo`, `connectionsRepo`, `nodesRepo`, `proxyPoolsRepo`, `usageRepo`, `requestDetailsRepo`, `kvStore`, `metaStore` | `ON CONFLICT (id) DO UPDATE …` (PG **requires** the target column in parens). |
| `ON CONFLICT(scope, key) DO UPDATE …` | `kvStore`, `pricingRepo`, `disabledModelsRepo` | `ON CONFLICT (scope, key) DO UPDATE …`. |
| `INTEGER PRIMARY KEY AUTOINCREMENT` (`usageHistory.id`) | `schema.js` | `BIGSERIAL` or `BIGINT GENERATED ALWAYS AS IDENTITY` (recommended for new installs). |
| `INTEGER PRIMARY KEY CHECK (id = 1)` (`settings.id`) | `schema.js` | `INTEGER PRIMARY KEY CHECK (id = 1)` — works in PG too. |
| `?` positional placeholders | Everywhere | `$1, $2, …` (PG `pg` driver) **or** keep `?` if you wrap with a `pg-format` / `named-placeholders` shim. The simplest path: change adapter interface to use `$1`/`$2` and rewrite all SQL strings — or introduce a placeholder translation layer. |
| `db.run` return shape `{ changes, lastInsertRowid }` | `apiKeysRepo`, `combosRepo` | PG returns `{ rowCount }`. Adapter must map. |
| `db.transaction(() => { ... })` synchronous callback | All repos | PG requires `BEGIN`/`COMMIT` (or `pg`'s `client.query('BEGIN')` etc.). Adapter must convert to async with proper error rollback. |
| `db.prepare(sql).run(...)` | `usageRepo` (inserts), `kvStore` (setMany) | PG: prepared statements are async (`client.query({ name, text, values })`) and bound to a connection. Adapter must hide the async-ness or expose a sync facade. |
| PRAGMAs (`PRAGMA_SQL`) | `schema.js` | Drop entirely on PG; not legal. |
| `dateKey` is `TEXT` formatted `YYYY-MM-DD` | `usageDaily` | PG: could be `DATE` for range queries / `to_char` rollups. Currently treated as opaque string. |
| `parseJson`/`stringifyJson` helpers reading/writing JSON-as-TEXT | All repos | PG: use `JSONB` columns and `jsonb_extract_path_text` etc. Migration path: keep `TEXT` columns for the first port (drop-in) and switch the type later. |

### 5.2 Transactional / concurrency differences

- **SQLite transactions are synchronous and run on a single connection.** The repos rely on `db.transaction(() => { ... })` being a blocking, all-or-nothing boundary (e.g., the OAuth refresh race in `connectionsRepo.updateProviderConnection` lines 156-170, the JSON merge in `pricingRepo.updatePricing` lines 62-81, the lifetime counter in `usageRepo` lines 291-308).
- **PG transactions are async and connection-bound.** The adapter must hold a single `pg.Client` for the duration of `db.transaction`, and roll back on throw. The current `async` repo signatures accommodate this if the adapter exposes a real async API. **Risk:** any `await` inside the transaction body (e.g., a dynamic import like `import("./connectionsRepo.js")` inside `usageRepo.getUsageStats` — that is **not** inside a transaction today, but if anyone moves it in, it would silently break SQLite semantics).
- **WAL mode, `busy_timeout=5000`** are SQLite concurrency tools. PG handles concurrency via MVCC + row-level locks. The `_meta.totalRequestsLifetime` increment would race on PG under load — switch to `UPDATE _meta SET value = (value::int + $1)::text WHERE key = 'totalRequestsLifetime' RETURNING value` (single statement, atomic).

### 5.3 Data-type / schema issues

- **`INTEGER` (32-bit) for `usageHistory.id`** — fine for ~2B rows but PG `BIGSERIAL` is safer for a K8s deployment that could outgrow it.
- **`cost REAL`** (`usageHistory.cost`) — SQLite `REAL` is 8-byte IEEE; PG `REAL` is 4-byte. Use `DOUBLE PRECISION` to preserve precision.
- **`isActive INTEGER DEFAULT 1`** stored as 0/1 — `BOOLEAN` is the idiomatic PG choice; migration can keep the column type and cast.
- **JSON-as-TEXT in `data`, `tokens`, `meta`, `models`, `usageDaily.data`, `requestDetails.data`** — JSONB in PG would enable real JSON queries; otherwise leave as `TEXT` for parity.
- **`timestamp TEXT`** (ISO 8601) — `TIMESTAMPTZ` is the PG equivalent; the strings are already ISO so a one-shot `str_to_timestamptz` could backfill. Out of scope for SQL rewriting but worth noting in the migration runbook.

### 5.4 Adapter / driver layer

- New file: `src/lib/db/adapters/pgAdapter.js` — implements `{ get, all, run, prepare, transaction, driver }` with the same surface.
- Add a 5th branch in `src/lib/db/driver.js` (or repurpose the chain) to prefer PG when `DATABASE_URL` is set.
- Move all `PRAGMA_SQL` execution behind a SQLite-only branch.
- Provide a `pgPool` (1+ per app instance) — `usageRepo`'s write queue must serialize to a single connection for the txn, or use explicit row-level locks for the lifetime counter.

### 5.5 Operational / K8s concerns

- **`usageHistory` is unbounded growth** — the only trim is the JSON-aggregate in `usageDaily`. PG: consider a time-based partition (`PARTITION BY RANGE (timestamp)`) and a retention job.
- **`requestDetails` is bounded** by `observabilityMaxRecords` (default 200). The `DELETE … IN (subquery LIMIT N)` is correct but at 200 rows it is fine; on PG, the trim runs in a single txn (per `flushToDatabase`).
- **`settingsCache` / `connCache` / module-level `cache` in `pricingRepo`** live in `global.*` to survive Next.js HMR — these are process-local. In K8s, multiple pods will each have their own cache; this is **acceptable for read-heavy settings** but the 5-second TTL means slightly delayed propagation of settings updates across pods. Not a SQL concern but worth flagging.
- **`usageWriteQueue` is process-local** — usage written to pod A is not visible to pod B until the flush completes. In a multi-replica deployment each pod has its own write queue and its own `recentRing`. The `_meta.totalRequestsLifetime` counter on a shared PG instance would correctly aggregate across pods, but the in-memory `pendingRequests` / `recentRing` are per-pod. Acceptable for the current observability use case.

### 5.6 Indexes to carry over (and add)

Current SQLite indexes (from `schema.js`):

- `providerConnections(provider)`, `providerConnections(provider, isActive)`, `providerConnections(provider, priority)`
- `providerNodes(type)`
- `proxyPools(isActive)`, `proxyPools(testStatus)`
- `apiKeys(key)` (UNIQUE constraint also on column)
- `combos(name)` (UNIQUE constraint also on column)
- `kv(scope)` + composite PK `(scope, key)`
- `usageHistory(timestamp DESC)`, `usageHistory(provider)`, `usageHistory(model)`, `usageHistory(connectionId)`, `usageHistory(apiKey)`, `usageHistory(apiKey, timestamp)`
- `requestDetails(timestamp DESC)`, `requestDetails(provider)`, `requestDetails(model)`, `requestDetails(connectionId)`

All should be recreated on PG. Consider **adding**:

- `usageHistory(timestamp, provider, model)` for composite range+filter queries.
- `usageDaily(dateKey)` already PK — no extra needed.
- PG: `jsonb_path_ops` GIN on `usageDaily.data`, `requestDetails.data` **only if** switching from JSON-as-TEXT to JSONB.

### 5.7 No-JOIN, no-subquery fact

Across **all 12 files** the only subquery is `requestDetailsRepo.js:112` (`DELETE … WHERE id IN (SELECT id … ORDER BY timestamp ASC LIMIT ?)`). The only `GROUP BY` is **none**. The only `JOIN` is **none**. Aggregation is entirely JS-side after reading JSON blobs. This is a significant simplification for the PG port: there are no JOIN-ordering, hash-aggregate, or planner-stability concerns to reason about. The risk surface is concentrated in (a) the upsert-heavy write paths and (b) the JSON-blob read-modify-write pattern in `usageDaily`.

### 5.8 SQLite-specific syntax: NOT used

Confirmed by sweep of all 12 files:

- `json_extract()` / `json_set()` / `json_group_array()` — **NOT used**. All JSON ops are in JS via `parseJson`/`stringifyJson`.
- `date('now')` / `strftime()` — **NOT used**. Timestamps are `new Date().toISOString()` in JS.
- `INSERT OR REPLACE` — **NOT used**. Upsert is via `ON CONFLICT(id) DO UPDATE` exclusively.
- `WITHOUT ROWID` — **NOT used**.
- `ATTACH DATABASE` — **NOT used**.

This means the only SQLite-isms to translate are the `ON CONFLICT` parenthesis-optional form, the `?` placeholders, and the `db.transaction`/`db.prepare` synchronous-callback API.

---

## 6. Quick Stats

| Metric | Count |
|--------|------:|
| Repos analyzed | 12 (11 repos + 1 shim) |
| Files with direct SQL | 10 |
| Distinct SQL statements (incl. helpers) | ~40 |
| `JOIN` usages | 0 |
| `GROUP BY` usages | 0 |
| Subqueries | 1 (`requestDetailsRepo.js:112`) |
| `COUNT(*)` aggregations | 3 (`connectionsRepo.js:187`, `requestDetailsRepo.js:109`, `requestDetailsRepo.js:157`) |
| `SUM` / `AVG` / `MIN` / `MAX` | 0 |
| `ON CONFLICT` upserts | 12 (5 unique shapes) |
| Transactions (`db.transaction`) | 19 call sites across 8 files |
| Prepared statements | 2 (`usageHistory` insert, `usageDaily` upsert) |
| PRAGMAs | 7 (must be dropped on PG) |
| Tables | 11 |

---

## 7. File-Line Index of All SQL (for quick navigation)

| File | Line | Statement (truncated) |
|------|-----:|-----------------------|
| `src/lib/db/repos/settingsRepo.js` | 47 | `SELECT data FROM settings WHERE id = 1` |
| `src/lib/db/repos/settingsRepo.js` | 88 | `SELECT data FROM settings WHERE id = 1` (in txn) |
| `src/lib/db/repos/settingsRepo.js` | 92 | `INSERT INTO settings(id, data) VALUES(1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data` |
| `src/lib/db/repos/connectionsRepo.js` | 49 | `INSERT INTO providerConnections(...) VALUES(...) ON CONFLICT(id) DO UPDATE SET ...` |
| `src/lib/db/repos/connectionsRepo.js` | 65 | `SELECT * FROM providerConnections [WHERE ...]` |
| `src/lib/db/repos/connectionsRepo.js` | 74 | `SELECT * FROM providerConnections WHERE id = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 80 | `SELECT * FROM providerConnections WHERE provider = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 87 | `UPDATE providerConnections SET priority = ? WHERE id = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 97 | `SELECT * FROM providerConnections WHERE provider = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 161 | `SELECT * FROM providerConnections WHERE id = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 176 | `SELECT provider FROM providerConnections WHERE id = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 178 | `DELETE FROM providerConnections WHERE id = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 187 | `SELECT COUNT(*) AS n FROM providerConnections WHERE provider = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 188 | `DELETE FROM providerConnections WHERE provider = ?` |
| `src/lib/db/repos/connectionsRepo.js` | 208 | `SELECT * FROM providerConnections` |
| `src/lib/db/repos/nodesRepo.js` | 33 | `INSERT INTO providerNodes(...) VALUES(...) ON CONFLICT(id) DO UPDATE SET ...` |
| `src/lib/db/repos/nodesRepo.js` | 46 | `SELECT * FROM providerNodes [WHERE ...]` |
| `src/lib/db/repos/nodesRepo.js` | 52 | `SELECT * FROM providerNodes WHERE id = ?` |
| `src/lib/db/repos/nodesRepo.js` | 76 | `SELECT * FROM providerNodes WHERE id = ?` |
| `src/lib/db/repos/nodesRepo.js` | 89 | `SELECT * FROM providerNodes WHERE id = ?` |
| `src/lib/db/repos/nodesRepo.js` | 92 | `DELETE FROM providerNodes WHERE id = ?` |
| `src/lib/db/repos/proxyPoolsRepo.js` | 33 | `INSERT INTO proxyPools(...) VALUES(...) ON CONFLICT(id) DO UPDATE SET ...` |
| `src/lib/db/repos/proxyPoolsRepo.js` | 48 | `SELECT * FROM proxyPools [WHERE ...]` |
| `src/lib/db/repos/proxyPoolsRepo.js` | 56 | `SELECT * FROM proxyPools WHERE id = ?` |
| `src/lib/db/repos/proxyPoolsRepo.js` | 84 | `SELECT * FROM proxyPools WHERE id = ?` |
| `src/lib/db/repos/proxyPoolsRepo.js` | 97 | `SELECT * FROM proxyPools WHERE id = ?` |
| `src/lib/db/repos/proxyPoolsRepo.js` | 100 | `DELETE FROM proxyPools WHERE id = ?` |
| `src/lib/db/repos/apiKeysRepo.js` | 18 | `SELECT * FROM apiKeys ORDER BY createdAt ASC` |
| `src/lib/db/repos/apiKeysRepo.js` | 24 | `SELECT * FROM apiKeys WHERE id = ?` |
| `src/lib/db/repos/apiKeysRepo.js` | 42 | `INSERT INTO apiKeys(id, key, name, machineId, isActive, createdAt) VALUES(?, ?, ?, ?, ?, ?)` |
| `src/lib/db/repos/apiKeysRepo.js` | 52 | `SELECT * FROM apiKeys WHERE id = ?` |
| `src/lib/db/repos/apiKeysRepo.js` | 56 | `UPDATE apiKeys SET key = ?, name = ?, machineId = ?, isActive = ? WHERE id = ?` |
| `src/lib/db/repos/apiKeysRepo.js` | 66 | `DELETE FROM apiKeys WHERE id = ?` |
| `src/lib/db/repos/apiKeysRepo.js` | 72 | `SELECT isActive FROM apiKeys WHERE key = ?` |
| `src/lib/db/repos/combosRepo.js` | 19 | `SELECT * FROM combos ORDER BY createdAt ASC` |
| `src/lib/db/repos/combosRepo.js` | 25 | `SELECT * FROM combos WHERE id = ?` |
| `src/lib/db/repos/combosRepo.js` | 31 | `SELECT * FROM combos WHERE name = ?` |
| `src/lib/db/repos/combosRepo.js` | 47 | `INSERT INTO combos(id, name, kind, models, createdAt, updatedAt) VALUES(?, ?, ?, ?, ?, ?)` |
| `src/lib/db/repos/combosRepo.js` | 57 | `SELECT * FROM combos WHERE id = ?` |
| `src/lib/db/repos/combosRepo.js` | 61 | `UPDATE combos SET name = ?, kind = ?, models = ?, updatedAt = ? WHERE id = ?` |
| `src/lib/db/repos/combosRepo.js` | 71 | `DELETE FROM combos WHERE id = ?` |
| `src/lib/db/repos/aliasRepo.js` | 38 | `SELECT 1 FROM kv WHERE scope = 'customModels' AND key = ?` |
| `src/lib/db/repos/aliasRepo.js` | 41 | `INSERT INTO kv(scope, key, value) VALUES('customModels', ?, ?)` |
| `src/lib/db/repos/pricingRepo.js` | 67 | `SELECT value FROM kv WHERE scope = 'pricing' AND key = ?` |
| `src/lib/db/repos/pricingRepo.js` | 74 | `INSERT INTO kv(scope, key, value) VALUES('pricing', ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` |
| `src/lib/db/repos/pricingRepo.js` | 88 | `DELETE FROM kv WHERE scope = 'pricing' AND key = ?` |
| `src/lib/db/repos/pricingRepo.js` | 91 | `SELECT value FROM kv WHERE scope = 'pricing' AND key = ?` |
| `src/lib/db/repos/pricingRepo.js` | 95 | `DELETE FROM kv WHERE scope = 'pricing' AND key = ?` |
| `src/lib/db/repos/pricingRepo.js` | 98 | `INSERT INTO kv(scope, key, value) VALUES('pricing', ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` |
| `src/lib/db/repos/disabledModelsRepo.js` | 8 | `SELECT key, value FROM kv WHERE scope = ?` |
| `src/lib/db/repos/disabledModelsRepo.js` | 16 | `SELECT value FROM kv WHERE scope = ? AND key = ?` |
| `src/lib/db/repos/disabledModelsRepo.js` | 25 | `SELECT value FROM kv WHERE scope = ? AND key = ?` |
| `src/lib/db/repos/disabledModelsRepo.js` | 29 | `INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` |
| `src/lib/db/repos/disabledModelsRepo.js` | 40 | `DELETE FROM kv WHERE scope = ? AND key = ?` |
| `src/lib/db/repos/disabledModelsRepo.js` | 43 | `SELECT value FROM kv WHERE scope = ? AND key = ?` |
| `src/lib/db/repos/disabledModelsRepo.js` | 48 | `DELETE FROM kv WHERE scope = ? AND key = ?` |
| `src/lib/db/repos/disabledModelsRepo.js` | 51 | `INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` |
| `src/lib/db/repos/usageRepo.js` | 114 | `SELECT … FROM usageHistory ORDER BY id DESC LIMIT ?` (ring buffer) |
| `src/lib/db/repos/usageRepo.js` | 294 | `INSERT INTO usageHistory(...) VALUES(?, ?, ?, …)` (prepared) |
| `src/lib/db/repos/usageRepo.js` | 299 | `INSERT INTO usageDaily(dateKey, data) VALUES(?, ?) ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data` |
| `src/lib/db/repos/usageRepo.js` | 305 | `SELECT value FROM _meta WHERE key = 'totalRequestsLifetime'` |
| `src/lib/db/repos/usageRepo.js` | 307 | `INSERT INTO _meta(key, value) VALUES('totalRequestsLifetime', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value` |
| `src/lib/db/repos/usageRepo.js` | 349 | `SELECT … FROM usageHistory ${where} ORDER BY id ASC` |
| `src/lib/db/repos/usageRepo.js` | 360 | `SELECT dateKey, data FROM usageDaily` |
| `src/lib/db/repos/usageRepo.js` | 365 | `SELECT dateKey, data FROM usageDaily WHERE dateKey >= ?` |
| `src/lib/db/repos/usageRepo.js` | 397 | `SELECT timestamp, provider, model, tokens, status FROM usageHistory ORDER BY id DESC LIMIT 100` |
| `src/lib/db/repos/usageRepo.js` | 456 | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ? AND timestamp <= ?` |
| `src/lib/db/repos/usageRepo.js` | 600/601 | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint FROM usageHistory WHERE timestamp >= ? [AND apiKey = ?]` |
| `src/lib/db/repos/usageRepo.js` | 637 | `SELECT … FROM usageHistory WHERE timestamp >= ? [AND apiKey = ?]` (24h/today) |
| `src/lib/db/repos/usageRepo.js` | 735 | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ? [AND apiKey = ?]` (today chart) |
| `src/lib/db/repos/usageRepo.js` | 758 | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ? [AND apiKey = ?]` (24h chart) |
| `src/lib/db/repos/usageRepo.js` | 814 | `SELECT … FROM usageHistory ORDER BY id DESC LIMIT ?` (recent logs) |
| `src/lib/db/repos/usageRepo.js` | 899 | `INSERT INTO usageHistory(...) VALUES(?, ?, …)` (shutdown flush) |
| `src/lib/db/repos/usageRepo.js` | 901 | `INSERT INTO usageDaily(dateKey, data) VALUES(?, ?) ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data` |
| `src/lib/db/repos/usageRepo.js` | 903 | `SELECT value FROM _meta WHERE key = 'totalRequestsLifetime'` |
| `src/lib/db/repos/usageRepo.js` | 905 | `INSERT INTO _meta(key, value) VALUES('totalRequestsLifetime', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value` |
| `src/lib/db/repos/requestDetailsRepo.js` | 104 | `INSERT INTO requestDetails(...) VALUES(...) ON CONFLICT(id) DO UPDATE SET …` |
| `src/lib/db/repos/requestDetailsRepo.js` | 109 | `SELECT COUNT(*) as c FROM requestDetails` |
| `src/lib/db/repos/requestDetailsRepo.js` | 112 | `DELETE FROM requestDetails WHERE id IN (SELECT id FROM requestDetails ORDER BY timestamp ASC LIMIT ?)` |
| `src/lib/db/repos/requestDetailsRepo.js` | 157 | `SELECT COUNT(*) as c FROM requestDetails ${where}` |
| `src/lib/db/repos/requestDetailsRepo.js` | 166 | `SELECT data FROM requestDetails ${where} ORDER BY timestamp DESC LIMIT ? OFFSET ?` |
| `src/lib/db/repos/requestDetailsRepo.js` | 179 | `SELECT data FROM requestDetails WHERE id = ?` |
| `src/lib/db/helpers/kvStore.js` | 8 | `SELECT value FROM kv WHERE scope = ? AND key = ?` |
| `src/lib/db/helpers/kvStore.js` | 13 | `SELECT key, value FROM kv WHERE scope = ?` |
| `src/lib/db/helpers/kvStore.js` | 20 | `INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value` |
| `src/lib/db/helpers/kvStore.js` | 26 | same as line 20 (in txn) |
| `src/lib/db/helpers/kvStore.js` | 32 | `DELETE FROM kv WHERE scope = ? AND key = ?` |
| `src/lib/db/helpers/kvStore.js` | 36 | `DELETE FROM kv WHERE scope = ?` |
| `src/lib/db/helpers/metaStore.js` | 5 | `SELECT value FROM _meta WHERE key = ?` |
| `src/lib/db/helpers/metaStore.js` | 11 | `INSERT INTO _meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value` |

---

*Generated for imp4 — Kubernetes PostgreSQL Migration Analysis*
