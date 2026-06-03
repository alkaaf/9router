# Read Operations & Query Pattern Analysis

**Scope:** All SELECT queries across `src/lib/db/repos/*` and the per-request hot path.
**Goal:** Identify which reads become bottlenecks in a multi-pod Kubernetes deployment and quantify the migration benefit of moving from SQLite to PostgreSQL.

---

## 1. Read Operation Summary by Table

### 1.1 `apiKeys` (auth & dashboard)

| Operation | SQL | Repo / File | Hot? |
|---|---|---|---|
| Validate key (auth) | `SELECT isActive FROM apiKeys WHERE key = ?` | `apiKeysRepo.validateApiKey` (line 70) | **Yes — every request** |
| List all keys | `SELECT * FROM apiKeys ORDER BY createdAt ASC` | `getApiKeys` (line 16) | Dashboard |
| Get by id | `SELECT * FROM apiKeys WHERE id = ?` | `getApiKeyById` (line 22) | UI only |
| Update | `SELECT … WHERE id = ?` + `UPDATE …` in tx | `updateApiKey` (line 48) | UI only |
| Insert | `INSERT INTO apiKeys …` | `createApiKey` (line 28) | UI only |
| Delete | `DELETE FROM apiKeys WHERE id = ?` | `deleteApiKey` (line 64) | UI only |

**Notes**
- `validateApiKey` only fetches the `isActive` boolean — the *minimal* payload needed for auth. Good shape for index-only scan.
- `key` column has a UNIQUE constraint + `idx_ak_key` index → `WHERE key = ?` is a single B-tree lookup.
- Multi-pod concern: under SQLite a single process holds the DB; under PG the auth lookup is now a network round-trip *per request*. See §3.

### 1.2 `settings` (hot path — 3× per request)

| Operation | SQL | Repo / File | Hot? |
|---|---|---|---|
| Read raw | `SELECT data FROM settings WHERE id = 1` | `settingsRepo.readRaw` (line 45) | **Yes — every cache miss** |
| Atomic update | `SELECT data …` + `INSERT … ON CONFLICT(id) DO UPDATE SET data = …` in tx | `updateSettings` (line 84) | UI / config |
| Cloud URL | (in-memory) `await getSettings().cloudUrl` | `getCloudUrl` (line 106) | Wrapper |
| Cloud enabled | (in-memory) `await getSettings().cloudEnabled` | `isCloudEnabled` (line 101) | Wrapper |
| Export | `await readRaw()` | `exportSettings` (line 116) | Backup / migration |

**Notes**
- `settings` table has exactly **one row** (`id = 1`).
- Cached in-memory for **5 s** (`SETTINGS_CACHE_TTL_MS = 5000`) — under SQLite this means a process-local cache, so each pod has its own copy. After settings change, *up to 5 s per pod* before it propagates.
- The single `SELECT` returns one row → trivial cost, but in a multi-pod world: each pod's first read after TTL expiry incurs a network round-trip; subsequent reads for 5 s are free.

### 1.3 `providerConnections` (priority ordering)

| Operation | SQL | Repo / File | Hot? |
|---|---|---|---|
| Filtered list (used for rotation) | `SELECT * FROM providerConnections WHERE provider = ? AND isActive = ?` | `getProviderConnections` (line 59), called with `{ provider, isActive: true }` from `auth.js:55, :201` | **Yes — every request** |
| All connections | (no `WHERE`) | `getProviderConnections({})` | Dashboard / stats |
| By id | `SELECT * FROM providerConnections WHERE id = ?` | `getProviderConnectionById` (line 72) | UI |
| Reorder (priority sync) | `SELECT * … WHERE provider = ?` then per-row `UPDATE` | `reorderInTx` (line 79) | Mutation only |
| Create / update | `INSERT … ON CONFLICT(id) DO UPDATE …` | `createProviderConnection` (line 91), `updateProviderConnection` (line 157) | UI |
| Delete (single / by provider) | `DELETE FROM providerConnections WHERE id = ?` / `WHERE provider = ?` | `deleteProviderConnection` (line 172), `deleteProviderConnectionsByProvider` (line 185) | UI |
| Cleanup | `SELECT * FROM providerConnections` (full scan) + per-row upsert | `cleanupProviderConnections` (line 197) | Maintenance |
| Count (delete-by-provider) | `SELECT COUNT(*) … WHERE provider = ?` | `deleteProviderConnectionsByProvider` (line 187) | Mutation |

**Notes**
- Three indexes: `provider`, `(provider, isActive)`, `(provider, priority)`. The hot filter `WHERE provider = ? AND isActive = ?` is **exactly covered** by `idx_pc_provider_active`.
- `getProviderConnections` does an in-memory sort by `priority` after the SQL fetch — small, bounded by row count per provider, no issue.
- `getConnectionMapCached` in `usageRepo` (line 96) caches the full map for **30 s**; the inner code falls through to `getProviderConnections()` (no filter) so it touches `idx_pc_provider` (single column index).
- Per-request usage: at least once (in `auth.js` for the chat/combo pipeline) and possibly twice if combo lookup happens. Both call paths are filtered by `provider + isActive`.

### 1.4 `usageHistory` (per-request, stats)

| Operation | SQL | Repo / File | Hot? |
|---|---|---|---|
| Recent ring (boot) | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint, cost, status, tokens FROM usageHistory ORDER BY id DESC LIMIT ?` (50) | `ensureRingInitialized` (line 109) | Cold start only |
| Recent logs | same as above, larger LIMIT (default 200) | `getRecentLogs` (line 810) | Dashboard |
| `getRecentLogs` / `getUsageStats` 100-row recent | `SELECT timestamp, provider, model, tokens, status FROM usageHistory ORDER BY id DESC LIMIT 100` | `getUsageStats` (line 397) | Dashboard |
| Last 10 min bucket | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ? AND timestamp <= ?` | `getUsageStats` (line 455) | Dashboard |
| `getUsageHistory` (filtered) | `SELECT … FROM usageHistory WHERE <dynamic> ORDER BY id ASC` (provider, model, apiKey, startDate, endDate) | `getUsageHistory` (line 337) | Dashboard |
| 24h/today live agg | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, tokens FROM usageHistory WHERE timestamp >= ?` (+ optional `AND apiKey = ?`) | `getUsageStats` 24h branch (line 636) | Dashboard |
| 7d/30d/60d lastUsed overlay | `SELECT timestamp, provider, model, connectionId, apiKey, endpoint FROM usageHistory WHERE timestamp >= ?` (+ optional `AND apiKey = ?`) | `getUsageStats` overlay (line 599) | Dashboard |
| Chart today (24h) | `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ?` (+ optional `AND apiKey = ?`) | `getChartData` "today" (line 734) | Dashboard |
| Chart 24h (rolling) | same shape, different cutoff | `getChartData` "24h" (line 757) | Dashboard |
| Chart 7d/30d/60d | **derived from `usageDaily`** (not `usageHistory`) | `getChartData` (line 776) | Dashboard |
| INSERT (write) | `INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)` | `_flushWriteQueue` (line 292) | **Yes — every request, batched** |
| Read in stats overlay | see above | same | Dashboard |

**Notes**
- 6 indexes: `timestamp DESC`, `provider`, `model`, `connectionId`, `apiKey`, `(apiKey, timestamp)`.
- Time-range queries use `timestamp >= ?`; the index `idx_uh_ts` is `timestamp DESC` so PG can scan it in reverse for ascending reads (still sargable).
- The chart `today` query has **no upper bound on `timestamp`** — relies on `idx_uh_ts`.
- INSERT path is batched (50 rows / 1 s) and runs in a transaction that *also* updates `usageDaily` + `_meta` — see §3.

### 1.5 `usageDaily` (precomputed aggregations)

| Operation | SQL | Repo / File | Hot? |
|---|---|---|---|
| All days | `SELECT dateKey, data FROM usageDaily` | `loadDaysInRange` (line 360) | Dashboard (7d/30d/60d) |
| Range | `SELECT dateKey, data FROM usageDaily WHERE dateKey >= ?` | `loadDaysInRange` (line 365) | Dashboard |
| Upsert | `INSERT INTO usageDaily(dateKey, data) VALUES(?, ?) ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data` | `_flushWriteQueue` (line 298) | Per request batch |

**Notes**
- This table is the **JSON-extraction hot path** (see §2.5 and §3.1).
- `dateKey` is the primary key → B-tree lookups on `>= ?` are range scans.
- The `data` column is a single TEXT blob — *all aggregation fields* (`byProvider`, `byModel`, `byAccount`, `byApiKey`, `byEndpoint`) live inside that JSON.
- `getUsageStats` and `getChartData` both pull `usageDaily` rows into memory and aggregate in JS — there is **no SQL aggregation** at all.

### 1.6 `requestDetails` (observability)

| Operation | SQL | Repo / File | Hot? |
|---|---|---|---|
| Paged list with filters | `SELECT COUNT(*) …` then `SELECT data FROM requestDetails WHERE … ORDER BY timestamp DESC LIMIT ? OFFSET ?` | `getRequestDetails` (line 144) | Dashboard |
| Get by id | `SELECT data FROM requestDetails WHERE id = ?` | `getRequestDetailById` (line 177) | Dashboard |
| Truncate oldest (write) | `DELETE FROM requestDetails WHERE id IN (SELECT id FROM requestDetails ORDER BY timestamp ASC LIMIT ?)` | `flushToDatabase` (line 111) | Bounded by `maxRecords` |
| Insert | `INSERT INTO requestDetails … ON CONFLICT(id) DO UPDATE SET …` | `flushToDatabase` (line 103) | Per request (batched) |

**Notes**
- Capped at `observabilityMaxRecords` (default 200) → always small, no scaling concern.
- 4 indexes (`timestamp DESC`, `provider`, `model`, `connectionId`) — all single-column.

### 1.7 Other tables (cold / non-hot)

| Table | Repo | Notes |
|---|---|---|
| `combos` | `combosRepo.js` | Looked up by `name` (UI route `/v1/models`), by `id` (admin). Small set, indexed. |
| `proxyPools` | `proxyPoolsRepo.js` | Read at request time when proxy enabled; bounded count. |
| `providerNodes` | `nodesRepo.js` | Read in `getUsageStats` to map provider IDs to display names. Cached separately. |
| `disabledModels` | `disabledModelsRepo.js` | `kv` scope — read on `/v1/models` listing. Bounded. |
| `_meta` | `helpers/metaStore.js`, `_flushWriteQueue` | `totalRequestsLifetime` counter. |
| `kv` (other scopes) | `kvStore.js` (modelAliases, customModels, mitmAlias, pricing) | Used in `/v1/models` and pricing hot path. |

---

## 2. Query Pattern Analysis

### 2.1 Simple lookup by PK / unique index
- `apiKeys` by `key` (UNIQUE + idx_ak_key)
- `apiKeys` by `id` (PK)
- `combos` by `id` (PK) and `name` (UNIQUE + idx_combo_name)
- `settings` by `id = 1` (PK, 1-row table)
- `providerConnections` by `id` (PK)
- `proxyPools` by `id` (PK)
- `requestDetails` by `id` (PK)
- `usageDaily` by `dateKey` (PK)

All single-row, B-tree lookup. Trivially fast on either SQLite or PG.

### 2.2 Lookup by indexed column(s)
- `providerConnections` filter `provider = ? AND isActive = ?` → `idx_pc_provider_active` (composite).
- `usageHistory` filter `apiKey = ?` → `idx_uh_apiKey`.
- `usageHistory` filter `(apiKey = ? AND timestamp >= ?)` → `idx_uh_apiKey_ts` (composite).
- `usageHistory` time range `timestamp >= ?` → `idx_uh_ts` (DESC).

Composite indexes in the codebase: 4 (`idx_pc_provider_active`, `idx_pc_provider_priority`, `idx_uh_apiKey_ts`, plus PK on `kv(scope,key)`).

### 2.3 Aggregations
- **No SQL-level aggregation** in any repo. All aggregations are performed in JS after fetching the rows.
- `getUsageStats` (line 368) pulls `usageDaily` rows for the period, parses JSON for each, then loops building `byProvider`, `byModel`, `byAccount`, `byApiKey`, `byEndpoint` maps in memory.
- `getChartData` does the same for daily buckets.
- For 24h/today the code switches to scanning `usageHistory` directly and aggregating per row in JS (line 626+).
- The 10-min bucket (line 455) is also a JS aggregation over a fetched rowset.
- Net effect: in PG, every dashboard hit pulls a JSON blob per day and parses in Node — work that the DB could do natively with `SUM`, `jsonb_path_query`, etc.

### 2.4 Date-range queries
- `usageHistory WHERE timestamp >= ? AND timestamp <= ?` — used in `getUsageStats` 10-min bucket and in `getChartData` "today"/"24h".
- `usageHistory WHERE timestamp >= ?` — used in `getUsageStats` 24h/today and overlay queries.
- `usageDaily WHERE dateKey >= ?` — used in `loadDaysInRange`.

All are sargable and use the appropriate index. Performance is fine on either engine.

### 2.5 JSON extraction
The 9Router codebase never extracts JSON inside SQL — there is **zero** use of `json_extract` / `->>` / etc. Aggregation is always done in JS. This is a deliberate design choice: `usageDaily.data` is stored as TEXT and parsed in Node via `parseJson()`.

**Consequence:** on PostgreSQL the equivalent of "JSON extraction" doesn't exist in the current code — instead, the whole `data` blob is shipped to Node and parsed there. The performance problem is **bandwidth + parse cost**, not DB CPU.

---

## 3. Index Usage Assessment

### 3.1 Indexes declared (`schema.js`)

| Table | Index | Columns | Hot query? |
|---|---|---|---|
| `apiKeys` | `idx_ak_key` | `key` | `validateApiKey` |
| `settings` | (PK only) | `id` | `readRaw` |
| `providerConnections` | `idx_pc_provider` | `provider` | `cleanupProviderConnections` (full scan) |
| `providerConnections` | `idx_pc_provider_active` | `provider, isActive` | `getProviderConnections({ provider, isActive })` (HOT) |
| `providerConnections` | `idx_pc_priority` | `provider, priority` | Used in ordering; the SQL has no ORDER BY priority so this is unused at read time |
| `providerNodes` | `idx_pn_type` | `type` | `getProviderNodes({ type })` |
| `proxyPools` | `idx_pp_active` | `isActive` | `getProxyPools({ isActive })` |
| `proxyPools` | `idx_pp_status` | `testStatus` | `getProxyPools({ testStatus })` |
| `combos` | `idx_combo_name` | `name` | `getComboByName` |
| `kv` | `idx_kv_scope` | `scope` | All `makeKv` reads |
| `usageHistory` | `idx_uh_ts` | `timestamp DESC` | All time-range + recent-N queries |
| `usageHistory` | `idx_uh_provider` | `provider` | `getUsageHistory({ provider })` |
| `usageHistory` | `idx_uh_model` | `model` | `getUsageHistory({ model })` |
| `usageHistory` | `idx_uh_conn` | `connectionId` | `getUsageHistory({ connectionId })`, `getRequestDetails({ connectionId })` |
| `usageHistory` | `idx_uh_apiKey` | `apiKey` | `getUsageHistory({ apiKey })` |
| `usageHistory` | `idx_uh_apiKey_ts` | `apiKey, timestamp` | `getUsageStats` 24h/today + overlay + `getChartData` 24h |
| `requestDetails` | `idx_rd_ts` | `timestamp DESC` | `getRequestDetails` ORDER BY |
| `requestDetails` | `idx_rd_provider` | `provider` | `getRequestDetails({ provider })` |
| `requestDetails` | `idx_rd_model` | `model` | `getRequestDetails({ model })` |
| `requestDetails` | `idx_rd_conn` | `connectionId` | `getRequestDetails({ connectionId })` |

### 3.2 Effective index coverage
- All hot-path filters have a matching index.
- Two unused-by-queries candidates: `idx_pc_priority` (no SQL `ORDER BY priority` anywhere; the sort is in JS) and `idx_uh_model` (only the 5-row-equivalent `getUsageHistory({ model })` uses it; mostly redundant with `idx_uh_provider`).
- PG will benefit from these for free via B-tree planning; the unused ones are no-ops.

---

## 4. Potential Performance Issues

### 4.1 JSON blob pull + JS aggregation (`usageDaily`)
- **Symptom:** `getUsageStats` and `getChartData` pull all `usageDaily.data` blobs into Node, then re-parse and re-aggregate. With N days and growing JSON size, the cost is `O(N * blob_size)` of *Node work*, plus the row-size shipped over the wire (in K8s/PG, every dashboard hit ships N blobs back).
- **Source of truth in code:** `usageRepo.js:358-366` (`loadDaysInRange`), `:470-558` (aggregation loop), `:776-799` (chart loop).
- **PG improvement:** switch the `data` column to `jsonb` and let PG do `SUM((d.data->'byProvider'->>'openai')::numeric)` style aggregations server-side, returning one row per period. Or precompute provider/model/account totals into normalized child tables written by the same flush job (no extra round-trip in the read path).

### 4.2 `kv` table is a multi-purpose scope bag
- **Symptom:** `kv` holds 5+ logical "tables" (`modelAliases`, `customModels`, `mitmAlias`, `pricing`, `disabledModels`). `kvStore.getAll()` does `SELECT key, value FROM kv WHERE scope = ?` which scans the whole index range for the scope. There is *no* index on `key` within a scope — only on `scope` (so lookups in the same scope are sequential).
- **Source of truth:** `src/lib/db/helpers/kvStore.js:11-17`, `schema.js:96-104`.
- **PG improvement:** split each scope into its own table (or one table with `(scope, key)` PK — *which exists* — but no covering index for `key` lookups). The `idx_kv_scope` index is sufficient under PG (composite PK), but the table would still benefit from normalization.

### 4.3 Settings cache TTL in a multi-pod world
- **Symptom:** `settingsCache` lives on `global._settingsCache` — *per pod*. If pod A updates a setting, pods B/C keep stale settings for up to 5 s. Worse: there is no cache invalidation signal between pods; the 5 s TTL is the only coherence mechanism.
- **Source of truth:** `settingsRepo.js:42-43`, `:70-99`.
- **PG improvement:** centralize in PG (the source is one row anyway), keep the 5 s TTL as a per-pod read cache, and (optional) emit a `LISTEN/NOTIFY` event for instant invalidation. The 5 s TTL itself remains valid as a request-rate limiter.

### 4.4 Auth: `validateApiKey` is one round-trip per request under PG
- **Symptom:** under SQLite, `validateApiKey` is a synchronous memory read. Under PG, every API request that uses a key issues a network query. This is the canonical PG migration hot-path concern.
- **Source of truth:** `apiKeysRepo.js:70-75`, called from `src/sse/services/auth.js:305-308`, reached from every handler in `src/sse/handlers/*`.
- **Mitigation options:**
  - Connection pool (PgBouncer / native `pg.Pool`) to keep a warm set of connections.
  - Per-pod LRU of `(key → {isActive, expiresAt})` with short TTL (e.g. 30 s) to absorb chat bursts.
  - For internal-to-pod callers, hoist `validateApiKey` calls out of inner loops (e.g. combo fan-out: validate once per request, not per sub-call).

### 4.5 In-process write queue does not survive across pods
- **Symptom:** `global._usageWriteQueue` (line 24) is per pod. If pod A receives N requests and is killed, those queued usage entries are lost (the shutdown handler best-effort flushes, but SIGKILL is uncatchable). In a Kubernetes world, pods churn — this becomes a data-loss risk.
- **Source of truth:** `usageRepo.js:14-26`, `:225-322`, `:842-913`.
- **PG improvement:** write directly in a single INSERT per batch (no in-memory queue), or use a single durable PG-backed queue table. Even the in-memory batching can be kept — but the source of truth is PG, not RAM.

### 4.6 `cleanupProviderConnections` full scan
- **Symptom:** `SELECT * FROM providerConnections` then per-row upsert. Bounded by connection count (typically <100), so not a real issue, but listed for completeness.
- **Source of truth:** `connectionsRepo.js:197-225`.

### 4.7 Settings read is 3× per request in chat handler
- **Symptom:** `chat.js:69, :127, :201` each call `getSettings()`. The 5 s in-memory cache amortizes this within a pod, so the actual DB hit is once per pod per 5 s, not 3× per request. Still: a single bundled read at handler entry that returns a frozen snapshot would make the contract explicit and prevent accidental stale reads.
- **Source of truth:** `src/sse/handlers/chat.js:69, :127, :201`.

---

## 5. PostgreSQL Migration Benefits (by read pattern)

| Read pattern | SQLite today | PG benefit |
|---|---|---|
| Auth (`validateApiKey`) | Sub-ms in-process | Sub-ms with `pg.Pool`; + LRU cache eliminates the round-trip in the common case. |
| Settings (`getSettings`) | Per-pod 5 s cache → 0 or 1 hit/5 s | Same per-pod 5 s cache; `LISTEN/NOTIFY` gives instant cross-pod invalidation. |
| `getProviderConnections` filtered | In-process B-tree | In-process plan to B-tree; identical latency. |
| `usageDaily` JSON aggregation | Pull blob → JS parse → JS aggregate | `jsonb` + native aggregates push work to DB; ship one row back. **Biggest win.** |
| `usageHistory` 24h / today | Full scan of one day in-process | Index range scan; PG parallelism for very large days. |
| `getChartData` 7d/30d/60d | Read 7-60 rows × JSON blob | Same row count but with `jsonb` + `SUM` returns aggregated numbers directly. |
| `getRequestDetails` paginated | Trivial | Trivial; benefit is concurrent reads from many pods. |
| `_meta` (lifetime counter) | Read-modify-write in tx → contended | `UPDATE … RETURNING` is atomic; or move to a single-row "stat" view. |
| `kv` lookups | Per-scope index | Composite PK already exists; behavior identical. |
| Schema migrations | Custom migration runner | PG `LISTEN/NOTIFY` for instant cross-pod cache invalidation; managed schemas. |

### 5.1 Concrete PG wins
1. **`jsonb` for `usageDaily.data` and `requestDetails.data`** — enables GIN indexes if the dashboard starts filtering by `byApiKey` keys, and lets aggregations stay in SQL.
2. **Real connection pooling** (PgBouncer transaction-mode + `pg.Pool` app-side) — removes the per-request handshake cost that PG adds over SQLite.
3. **Native async** — every repo is already `async`; the only thing blocking under PG is the actual network I/O, which is a constant per round-trip.
4. **`LISTEN/NOTIFY` for settings / connection-map invalidation** — kills the multi-pod stale-cache window (down to ms from s).
5. **Materialized views for dashboard cards** — `getUsageStats(period)` could become `SELECT FROM mv_usage_daily_provider WHERE dateKey >= ?`; refreshed on flush.
6. **Row-level locking via `SELECT … FOR UPDATE`** — currently the SQLite `db.transaction` provides this. PG needs explicit `FOR UPDATE` on the priority reorder path (`reorderInTx`).
7. **Time partitioning for `usageHistory`** — the table grows unbounded; PG native partitioning by `timestamp` (weekly/monthly) keeps the hot indexes small.

### 5.2 Things to preserve from SQLite
- The batched write path (50 entries / 1 s) is already a good shape for PG — no need to change.
- The 5 s settings cache is the right per-pod rate limiter under PG too.
- The connection-map cache (30 s) inside `usageRepo` continues to make sense.

---

## 6. Summary

- **Hot path (every request):** `validateApiKey`, `getSettings` (cached), `getProviderConnections({ provider, isActive: true })` filtered, plus the append path into `usageHistory` (batched).
- **Hot path (dashboard):** `getUsageStats`, `getChartData`, `getRequestDetails`, `getUsageHistory`. All currently do JS-side aggregation over `usageDaily` JSON blobs.
- **Bottleneck #1:** `usageDaily.data` JSON aggregation in JS — replace with `jsonb` + SQL aggregates.
- **Bottleneck #2:** per-pod in-memory state (`global._usageWriteQueue`, `global._settingsCache`, `pendingRequests`) does not survive pod churn — needs PG-backed truth.
- **Bottleneck #3 (latency, not throughput):** `validateApiKey` becomes a network round-trip under PG; needs `pg.Pool` + per-pod LRU.
- **Indexes are well-shaped** for the current SQL — only `idx_pc_priority` looks unused. No missing-index hot spots.

The migration is mostly a *latency* story (round-trips, network, JSON parsing in Node) rather than a *correctness* story. The schema and indexes are already close to what PG would want; the work is in (a) replacing the in-memory state with PG truth, (b) moving aggregations server-side via `jsonb`, and (c) adding a small app-level LRU for auth to keep p99 latency low.
