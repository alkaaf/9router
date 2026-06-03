# schema.js (SQLite) vs schema.postgres.js — Comparison Analysis

## Type mappings applied

| Column | SQLite (schema.js) | Postgres (schema.postgres.js) | Impact |
|---|---|---|---|
| usageHistory.id | `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` | Different auto-inc mechanism; both work |
| usageHistory.timestamp | `TEXT NOT NULL` | `TIMESTAMPTZ NOT NULL` | PG enforces timezone; SQLite stores as string |
| usageHistory.cost | `REAL DEFAULT 0` | `NUMERIC(12,6) DEFAULT 0` | PG has fixed precision; REAL is float64 |
| usageHistory.tokens | `TEXT` | `JSONB` | **Critical**: PG auto-parses JSONB; SQLite stores string |
| usageHistory.meta | `TEXT` | `JSONB` | **Critical**: Same as tokens |
| apiKeys.isActive | `INTEGER DEFAULT 1` | `BOOLEAN DEFAULT TRUE` | **Type mismatch**: SQLite stores 0/1, PG stores true/false |
| settings.data | `TEXT NOT NULL` | `JSONB NOT NULL` | PG supports structured queries on data |
| providerConnections.data | `TEXT NOT NULL` | `JSONB NOT NULL` | Same |
| kv.value | `TEXT NOT NULL` | `JSONB NOT NULL` | **Critical for kvStore behavior** |
| combos.models | `TEXT NOT NULL` | `JSONB NOT NULL` | Same pattern |
| requestDetails.data | `TEXT NOT NULL` | `JSONB NOT NULL` | Same pattern |

## Column name differences

No column name differences between the two schemas for the tables covered in tests. Both use identical column names for:
- usageHistory: id, timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta
- usageDailyByProvider: date, provider, requestCount, inputTokens, outputTokens, totalTokens, cost, updatedAt
- kv: scope, key, value
- apiKeys: id, key, name, machineId, isActive, createdAt

## Structural differences

1. **Rollup tables**: SQLite has NONE. Postgres has 5 normalized rollup tables (`usageDailyByProvider`, `usageDailyByModel`, `usageDailyByApiKey`, `usageDailyByAccount`, `usageDailyByEndpoint`). These are created and populated only when `db.driver === "postgres"` (usageRepo.js line 307).

2. **Indexes**: Both have similar index patterns. The PG schema includes all the same index names as SQLite.

3. **usageDaily table**: Both have it. SQLite: `dateKey TEXT PRIMARY KEY, data TEXT NOT NULL`. Postgres: `dateKey DATE PRIMARY KEY, data JSONB NOT NULL`.

## Critical implications for testing

1. **JSONB auto-parsing**: When `usageRepo.js` stores `tokens` and `meta` using `stringifyJson()`, SQLite stores a JSON string. Postgres stores that JSON string inside a JSONB column, which means reading it back returns a string (not an object) — the value is double-encoded. The kvStore avoids this by NOT calling `stringifyJson` on the PG path, but usageRepo.js always calls `stringifyJson` regardless of backend.

2. **Boolean type**: SQLite `isActive` uses INTEGER (0/1). Postgres uses BOOLEAN (true/false). Any code that does `row.isActive === 1` will fail on Postgres.

3. **NUMERIC vs REAL**: Cost values stored as `NUMERIC(12,6)` in PG may come back as strings from the pg driver in some configurations, though the adapter coerces `rowCount` to Number. The actual column values (like `cost`) are returned as-is by pg's query() — typically as strings for NUMERIC types unless `parseInt`/`parseFloat` is applied.
