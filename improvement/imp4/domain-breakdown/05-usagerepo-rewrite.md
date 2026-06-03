# Domain 5: usageRepo.js Rewrite

## Overview

Rewrite `src/lib/db/repos/usageRepo.js` (914 LOC) — the single most complex and
critical file in the migration. This repo handles 3 database writes per API request,
including a JSON-blob rewrite pattern that is hostile to concurrent PostgreSQL writers.

The rewrite has two parts:
- **Write path**: Replace `INSERT OR REPLACE INTO usageDaily ... SET data = excluded.data`
  (full JSON blob clobber) with normalized `usage_daily_by_*` rollup tables using
  `INSERT ... ON CONFLICT (date_key, dimension) DO UPDATE SET ...` for atomic
  incremental aggregation. Replace per-row prepared INSERT with multi-row INSERT.
- **Read path**: Replace JS-side JSON aggregation with SQL aggregates over the
  normalized tables, or keep JS aggregation over JSONB blobs (JSONB is faster to
  parse and can be partially queried with `jsonb_path_query`).

## Scope

- **In scope**: _flushWriteQueue rewrite, getUsageStats SQL aggregation, getChartData
  optimization, usageDaily normalization, _meta counter atomic update
- **Out of scope**: TimescaleDB hypertable (future), SSE cross-pod pub/sub (future)

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `src/lib/db/repos/usageRepo.js` | MODIFY | ~+50 / ~-80 (restructure) |
| `src/lib/db/helpers/metaStore.js` | MODIFY | +10 (atomic counter) |

## Dependencies

Domain 4 must be complete (all other repos migrated, jsonCol.js guards in place).

## Sub-Tasks

1. **Replace `_meta` counter with atomic SQL**
   - Description: Replace the read-modify-write pattern (`SELECT value FROM _meta
     WHERE key = 'totalRequestsLifetime'` → compute new value → UPSERT) with a
     single atomic statement:
     `UPDATE _meta SET value = (value::bigint + $1)::text WHERE key = 'totalRequestsLifetime'
     RETURNING value`. This eliminates the race condition where two concurrent flushes
     read the same baseline and one's update is lost.
   - Acceptance: 100 parallel `saveRequestUsage` calls → `totalRequestsLifetime`
     equals 100 (existing test invariant preserved). No lost counter updates under
     concurrent flushes.
   - Risk: Medium (the counter is load-bearing for billing/limits)

2. **Rewrite `_flushWriteQueue` write path — multi-row INSERT**
   - Description: Replace the per-row `insertStmt.run(...)` loop with a single
     multi-row INSERT: `INSERT INTO usageHistory(timestamp, provider, model, ...)
     VALUES ($1,$2,...), ($11,$12,...), ...`. Build the VALUES clause dynamically
     from the entries array. Same for `usageDaily` UPSERT — use multi-row VALUES.
   - Acceptance: Batch of 50 entries produces exactly 50 rows in usageHistory.
     Performance: single round-trip instead of 50.
   - Risk: Medium (SQL construction must handle varying entry counts correctly)

3. **Rewrite `usageDaily` — normalized rollup tables (CRITICAL)**
   - Description: Create 5 normalized tables to replace the monolithic JSON blob:
     - `usage_daily_by_provider` (date_key DATE, provider TEXT, requests BIGINT,
       promptTokens BIGINT, completionTokens BIGINT, cost NUMERIC(12,6), lastUsed
       TIMESTAMPTZ, PK: (date_key, provider))
     - `usage_daily_by_model` (PK: date_key, model, provider)
     - `usage_daily_by_account` (PK: date_key, connectionId)
     - `usage_daily_by_api_key` (PK: date_key, apiKey)
     - `usage_daily_by_endpoint` (PK: date_key, endpoint, model, provider)
     
     Each table uses `INSERT ... ON CONFLICT (date_key, dimension) DO UPDATE SET
     requests = table.requests + EXCLUDED.requests, ...`. The `aggregateEntryToDay()`
     helper now writes to these tables instead of building a nested JSON object.
   - Acceptance: After flush, querying each table returns correct incremental totals.
     Concurrent flushes from different pods don't lose updates (ON CONFLICT handles
     this). The full JSON blob `data` can still be reconstructed for backward compat
     via a `rebuildDailyBlob()` helper if needed.
   - Risk: High (this is the core architectural change; 5 new tables, all read paths
     affected)

4. **Rewrite `getUsageStats` — SQL aggregation**
   - Description: For the daily-summary path (7d/30d/60d), replace the JS-side
     `loadDaysInRange` + parse-JSON + fold-into-maps pattern with SQL:
     ```sql
     SELECT provider, SUM(requests) as totalRequests, SUM(promptTokens) as totalPromptTokens,
            SUM(completionTokens) as totalCompletionTokens, SUM(cost) as totalCost
     FROM usage_daily_by_provider WHERE date_key >= $1 GROUP BY provider
     ```
     Run 5 queries (one per rollup table) and merge results in JS. For the 24h/today
     path, keep the live `usageHistory` scan but optimize the `lastUsed` overlay:
     replace the full scan with `SELECT DISTINCT ON (apiKey) apiKey, timestamp,
     provider, model, connectionId, endpoint FROM usageHistory WHERE timestamp >= $1
     ORDER BY apiKey, timestamp DESC`.
   - Acceptance: `getUsageStats("7d")` returns identical shape as SQLite path:
     `{ totalRequests, totalPromptTokens, totalCompletionTokens, totalCost,
     byProvider, byModel, byApiKey, byAccount, byEndpoint, lastUsed, ... }`.
     Performance: 5 SQL queries instead of loading + parsing N JSON blobs.
   - Risk: High (this is the primary dashboard query; must be bit-identical output)

5. **Rewrite `getChartData` — SQL time-bucketing**
   - Description: For 7d/30d/60d, query `usageDaily` (or rollup tables) for daily
     buckets. For today/24h, use PostgreSQL `date_trunc('hour', timestamp)` to bucket
     in SQL instead of JS:
     ```sql
     SELECT date_trunc('hour', timestamp) as hour,
            SUM(promptTokens) as promptTokens, SUM(completionTokens) as completionTokens,
            SUM(cost) as cost
     FROM usageHistory WHERE timestamp >= $1 GROUP BY hour ORDER BY hour
     ```
   - Acceptance: Chart data returns identical `{ labels, values }` shape. Hourly
     buckets correct for today/24h. Daily buckets correct for 7d+.
   - Risk: Medium

6. **Optimize `getUsageHistory` and `getRecentLogs`**
   - Description: These are simple SELECT queries — the only change is `$1, $2, ...`
     placeholders (handled by adapter's `_convertParams`). Add `NULLS LAST` to
     `ORDER BY` for consistent null-sorting.
   - Acceptance: Paginated history and logs return identical data. Performance
     unchanged or better (PostgreSQL B-tree indexes).
   - Risk: Low

7. **Preserve in-memory state (ring buffer, pending requests)**
   - Description: `recentRing` (50-entry LRU), `pendingRequests` (in-flight counter),
     `statsEmitter` (EventEmitter) all remain in-memory per-pod. The write queue
     (`global._usageWriteQueue`) stays in-memory but the flush target is now
     PostgreSQL instead of SQLite. This is acceptable: pod churn means some queued
     entries may be lost on SIGKILL, but the underlying truth (usageHistory in PG)
     is durable.
   - Acceptance: `getActiveRequests()` reads from ring + pending as before.
     SSE events still fire locally.
   - Risk: Low (documented trade-off, not a correctness issue)

8. **Integration tests for usageRepo**
   - Description: Write integration tests that verify: (a) batch flush writes correct
     rows to normalized tables, (b) concurrent flushes don't lose updates, (c)
     getUsageStats returns bit-identical output, (d) getChartData returns correct
     buckets, (e) shutdown flush preserves all queued entries.
   - Acceptance: All usageRepo integration tests pass with PostgreSQL adapter.
     The existing unit tests (`tests/unit/db-concurrent.test.js`) invariant
     (100 parallel writes = 100 count) is preserved.
   - Risk: Medium (this is the hardest test surface)

## Effort Estimate

5-10 days (highest effort domain; requires careful design review)

## Risk Level

High — the JSON-blob-to-normalized-tables rewrite changes the core data model.
Concurrent write correctness is load-bearing. Output shape of getUsageStats must
be bit-identical for the dashboard.

## Testing Requirements

- Unit tests: Yes — individual helpers (aggregateEntryToDay, buildMultiRowInsert,
  atomic counter update)
- Integration tests: Yes — full flush pipeline, concurrent flushes, all read paths
- Manual testing: Make 100 API calls, verify usage stats, chart data, history, logs
  all show correct data. Compare SQLite vs PostgreSQL output for 100 requests.
