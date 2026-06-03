# schema.postgres.js — Source Code Analysis

## Type mapping strategy (SQLite → Postgres)

| SQLite type | Postgres type | Notes |
|---|---|---|
| `TEXT` | `TEXT` | Direct mapping |
| `INTEGER` | `INTEGER` or `BIGINT` | BIGINT for rollup counters |
| `REAL` | `NUMERIC(12,6)` | For cost/price precision |
| `TEXT` (JSON blob) | `JSONB` | Auto-serialization by pg driver |
| `INTEGER DEFAULT 1` (boolean flag) | `BOOLEAN DEFAULT TRUE` | Real boolean type |
| `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` | Auto-increment |
| `TEXT` (timestamp string) | `TIMESTAMPTZ` | Timezone-aware timestamp |
| `TEXT` (date string) | `DATE` | For date-only columns |

## usageHistory table
```sql
CREATE TABLE IF NOT EXISTS usageHistory (
  id               BIGSERIAL PRIMARY KEY,
  timestamp        TIMESTAMPTZ NOT NULL,
  provider         TEXT,
  model            TEXT,
  connectionId     TEXT,
  apiKey           TEXT,
  endpoint         TEXT,
  promptTokens     INTEGER DEFAULT 0,
  completionTokens INTEGER DEFAULT 0,
  cost             NUMERIC(12,6) DEFAULT 0,
  status           TEXT,
  tokens           JSONB,
  meta             JSONB
);
```
- `id`: BIGSERIAL (auto-incrementing bigint) — SQLite uses `INTEGER PRIMARY KEY AUTOINCREMENT`
- `tokens` and `meta`: JSONB (not TEXT) — **critical difference**
- `cost`: NUMERIC(12,6) (not REAL) — **precision difference**

## Rollup tables (all share the same structure pattern)

```sql
-- usageDailyByProvider
date         DATE NOT NULL,
provider     TEXT NOT NULL,
requestCount BIGINT NOT NULL DEFAULT 0,
inputTokens  BIGINT NOT NULL DEFAULT 0,
outputTokens BIGINT NOT NULL DEFAULT 0,
totalTokens  BIGINT NOT NULL DEFAULT 0,
cost         NUMERIC(12,6) NOT NULL DEFAULT 0,
updatedAt    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
PRIMARY KEY (date, provider)
```

Same structure for `usageDailyByModel`, `usageDailyByApiKey`, `usageDailyByAccount`, `usageDailyByEndpoint` — only the dimension column name changes:
- `usageDailyByModel`: `model TEXT NOT NULL`
- `usageDailyByApiKey`: `apiKeyId TEXT NOT NULL`
- `usageDailyByAccount`: `accountId TEXT NOT NULL`
- `usageDailyByEndpoint`: `endpoint TEXT NOT NULL`

All use `DATE` for the date column (not TIMESTAMPTZ), `BIGINT` for counters, `NUMERIC(12,6)` for cost, and `TIMESTAMPTZ DEFAULT NOW()` for `updatedAt`.

## Key differences from SQLite schema
- SQLite has NO rollup tables at all (they're Postgres-only).
- SQLite `kv.value` is `TEXT`; Postgres `kv.value` is `JSONB`.
- SQLite `apiKeys.isActive` is `INTEGER DEFAULT 1`; Postgres uses `BOOLEAN DEFAULT TRUE`.
- SQLite `settings.data` is `TEXT NOT NULL`; Postgres uses `JSONB NOT NULL`.
- Postgres has no `usageDaily` table in the new schema — wait, it does: `buildUsageDailyTable()` creates it with `dateKey DATE PRIMARY KEY, data JSONB NOT NULL`.
