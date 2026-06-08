---
id: USAGE-003
domain: usage
status: TODO
estimate: 1h
title: RequestDetail GORM Model
---

## Description

Define the `RequestDetail` GORM model for observability/request tracing. Stores full request/response bodies with sensitive field sanitization.

## Input

- Go struct definition
- JSON data shape for the `data` column
- List of sensitive header keys to sanitize

## Output

- GORM struct `RequestDetail` mapped to `requestDetails` table
- 4 indexes for efficient querying

## Schema (from schema.js)

```
requestDetails:
  id          TEXT PRIMARY KEY   → Go: string (UUID-like: timestamp-random-model)
  timestamp   TEXT NOT NULL      → Go: time.Time
  provider    TEXT               → Go: *string
  model       TEXT               → Go: *string
  connectionId TEXT              → Go: *string
  status      TEXT               → Go: *string
  data        TEXT NOT NULL      → Go: string (JSON blob)
```

## Indexes

```
idx_rd_ts        ON (timestamp DESC)
idx_rd_provider  ON (provider)
idx_rd_model     ON (model)
idx_rd_conn      ON (connectionId)
```

## JSON data shape (stored in `data` column)

```json
{
  "latency": { "total": 1234, "provider": 1000, "parsing": 234 },
  "tokens": { "prompt_tokens": 100, "completion_tokens": 50 },
  "request": { "method": "POST", "url": "...", "headers": { "sanitized": true } },
  "providerRequest": { ... },
  "providerResponse": { ... },
  "response": { ... }
}
```

## Sensitive header keys sanitized

- authorization
- x-api-key
- cookie
- token
- api-key

## Logic

1. Define `RequestDetail` struct with string ID (not auto-increment — uses UUID-like format)
2. Define `RequestDetailData` struct for the JSON data blob
3. Implement `SanitizeHeaders()` function that removes sensitive keys before storage
4. Implement `TruncateData()` function for oversized JSON bodies (>maxJsonSize, default 5KB)
5. Create indexes on AutoMigrate
6. ID generation: use format `"{timestamp}-{random6chars}-{model}"` or UUID

## Acceptance Criteria

- [ ] RequestDetail struct compiles and maps to requestDetails table
- [ ] All 4 indexes created on AutoMigrate
- [ ] Sensitive headers (authorization, cookie, x-api-key, etc.) are stripped
- [ ] JSON data truncated when exceeding maxJsonSize (5KB)
- [ ] ID format is deterministic (timestamp-random-model pattern)

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Basic insert | Full RequestDetail with all data fields | All fields retrieved correctly |
| Sanitize auth header | data.request.headers.authorization = "Bearer sk-xxx" | Field removed from stored data |
| Sanitize multiple | Multiple sensitive headers present | All removed, others preserved |
| JSON truncation | data body > 5KB | Truncated with marker, < 5KB stored |
| Timestamp ordering | Query ordered by timestamp DESC | Most recent first |
| Provider filter | Query by provider index | Uses idx_rd_provider |
