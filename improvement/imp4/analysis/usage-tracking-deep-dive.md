# Usage Tracking System — Deep Dive (imp4)

> **Scope:** The write-heavy usage pipeline that records every LLM API call, aggregates
> it for the dashboard, and pushes real-time updates to connected UI clients.
> This is the single most performance-critical subsystem in 9Router, and the
> hardest to migrate to PostgreSQL+Kubernetes.

---

## 1. UsageRepo Function Map

File: `src/lib/db/repos/usageRepo.js` (914 lines)

| Export | Kind | Purpose | Critical? |
|---|---|---|---|
| `saveRequestUsage(entry)` | WRITE | Queue an LLM-call record for batched persistence | **YES** — called on every chat completion |
| `flushWriteQueue()` | WRITE | Force-drain the write queue (tests, shutdown) | yes |
| `trackPendingRequest(model, provider, connectionId, started, error)` | IN-MEM | Increment/decrement active-request counter; emit "pending" SSE | yes |
| `getActiveRequests()` | READ | Snapshot of in-flight + recently-completed requests from ring buffer | yes |
| `getUsageStats(period, filter)` | READ | Aggregated totals + byProvider/byModel/byApiKey/byAccount/byEndpoint | **YES** — primary dashboard query |
| `getChartData(period, filter)` | READ | Time-bucketed token/cost series for charts | yes |
| `getUsageHistory(filter)` | READ | Raw usageHistory rows for per-key views | yes |
| `getRecentLogs(limit)` | READ | Latest N rows formatted as log lines | yes |
| `appendRequestLog()` | no-op | Legacy hook — no longer writes anywhere (see line 808 comment) | no |
| `statsEmitter` | EVENT | In-process `EventEmitter`; "update" / "pending" events fan out to SSE clients | yes |

Internal helpers (not exported): `aggregateEntryToDay`, `addToCounter`, `pushToRing`, `getLocalDateKey`, `getConnectionMapCached`, `ensureRingInitialized`, `loadDaysInRange`, `_flushWriteQueue`, `scheduleFlush`, `formatLogDate`, `flushOnShutdown`.

---

## 2. The Two Write Repos

There are **two independent batching write paths** that must each be migrated:

### 2a. `usageRepo.js` — `saveRequestUsage` → `usageHistory` + `usageDaily` + `_meta`

Three write targets per batched flush, all in one `db.transaction()`.

### 2b. `requestDetailsRepo.js` — `saveRequestDetail` → `requestDetails`

Observability/debugging telemetry. Gated by `settings.enableObservability`.
Self-pruning: if `COUNT(*) > maxRecords`, deletes oldest by `timestamp ASC`.
Larger payload (full request/response bodies truncated to `maxJsonSize` bytes).

Both share the same SQLite adapter contract (`run`, `get`, `all`, `exec`, `transaction`).

---

## 3. `saveRequestUsage()` Step-by-Step Flow

```text
chatCore (open-sse) calls saveUsageStats(...)
        │
        ▼
saveRequestUsage(entry)         [usageRepo.js:215]
   1. set entry.timestamp if missing
   2. writeQueue.push({...entry, _queuedAt: now})
   3. scheduleFlush()             (1s debounce timer)
   4. RETURN IMMEDIATELY (no I/O on hot path!)
        │
        │   ... up to 1s later, OR when WRITE_BATCH_MAX=50 reached ...
        │
        ▼
_flushWriteQueue()              [usageRepo.js:225]
   ┌─ 1. dedup pricing lookups  (P2-1/P2-2 hot path optimization)
   │     • build `pricingMap` keyed by "provider:model"
   │     • Promise.all unique lookups
   │     • single pricingMap read per entry
   │
   ├─ 2. compute cost + normalize tokens for each entry
   │     • cost = calculateCostFromTokens(tokens, pricing)
   │     • promptTokens = tokens.prompt_tokens || tokens.input_tokens
   │     • completionTokens = tokens.completion_tokens || tokens.output_tokens
   │     • entry object fully built
   │
   ├─ 3. group entries by local dateKey into `dayAgg[dk]`
   │     • aggregateEntryToDay() builds byProvider, byModel, byAccount,
   │       byApiKey, byEndpoint counter objects
   │     • byApiKey key = "apiKey|model|provider"
   │     • byEndpoint key = "endpoint|model|provider"
   │     • byModel   key = "model|provider"
   │     • byAccount key = connectionId
   │
   ├─ 4. db.transaction(() => {                          ← single SQLite txn
   │       a. INSERT INTO usageHistory (one row per entry, prepared stmt)
   │       b. INSERT INTO usageDaily  ON CONFLICT(dateKey) DO UPDATE
   │          SET data = excluded.data                   ← FULL JSON REWRITE
   │       c. SELECT _meta.totalRequestsLifetime (current count)
   │          then INSERT OR UPDATE with cur + entries.length
   │     })
   │
   ├─ 5. pushToRing(entry)  for each entry
   │     • recentRing.items (max 50, in-memory LRU)
   │     • read by getActiveRequests() / getUsageStats() for "recentRequests"
   │
   └─ 6. statsEmitter.emit("update")
        • all SSE clients subscribed to /api/usage/stream
        • each fires a full getUsageStats() recompute + push
```

**Total DB writes per request: 3** (one row insert + one row upsert + one counter update).
Per flush batch of N entries: 1 + (#distinct days) + 1.

The `INSERT OR REPLACE` pattern on `usageDaily` is what the task brief describes as
"UPSERT (INSERT OR REPLACE) usageDaily" — this **rewrites the entire JSON blob** for
that date on every batch. That's the second hot path.

---

## 4. `usageDaily` JSON Structure (CRITICAL)

```json
// row 1:  dateKey = "2026-06-03"   data = <below>
{
  "requests": 1842,
  "promptTokens": 9410233,
  "completionTokens": 1124883,
  "cost": 12.47,

  "byProvider": {
    "openai":  { "requests": 902, "promptTokens": 4500000, "completionTokens": 600000, "cost": 7.10 },
    "anthropic": { "requests": 940, "promptTokens": 4910233, "completionTokens": 524883, "cost": 5.37 }
  },

  "byModel": {
    // key format: "model|provider" (legacy) OR with rawModel/provider meta inlined
    "gpt-4|openai": {
      "requests": 902, "promptTokens": 4500000, "completionTokens": 600000, "cost": 7.10,
      "rawModel": "gpt-4", "provider": "openai"
    }
  },

  "byAccount": {
    // key is just connectionId
    "conn-abc-123": {
      "requests": 12, "promptTokens": 50000, "completionTokens": 8000, "cost": 0.21,
      "rawModel": "gpt-4", "provider": "openai"
    }
  },

  "byApiKey": {
    // key format: "apiKey|model|provider"
    "sk-xxxx...|gpt-4|openai": {
      "requests": 142, "promptTokens": 320000, "completionTokens": 41000, "cost": 0.83,
      "rawModel": "gpt-4", "provider": "openai",
      "apiKey": "sk-xxxx..."           ← duplicated into the value
    },
    "local-no-key": { ... }
  },

  "byEndpoint": {
    // key format: "endpoint|model|provider"
    "/v1/chat|gpt-4|openai": {
      "requests": 902, "promptTokens": 4500000, "completionTokens": 600000, "cost": 7.10,
      "endpoint": "/v1/chat", "rawModel": "gpt-4", "provider": "openai"
    }
  }
}
```

### Mutation Pattern

The `INSERT INTO usageDaily ... ON CONFLICT DO UPDATE SET data = excluded.data`
clobbers the entire blob. The previous blob is **never read** during the write path —
increments are computed in memory by `aggregateEntryToDay()` from the queued batch.

This works in SQLite because:
- Single-writer model (WAL, `busy_timeout=5s`)
- `db.transaction` provides serializability
- Reads (getUsageStats) happen on different queries against the same blob

**This pattern is hostile to PostgreSQL.** Concurrent writes from multiple replicas
will race on the `data` column (last-write-wins on the JSON string). It needs a
complete rewrite using `jsonb` + `jsonb_set` or, better, normalized rows.

---

## 5. Write Volume & Hot Path

| Call site | Per request | Per day (rough estimate) |
|---|---|---|
| `saveRequestUsage` (in queue) | 0 DB writes (in-memory push) | — |
| `_flushWriteQueue` | 3 DB writes (1 history row + 1 daily upsert + 1 counter) | — |
| At 100 req/s: 100 queue pushes/s, ~1 flush/s of 50 entries = 1.5k rows/s sustained | — | ~13M rows/day |
| `saveRequestDetail` (if observability on) | 1 DB write per detail | ~8.6M rows/day at 100 req/s |
| `trackPendingRequest` | 0 DB writes (in-memory only) | — |

The **3 writes per request** is the bottleneck. The daily-aggregation rewrite
re-uploads the same `data` blob (growing monotonically through the day) every
flush. By end of day, that blob is ~1MB+ for a busy install.

---

## 6. SSE Integration

File: `src/app/api/usage/stream/route.js`

```text
GET /api/usage/stream           (Server-Sent Events)
   │
   ├─ initial frame:  await getUsageStats()              (full recompute, ~50-300ms)
   │
   ├─ on "update" event  → send full stats + cached frame  (HEAVY: full recompute)
   ├─ on "pending" event → send lightweight frame
   │   (activeRequests + recentRequests from ring buffer)
   │
   └─ keepalive ping every 25s
```

`statsEmitter` is **process-local** (Node `EventEmitter`). This is fine in a
single-process Next.js server, but a **hard requirement** for the Kubernetes
migration: every replica has its own `statsEmitter`. The SSE route only sees
events from its own process. The in-memory `pendingRequests`, `recentRing`,
`connCache`, and `writeQueue` are all process-local too.

### Implications for K8s

- A request landing on replica A increments A's pending counter. Replica B's
  SSE clients never see it.
- The recentRing (last 50 completed requests) is per-replica and arbitrary —
  UI shows whatever happened to be processed on whichever replica they
  happen to be connected to.
- `lastErrorProvider` (sticky error indicator) is per-replica.
- The `pendingTimers` (60s stale-request cleanup) are per-replica.

These are all "best effort" UI features — the underlying truth still lives in
`usageHistory`. The migration plan must accept that the "real-time" view will
degrade, OR replace this with Redis pub/sub.

---

## 7. Read Paths — Query Inventory

These are the read queries that must be preserved (semantics) post-migration:

### 7a. `getUsageStats(period, filter)` — the big one

Two execution paths:

1. **`period in {"7d", "30d", "60d", "all"}` (useDailySummary=true):**
   - `loadDaysInRange()` → `SELECT dateKey, data FROM usageDaily` (with optional
     dateKey >= cutoff for "7d"/"30d"/"60d", full scan for "all")
   - For each day row: parse JSON, fold byProvider/byModel/byAccount/byApiKey/byEndpoint
     into running `stats` object
   - **Overlay pass:** `SELECT timestamp, provider, model, connectionId, apiKey, endpoint
     FROM usageHistory WHERE timestamp >= ?` (and apiKey filter) — used only to set
     `lastUsed` timestamps. This is a full scan of the date range on every stats call!

2. **`period in {"24h", "today"}`:** live scan of `usageHistory` with the cutoff
   filter. Aggregates in JS.

For `filter.apiKey`, the daily-summary path also recomputes totals from
`byApiKey` and clears `byModel`/`byProvider` (because daily aggregations don't
break down by API key at the model/provider level — only at the apiKey key level).

3. **Plus** a "last 10 minutes" bucketed query (always runs):
   `SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory
    WHERE timestamp >= ? AND timestamp <= ?` (10-min window)

4. **Plus** `recentRequests`: `SELECT timestamp, provider, model, tokens, status
   FROM usageHistory ORDER BY id DESC LIMIT 100`

5. **Plus** connection map & node map lookups (3 separate repo imports)

This function is **CPU-heavy on the JS side** and **DB-heavy** (multiple queries,
including a full history scan for lastUsed overlay). The daily-summary path
should ideally run as a SQL aggregate; the live-scan path is a hot loop.

### 7b. `getChartData(period, filter)`

- "today": 24 hourly buckets, scans today's `usageHistory`
- "24h": 24 hourly buckets, scans last 24h of `usageHistory`
- "7d"/"30d"/"60d": reads `usageDaily` rows for the window (one per day)

### 7c. `getUsageHistory(filter)`

Paginates raw `usageHistory` rows. No aggregation.

### 7d. `getRecentLogs(limit)`

`SELECT timestamp, provider, model, connectionId, promptTokens, completionTokens,
status, tokens FROM usageHistory ORDER BY id DESC LIMIT N`. Joins with
connections to get account names.

### 7e. `getRequestDetails(filter)` (requestDetailsRepo)

Paginated read of `requestDetails` with provider/model/connectionId/status/date filters.

### 7f. `getRequestDetailById(id)`

Point lookup by primary key. Single SELECT, returns parsed JSON blob.

---

## 8. Data Lifecycle

### What's never cleaned up

- **`usageHistory`**: every chat completion is a row. Forever. Grows linearly.
  No partitioning, no archival, no TTL.
- **`requestDetails`**: self-prunes to `observabilityMaxRecords` (default 200).
  Bounded — good.
- **`usageDaily`**: one row per calendar day. Grows by ~365 rows/year.
  Trivial.
- **`_meta.totalRequestsLifetime`**: monotonically incremented counter. Bounded.

### No data retention policy

Nothing in code deletes from `usageHistory`. A busy install running 1000 req/s
will write 86M rows/day, 31B rows/year. SQLite handles this poorly past
~100M rows in a single table — the overlay scan and the "lastUsed" overlay
in `getUsageStats` will eventually become IO-bound.

This is **the most important reason** the migration to PostgreSQL is happening:
SQLite can no longer scale to the usage volume 9Router is now seeing.

---

## 9. Concurrency Model (Current)

- **Single Node process.** SQLite with WAL + `busy_timeout=5s` gives serializable
  writes from one process.
- **Multiple Next.js workers** in the same process share `global._pendingRequests`,
  `global._statsEmitter`, etc. (this is intentional — see lines 15-26).
- **Hot reload in dev** resets module state, hence the `global.X` pattern.

### Invariants the tests enforce (`tests/unit/db-concurrent.test.js`)

- 100 parallel `saveRequestUsage` → `totalRequests === 100` (no count loss)
- 200 parallel `saveRequestDetail` → all flushed
- Mixed concurrent usage + details + connections + aliases → atomic

These will **all break** if migrated naively to PostgreSQL. The current
"rewrite the JSON blob" pattern has no analog in PG without explicit
`SELECT ... FOR UPDATE` + `jsonb_set` + `UPDATE`.

---

## 10. PostgreSQL Migration Complexity Assessment

### 10.1 What works essentially as-is

- The **schema** maps cleanly: `usageHistory` → `usage_events` table, same columns.
- The **read queries** (`getUsageHistory`, `getRecentLogs`, "last 10 minutes",
  24h/today aggregations) translate to PG with literal syntax changes.
- `requestDetails` table is a clean 1:1 mapping.
- `_meta` table is portable (just `key TEXT PK, value TEXT`).

### 10.2 What must be rewritten

#### A. `usageDaily` JSON blob — biggest change

Three options, in order of recommendation:

1. **JSONB + atomic update with `jsonb_set`:**
   ```sql
   UPDATE usage_daily
   SET data = jsonb_set(
     jsonb_set(
       jsonb_set(data, '{byProvider,openai}', ...),
       '{byModel,"gpt-4|openai"}', ...
     ),
     '{requests}', (data->>'requests')::int + N
   )
   WHERE date_key = '2026-06-03';
   ```
   Still racy across replicas. Requires `SELECT ... FOR UPDATE` on the row.
   Doesn't actually save much over the SQLite pattern.

2. **Normalized aggregation tables (recommended):**
   ```sql
   CREATE TABLE usage_daily_by_provider (
     date_key DATE NOT NULL,
     provider TEXT NOT NULL,
     requests BIGINT NOT NULL DEFAULT 0,
     prompt_tokens BIGINT NOT NULL DEFAULT 0,
     completion_tokens BIGINT NOT NULL DEFAULT 0,
     cost NUMERIC(12, 6) NOT NULL DEFAULT 0,
     last_used TIMESTAMPTZ,
     PRIMARY KEY (date_key, provider)
   );
   -- same for by_model, by_account, by_api_key, by_endpoint
   ```
   Use `INSERT ... ON CONFLICT (date_key, provider) DO UPDATE SET
   requests = usage_daily_by_provider.requests + EXCLUDED.requests, ...`.
   This is **trivially parallelizable** and **indexable**.

3. **Materialized view / continuous aggregate** (best long-term):
   Let PostgreSQL maintain the rollup via a refresh-on-write trigger
   or a TimescaleDB continuous aggregate.

#### B. Hot-path write volume → use TimescaleDB

`usageHistory` is a textbook time-series dataset. TimescaleDB gives us:

- **Hypertables** with automatic time-based chunking
- **Native compression** for old chunks (10-20x savings)
- **Continuous aggregates** for pre-computed `usageDaily`-equivalent rollups
- **Data retention policies** to drop chunks older than N days

Suggested schema:
```sql
CREATE TABLE usage_events (
  id BIGSERIAL,
  ts TIMESTAMPTZ NOT NULL,
  provider TEXT, model TEXT, connection_id TEXT, api_key TEXT, endpoint TEXT,
  prompt_tokens INT, completion_tokens INT, cost NUMERIC(12, 6),
  status TEXT, tokens JSONB, meta JSONB
);
SELECT create_hypertable('usage_events', 'ts', chunk_time_interval => INTERVAL '1 day');

CREATE MATERIALIZED VIEW usage_daily_by_provider
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 day', ts) AS day,
  provider,
  COUNT(*) AS requests,
  SUM(prompt_tokens) AS prompt_tokens,
  SUM(completion_tokens) AS completion_tokens,
  SUM(cost) AS cost
FROM usage_events
WHERE provider IS NOT NULL
GROUP BY day, provider
WITH NO DATA;
```

The continuous aggregate **replaces `usageDaily` entirely**. The batch flush
becomes a single bulk INSERT into `usage_events`, and Postgres's auto-refresh
keeps the rollup current. Read paths switch to querying the materialized view.

#### C. `requestDetails` self-pruning

Replace the `COUNT(*) > maxRecords` check with:
- Native partitioning by `timestamp` (e.g., monthly partitions, drop oldest)
- OR a TimescaleDB hypertable with `add_retention_policy('request_details', INTERVAL '30 days')`

#### D. `_meta.totalRequestsLifetime` counter

Becomes a row-level lock / atomic increment:
```sql
INSERT INTO _meta(key, value) VALUES('totalRequestsLifetime', $1)
ON CONFLICT (key) DO UPDATE SET value = (CASE
  WHEN _meta.value ~ '^[0-9]+$' THEN (_meta.value::bigint + $2)::text
  ELSE $2::text
END);
```

Or simpler: dedicated `counters` table with `UPDATE counters SET n = n + $1 WHERE name = 'total_requests_lifetime'`.

### 10.3 What must be designed around

- **SSE pub/sub across pods.** `statsEmitter` is in-process. Options:
  1. Accept that "real-time" view is per-replica (current behavior for a
     multi-replica K8s deploy, but explicit now).
  2. Add Redis pub/sub: each pod publishes to a channel, every pod subscribes
     and re-emits on its local `statsEmitter`.
  3. Use Postgres `LISTEN`/`NOTIFY` (triggers on `usage_events` insert).

- **Connection pooling.** Every PG query needs a pool. Add `pg.Pool` with
  sized connection limit (e.g., 20 per pod × 3 pods = 60 max connections).
  SQLite has zero connection overhead.

- **Transactions.** `db.transaction(() => { ... })` is SQLite's BEGIN/COMMIT
  wrapper. PG equivalent: `BEGIN ... COMMIT` per request, or batch the whole
  flush in one `pg.Pool.connect()` + transaction.

- **Async driver change.** `pg` is async at every call. The current SQLite
  adapter is mostly sync (better-sqlite3) with async wrappers. The flush
  pipeline already awaits, so this is fine; the read paths will gain latency.

- **Migration of existing SQLite data.** Need a one-shot exporter that
  streams `usageHistory` → `INSERT INTO pg.usage_events`. For installs with
  billions of rows, this is days of work. Consider parallel COPY imports.

- **Local time vs UTC.** `getLocalDateKey()` uses the Node process's local
  TZ to bucket days. In K8s, pods can have different TZ configs. Must
  pin the process to UTC (or a configured TZ) and migrate `dateKey`
  computation to UTC-based bucketing.

### 10.4 Risk-ranked migration tasks

| # | Risk | Task | Effort |
|---|---|---|---|
| 1 | **HIGH** | Rewrite `_flushWriteQueue` to use normalized `usage_daily_by_*` tables with `INSERT ... ON CONFLICT DO UPDATE` | L |
| 2 | **HIGH** | Add TimescaleDB extension; convert `usageHistory` → hypertable; convert `usageDaily` → continuous aggregate | L |
| 3 | **HIGH** | Add `requestDetails` retention policy (drop oldest partition / chunk) | M |
| 4 | MED | Replace `_meta` counter with atomic `UPDATE ... SET n = n + $1` | S |
| 5 | MED | Add Redis pub/sub (or pg LISTEN/NOTIFY) to replace cross-pod `statsEmitter` | M |
| 6 | MED | Connection pool config + per-pod pool sizing | S |
| 7 | MED | Move all `getLocalDateKey` calls to UTC | S |
| 8 | LOW | Port `getUsageStats` "lastUsed overlay" query to PG syntax; add index `usage_events(api_key, ts)` | S |
| 9 | LOW | Update `requestDetails` self-prune to a partition-drop | S |
| 10 | LOW | `pg` driver adapter implementing same `run/get/all/exec/transaction` interface as current SQLite adapter | M |

### 10.5 What does NOT need to change

- Public API surface (`saveRequestUsage`, `getUsageStats`, etc.) — signature-stable.
- The call sites in `open-sse/handlers/chatCore/*` and `open-sse/utils/usageTracking.js` — they only see the JS API.
- The shape of the returned `stats` object from `getUsageStats` — must remain bit-identical for the dashboard.
- The shape of `getChartData` and `getUsageHistory` responses.
- `trackPendingRequest` semantics — still in-memory per-pod.
- The 1s batch / 50-entry batching window — good defaults, keep them.

---

## 11. Summary — Why This Is The Hardest Migration

1. **3 DB writes per API call, including one that rewrites an unbounded JSON blob.**
   This is the inverse of every best practice for a high-write OLTP system. It's
   only viable today because SQLite WAL serializes everything in one process.

2. **`usageHistory` is append-only and unbounded** — the dataset that most needs
   time-series features (partitioning, compression, retention) gets the least.

3. **The read path includes a full-scan overlay** for `lastUsed` timestamps —
   this is a hidden N×M scan that must be replaced with a window function
   (`DISTINCT ON` or `LAST_VALUE`) in PG.

4. **SSE / in-memory state is process-local.** The migration exposes this fact;
   the fix (Redis or PG NOTIFY) is a non-trivial design decision.

5. **The JSON-blob aggregation pattern is a complete rewrite** — there's no
   drop-in PG equivalent. The cleanest replacement is normalized rollup tables
   populated by a Postgres trigger or TimescaleDB continuous aggregate.

6. **Tests enforce strict invariants** (no count loss under 100 parallel writes)
   that PG must satisfy. The rewrite must preserve the test contract.

7. **Existing data must be migrated.** SQLite → PG COPY of `usageHistory` is
   non-trivial at scale and may require downtime or dual-write window.

The reward for getting this right: a usage-tracking layer that scales to
billions of rows, with sub-second dashboard queries, that survives multi-replica
Kubernetes deploys without locking contention.
