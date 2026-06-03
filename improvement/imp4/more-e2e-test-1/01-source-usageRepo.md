# usageRepo.js — Source Code Analysis

## What `flush()` does

`_flushWriteQueue()` (lines 225–362) is the core flush implementation. It:

1. **Drains the write queue**: Takes up to `WRITE_BATCH_MAX` (50) entries from the in-memory `writeQueue` array.
2. **Resolves pricing**: Deduplicates by `(provider, model)` key, then calls `getPricingForModel()` in parallel via `Promise.all()` to compute cost from token counts.
3. **Builds entries**: Converts each raw queue item into a normalized `entry` object with fields: timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens (JSON-stringified), meta (JSON-stringified `{}`).
4. **Aggregates daily rollups**: Calls `aggregateEntryToDay()` to build `dayAgg` keyed by local date (YYYY-MM-DD), accumulating byProvider, byModel, byAccount, byApiKey, byEndpoint sub-maps.
5. **Writes in a transaction**:
   - **usageHistory**: Single-row INSERT for 1 entry, multi-row INSERT for >1 entries. Columns: timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta.
   - **Rollup tables (Postgres only)**: For each entry, UPSERTs into `usageDailyByProvider`, `usageDailyByModel`, `usageDailyByApiKey`, `usageDailyByAccount`, `usageDailyByEndpoint` using `ON CONFLICT(date, dimCol) DO UPDATE SET ...`.
   - **usageDaily**: UPSERTs the aggregated daily JSON blob.
   - **_meta**: Increments `totalRequestsLifetime`.
6. **Pushes to ring buffer** and emits `"update"` event.

## SQL generated for PG vs SQLite

### usageHistory INSERT

**Single entry** (SQL identical for both backends, uses `?` placeholders):
```sql
INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
```

**Multi-row** (same SQL pattern, just more value tuples):
```sql
INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?), ...
```

The PG adapter's `convertPlaceholders()` translates `?` → `$1, $2, ...`.

### Rollup tables (Postgres only — skipped on SQLite)

```sql
INSERT INTO usageDailyByProvider(date, provider, requestCount, inputTokens, outputTokens, totalTokens, cost)
VALUES (?, ?, 1, ?, ?, ?, ?)
ON CONFLICT(date, provider) DO UPDATE SET
  requestCount = usageDailyByProvider.requestCount + 1,
  inputTokens = usageDailyByProvider.inputTokens + EXCLUDED.inputTokens,
  outputTokens = usageDailyByProvider.outputTokens + EXCLUDED.outputTokens,
  totalTokens = usageDailyByProvider.totalTokens + EXCLUDED.totalTokens,
  cost = usageDailyByProvider.cost + EXCLUDED.cost,
  updatedAt = NOW()
```

Same pattern repeated for: `usageDailyByModel`, `usageDailyByApiKey`, `usageDailyByAccount`, `usageDailyByEndpoint`.

### usageDaily UPSERT
```sql
INSERT INTO usageDaily(dateKey, data) VALUES(?, ?) ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data
```

### _meta increment
```sql
UPDATE _meta SET value = value + ? WHERE key = ?
```
(called via `incrementMetaSync`)

## Key type observations

- `tokens` and `meta` are passed as `stringifyJson()` output (a JSON string) to `db.run()`.
- On SQLite: the columns are `TEXT`, so the JSON string is stored as-is. On read, `parseJson()` decodes it.
- On Postgres: the columns are `JSONB`. The adapter header says "we do NOT call JSON.stringify / JSON.parse here" — but `usageRepo.js` does call `stringifyJson()` before passing to `db.run()`. This means Postgres receives a JSON string for a JSONB column, which pg driver will store as a JSON string value inside JSONB (i.e., double-encoded). **This is a likely bug.**
- `cost` is passed as a JS number. On SQLite it maps to `REAL`. On PG it maps to `NUMERIC(12,6)`.
