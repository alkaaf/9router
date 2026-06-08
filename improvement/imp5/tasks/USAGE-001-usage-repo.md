---
id: USAGE-001
domain: usage
status: TODO
estimate: 1h
title: UsageHistory GORM Model
---

## Description

Define the `UsageHistory` GORM model matching the SQLite `usageHistory` table. This is the per-request audit log table — the most write-heavy table in the system.

## Input

- Go struct definition with correct GORM tags based on schema.js

## Output

- GORM struct `UsageHistory` mapped to `usageHistory` table
- 12 indexes defined via GORM migrations

## Schema (from schema.js)

```
usageHistory:
  id          INTEGER PRIMARY KEY AUTOINCREMENT
  timestamp   TEXT NOT NULL          → Go: time.Time
  provider    TEXT                   → Go: *string
  model       TEXT                   → Go: *string
  connectionId TEXT                  → Go: *string (FK to providerConnections.id)
  apiKey      TEXT                   → Go: *string
  endpoint    TEXT                   → Go: *string
  promptTokens INTEGER DEFAULT 0     → Go: int
  completionTokens INTEGER DEFAULT 0 → Go: int
  cost        REAL DEFAULT 0         → Go: float64
  status      TEXT                   → Go: *string
  tokens      TEXT                   → Go: string (JSON — prompt_tokens, completion_tokens)
  meta        TEXT                   → Go: string (JSON — additional metadata)
```

## Indexes (12 total — critical for read performance)

```go
idx_uh_ts             ON (timestamp DESC)
idx_uh_id_desc        ON (id DESC)
idx_uh_provider       ON (provider)
idx_uh_model          ON (model)
idx_uh_conn           ON (connectionId)
idx_uh_apiKey         ON (apiKey)
idx_uh_provider_ts    ON (provider, timestamp DESC)
idx_uh_model_ts       ON (model, timestamp DESC)
idx_uh_conn_ts        ON (connectionId, timestamp DESC)
idx_uh_apiKey_ts      ON (apiKey, timestamp DESC)
```

## Logic

1. Define `UsageHistory` struct with GORM `model:` and `column:` tags
2. Use pointer types (*string) for nullable fields: provider, model, connectionId, apiKey, endpoint, status
3. Implement JSON tags for tokens and meta fields to serialize as JSON strings
4. Define indexes using GORM's `Migrator().CreateIndex()` or `table:"-"` with raw SQL in AutoMigrate
5. Support both SQLite and PostgreSQL dialects — PostgreSQL uses TEXT for timestamps, SQLite uses TEXT in ISO8601 format

## Acceptance Criteria

- [ ] UsageHistory struct compiles and maps to usageHistory table
- [ ] All 12 indexes are created on AutoMigrate
- [ ] NULL handling works for optional fields (provider, model, etc.)
- [ ] JSON fields (tokens, meta) serialize/deserialize correctly
- [ ] Timestamp ordering (DESC) works correctly for history queries

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Basic roundtrip | Insert record with all fields populated | All fields retrieved correctly |
| NULL optional fields | Insert record with provider=NULL | provider returned as nil pointer |
| JSON tokens field | tokens = {"prompt_tokens": 100, "completion_tokens": 50} | Parsed back as map[string]int |
| Timestamp DESC order | Query last 10 records ordered by timestamp DESC | Most recent first |
| Index usage | EXPLAIN QUERY PLAN on provider filter | Uses idx_uh_provider index |
