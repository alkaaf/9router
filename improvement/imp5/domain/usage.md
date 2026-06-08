# Usage Domain — Atomic Task Breakdown

**Domain:** `Usage` — usage tracking, statistics, charts, and real-time streaming for the AI routing gateway
**Part of:** 9Router Backend Rewrite (Node.js → Go/Fiber)
**Source files analyzed:**
- `src/app/api/usage/*/route.js` — 12 route files
- `src/lib/db/repos/usageRepo.js` — 990-line core usage logic
- `src/lib/db/repos/requestDetailsRepo.js` — observability/logging
- `src/lib/db/schema.js` — table definitions with 12 indexes

---

## High-Level Architecture

```
Client (Frontend / AI IDE)
       │
       ├─ GET /api/usage/stats          → getUsageStats() → usageDaily + usageHistory
       ├─ GET /api/usage/chart          → getChartData()  → usageHistory time-bucket
       ├─ GET /api/usage/history        → getUsageHistory() → usageHistory paginated
       ├─ GET /api/usage/stream         → SSE → statsEmitter → getActiveRequests()
       ├─ GET /api/usage/providers     → requestDetails + providerNodes
       ├─ GET /api/usage/logs          → getRecentLogs() → usageHistory
       ├─ GET /api/usage/request-details → getRequestDetails() → requestDetails table
       ├─ GET /api/usage/request-logs   → getRecentLogs() (alias)
       ├─ GET /api/usage/[connectionId] → getUsageForProvider() (external API)
       └─ GET /api/usage/per-key/[keyId]/* → filtered by API key

LLM Chat Handler (chatCore, embeddingsCore, etc.)
       │
       ├─ trackPendingRequest()         → in-memory pendingRequests map
       ├─ saveRequestUsage()           → write queue → usageHistory + usageDaily
       ├─ saveRequestDetail()          → requestDetails (observability)
       └─ statsEmitter.emit("update")  → triggers SSE stream

Go Target:
internal/model/usage_history.go
internal/model/usage_daily.go
internal/model/request_detail.go
internal/repository/usage.go
internal/repository/request_details.go
internal/handler/api/usage.go  (stats, chart, history, stream)
internal/handler/api/usage_providers.go
internal/handler/api/usage_per_key.go
internal/handler/api/usage_logs.go
internal/handler/api/request_details.go
internal/handler/api/usage_connection.go
internal/service/usage.go     (trackPending, saveUsage, flush, statsEmitter)
```

---

## Task Index

| ID | Task | Category | Est. Days |
|----|------|----------|-----------|
| USAGE-001 | UsageHistory GORM model | Model | 0.5 |
| USAGE-002 | UsageDaily GORM model | Model | 0.5 |
| USAGE-003 | RequestDetail GORM model | Model | 0.5 |
| USAGE-004 | Usage repository | Repository | 2 |
| USAGE-005 | RequestDetails repository | Repository | 1 |
| USAGE-006 | Daily aggregation logic | Repository | 0.5 |
| USAGE-007 | `/api/usage/stats` handler | Handler | 0.5 |
| USAGE-008 | `/api/usage/chart` handler | Handler | 0.5 |
| USAGE-009 | `/api/usage/history` handler | Handler | 0.5 |
| USAGE-010 | `/api/usage/stream` SSE handler | Handler | 1 |
| USAGE-011 | `/api/usage/providers` handler | Handler | 0.5 |
| USAGE-012 | `/api/usage/logs` + `/api/usage/request-logs` handlers | Handler | 0.5 |
| USAGE-013 | `/api/usage/request-details` handler | Handler | 0.5 |
| USAGE-014 | `/api/usage/[connectionId]` handler | Handler | 1 |
| USAGE-015 | `/api/usage/per-key/[keyId]` handlers | Handler | 1 |
| USAGE-016 | Usage tracking service (write queue, pending, emitter) | Service | 1.5 |
| USAGE-017 | LLM handler integration (trackPending, saveUsage) | Integration | 1 |
| USAGE-018 | Graceful shutdown flush | Integration | 0.5 |
| USAGE-019 | Unit tests for repository + service | Testing | 2 |
| USAGE-020 | Integration tests for all handlers | Testing | 1.5 |
| **Total** | | | **~17 days** |

---

## Detailed Tasks

---

### USAGE-001 — UsageHistory GORM Model

**Description:**
Define the `UsageHistory` GORM model matching the SQLite `usageHistory` table. This is the per-request audit log table — the most write-heavy table in the system.

**Input/Output Contract:**
- Input: Go struct definition with correct GORM tags
- Output: GORM struct `UsageHistory` that maps to `usageHistory` table

**Schema (from `schema.js`):**
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

**Indexes (12 total — critical for read performance):**
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

**Test Strategy:**
- Create in-memory SQLite DB with AutoMigrate
- Insert a record and SELECT it back — verify all fields round-trip
- Verify GORM `Where` queries use the correct index (EXPLAIN QUERY PLAN)
- Test NULL handling for optional fields (provider, model, etc.)
- Verify JSON fields (`tokens`, `meta`) parse correctly

**Dependencies:** `USAGE-004` (repo needs the model), `USAGE-017` (write path)

---

### USAGE-002 — UsageDaily GORM Model

**Description:**
Define the `UsageDaily` GORM model for daily aggregate snapshots. Used by `getUsageStats()` as the primary read path for periods >= 7d (avoids scanning millions of history rows).

**Input/Output Contract:**
- Input: Go struct definition
- Output: GORM struct `UsageDaily`

**Schema (from `schema.js`):**
```
usageDaily:
  dateKey TEXT PRIMARY KEY   → Go: string  (format: "YYYY-MM-DD")
  data    TEXT NOT NULL      → Go: string  (JSON blob — see below)
```

**JSON data shape (stored in `data` column):**
```json
{
  "requests": 1234,
  "promptTokens": 50000,
  "completionTokens": 30000,
  "cost": 1.50,
  "byProvider": { "openai": { "requests": 500, "promptTokens": 20000, ... } },
  "byModel": { "gpt-4|openai": { "requests": 200, "rawModel": "gpt-4", "provider": "openai", ... } },
  "byAccount": { "conn-uuid": { "requests": 100, "rawModel": "claude-3", ... } },
  "byApiKey": { "sk-xxx|gpt-4|openai": { "requests": 50, "keyName": "...", ... } },
  "byEndpoint": { "/v1/chat/completions|gpt-4|openai": { ... } }
}
```

**Test Strategy:**
- Create in-memory DB, AutoMigrate, upsert a daily record
- Deserialize `data` JSON and verify aggregation fields
- Test ON CONFLICT update behavior (daily upsert should merge, not replace)
- Verify `dateKey` uniqueness

**Dependencies:** `USAGE-004`, `USAGE-006` (aggregation logic writes here)

---

### USAGE-003 — RequestDetail GORM Model

**Description:**
Define the `RequestDetail` GORM model for observability/request tracing. Stores full request/response bodies with sensitive field sanitization.

**Input/Output Contract:**
- Input: Go struct definition
- Output: GORM struct `RequestDetail`

**Schema (from `schema.js`):**
```
requestDetails:
  id          TEXT PRIMARY KEY   → Go: string (UUID-like: timestamp-random-model)
  timestamp   TEXT NOT NULL      → Go: time.Time
  provider    TEXT               → Go: *string
  model       TEXT               → Go: *string
  connectionId TEXT              → Go: *string
  status      TEXT               → Go: *string
  data        TEXT NOT NULL      → Go: string (JSON blob — see below)
```

**Indexes:**
```
idx_rd_ts        ON (timestamp DESC)
idx_rd_provider  ON (provider)
idx_rd_model     ON (model)
idx_rd_conn      ON (connectionId)
```

**JSON data shape (stored in `data` column):**
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

**Sensitive header keys sanitized:** authorization, x-api-key, cookie, token, api-key

**Test Strategy:**
- Insert with sanitized headers — verify sensitive fields are absent
- Test truncation for oversized JSON bodies (>5KB default)
- Test pagination queries with offset/limit
- Verify auto-cleanup deletes oldest records when > maxRecords (200 default)

**Dependencies:** `USAGE-005`

---

### USAGE-004 — Usage Repository

**Description:**
Implement the main `UsageRepository` with all query methods. This is the largest single task in the Usage domain — the Node.js `usageRepo.js` is ~500 lines of complex aggregation logic. Must be broken down carefully.

**Key methods to implement:**

#### 4a. `SaveRequestUsage(entry UsageHistoryEntry)` — Write path
```
- Input: entry with {timestamp, provider, model, connectionId, apiKey, endpoint, tokens, status}
- Writes to: usageHistory (individual row)
- Writes to: usageDaily (upsert daily aggregation JSON)
- Increments: _meta.totalRequestsLifetime
- For PostgreSQL: also writes to usageDailyByProvider/Model/ApiKey/Account/Endpoint rollup tables
- Uses: write queue with batch flush (WRITE_BATCH_MAX=50, WRITE_BATCH_MS=1000)
- Cost calculation: look up pricing by (provider, model) → calculateCostFromTokens(tokens, pricing)
```
**Note:** This must be called by chat/embeddings handlers after every LLM request.

#### 4b. `GetUsageHistory(filter HistoryFilter)` — Paginated history
```
- Input: filter {provider, model, connectionId, apiKeyId, apiKey, status, startDate, endDate, limit, offset}
- Output: []HistoryEntry — ordered by timestamp DESC, id DESC
- LIMIT/OFFSET pagination (not cursor-based)
- Reads from: usageHistory
```

#### 4c. `GetUsageStats(period string, filter StatsFilter)` — Aggregate stats
```
- Input: period in {today, 24h, 7d, 30d, 60d, all}, filter {apiKey}
- Output: UsageStats struct:
  {
    totalRequests, totalPromptTokens, totalCompletionTokens, totalCost,
    byProvider: { [provider]: { requests, promptTokens, completionTokens, cost } },
    byModel: { [modelKey]: { requests, cost, rawModel, provider, lastUsed } },
    byAccount: { [accountKey]: { requests, cost, connectionId, accountName } },
    byApiKey: { [apiKeyKey]: { requests, cost, keyName } },
    byEndpoint: { [endpointKey]: { requests, cost, endpoint } },
    last10Minutes: [{ requests, promptTokens, completionTokens, cost }],  // 10 buckets
    activeRequests: [{ model, provider, account, count }],
    recentRequests: [{ timestamp, model, provider, promptTokens, completionTokens, status }],
    errorProvider: string,
    pending: { byModel: {}, byAccount: {} }
  }
- Logic:
  - period == "today" | "24h": read live from usageHistory (query cutoff timestamp)
  - period == "7d" | "30d" | "60d" | "all": read from usageDaily aggregates (much faster)
  - activeRequests + recentRequests: in-memory state (pendingRequests map + recent ring buffer)
  - last10Minutes: 10-bucket minute-by-minute aggregation from usageHistory
  - For PostgreSQL: use GORM scopes with date_trunc() for time buckets
```

#### 4d. `GetChartData(period string, filter ChartFilter)` — Time-series for charts
```
- Input: period in {today, 24h, 7d, 30d, 60d}, filter {apiKey}
- Output: []ChartBucket:
  {
    today:  24 buckets × hourly  (label: "HH:MM")
    24h:    24 buckets × hourly  (label: "HH:MM")
    7d:     7  buckets × daily   (label: "Jun 3")
    30d:    30 buckets × daily   (label: "Jun 3")
    60d:    60 buckets × daily   (label: "Jun 3")
  }
  Each bucket: { label: string, tokens: int, cost: float64 }
- SQLite: use strftime() for bucket grouping
- PostgreSQL: use date_trunc() for bucket grouping
- Read from: usageHistory (SUM promptTokens+completionTokens, SUM cost) GROUP BY bucket
```

#### 4e. `TrackPendingRequest(model, provider, connectionId string, started bool, error bool)` — In-memory
```
- Updates: global pendingRequests map { byModel: {}, byAccount: {} }
- 60-second timeout per request (stale pending cleanup)
- Emits: statsEmitter.SendPending()
- Logs: [PENDING] START/END with provider, model
```

#### 4f. `GetActiveRequests()` — Current live state
```
- Reads: pendingRequests map + recentRing (last 50 entries from usageHistory)
- Deduplicates recentRequests by (model|provider|promptTokens|completionTokens|minute)
- Returns: { activeRequests, recentRequests, errorProvider }
```

#### 4g. `FlushWriteQueue()` — Batch write
```
- Drains writeQueue up to WRITE_BATCH_MAX (50) entries
- Computes cost for each entry via pricing lookup
- Writes to usageHistory in single transaction
- Updates usageDaily aggregates
- Calls pushToRing() for each entry (updates recent ring)
- Emits: statsEmitter.SendUpdate()
- Re-schedules if queue not empty
```

**Test Strategy:**
- Unit: Test each method with in-memory SQLite GORM
- Seed realistic data, verify aggregation correctness
- Test edge cases: empty results, single entry, full batch flush
- Test period-specific logic (today vs 7d reads different paths)
- Benchmark: ensure GetUsageStats(7d) uses daily aggregates, not history scan

**Dependencies:** `USAGE-001`, `USAGE-002`, `USAGE-016`

---

### USAGE-005 — RequestDetails Repository

**Description:**
Implement `RequestDetailsRepository` for observability logging.

**Key methods:**

#### 5a. `SaveRequestDetail(detail RequestDetailInput)` — Write path
```
- Input: {id, provider, model, connectionId, timestamp, status, latency, tokens,
          request, providerRequest, providerResponse, response}
- Sanitizes sensitive headers before storing
- Truncates JSON if > maxJsonSize (default 5KB)
- Uses write buffer + periodic flush (FLUSH_INTERVAL_MS=5000, BATCH_SIZE=20)
- Auto-deletes oldest records when total > maxRecords (default 200)
```

#### 5b. `GetRequestDetails(filter RequestDetailsFilter)` — Paginated
```
- Input: filter {provider, model, connectionId, status, startDate, endDate, page, pageSize}
- pageSize clamped 1-100
- Output: { details: []ParsedDetail, pagination: { page, pageSize, totalItems, totalPages, hasNext, hasPrev } }
- Reads from: requestDetails, ordered by timestamp DESC
```

#### 5c. `GetRequestDetailById(id string)` — Single record
```
- Input: detail ID
- Output: full JSON data blob or null
```

**Test Strategy:**
- Test header sanitization: verify Authorization/Cookie headers are stripped
- Test JSON truncation: oversized body is truncated with `_truncated` marker
- Test pagination: verify totalItems, hasNext, hasPrev
- Test auto-cleanup: insert 300 records, verify only 200 remain

**Dependencies:** `USAGE-003`

---

### USAGE-006 — Daily Aggregation Logic

**Description:**
Extract and port the `aggregateEntryToDay()` helper from Node.js. This is called inside the write flush path to build the `usageDaily` JSON blob.

**Logic:**
```go
func aggregateEntryToDay(day *DayAggregate, entry *UsageHistoryEntry) {
    day.requests++
    day.promptTokens += entry.PromptTokens
    day.completionTokens += entry.CompletionTokens
    day.cost += entry.Cost

    // byProvider
    if entry.Provider != "" {
        addToCounter(day.ByProvider, entry.Provider, vals)
    }

    // byModel: key = "model|provider" or just "model"
    modelKey := entry.Provider ? `${entry.Model}|${entry.Provider}` : entry.Model
    addToCounter(day.ByModel, modelKey, vals, meta={rawModel, provider})

    // byAccount: keyed by connectionId
    if entry.ConnectionId != "" {
        addToCounter(day.ByAccount, entry.ConnectionId, vals, meta={rawModel, provider})
    }

    // byApiKey: key = "apiKey|model|provider" or "local-no-key"
    apiKeyVal := entry.ApiKey ?? "local-no-key"
    akKey := `${apiKeyVal}|${entry.Model}|${entry.Provider}`
    addToCounter(day.ByApiKey, akKey, vals, meta={rawModel, provider, apiKey})

    // byEndpoint: key = "endpoint|model|provider"
    epKey := `${entry.Endpoint}|${entry.Model}|${entry.Provider}`
    addToCounter(day.ByEndpoint, epKey, vals, meta={endpoint, rawModel, provider})
}
```

**Test Strategy:**
- Feed 10 entries across 2 providers, 3 models, 2 API keys
- Verify aggregated counts match manual calculation
- Test that `usageDaily` upsert correctly MERGES (not replaces) existing data

**Dependencies:** `USAGE-004` (called by flush)

---

### USAGE-007 — `/api/usage/stats` Handler

**Description:**
Implement the `GET /api/usage/stats` route handler.

**Input/Output Contract:**
```
GET /api/usage/stats?period={today|24h|7d|30d|60d|all}&apiKey={optional}

Query params:
  period: string (optional, default "7d", validated against VALID_PERIODS set)
  apiKey: string (optional, filter by API key)

Response 200:
{
  totalRequests: number,
  totalPromptTokens: number,
  totalCompletionTokens: number,
  totalCost: number,
  byProvider: { [key]: { requests, promptTokens, completionTokens, cost } },
  byModel: { [key]: { requests, cost, rawModel, provider, lastUsed } },
  byAccount: { [key]: { requests, cost, connectionId, accountName, rawModel, provider } },
  byApiKey: { [key]: { requests, cost, keyName, rawModel, provider, apiKey } },
  byEndpoint: { [key]: { requests, cost, endpoint, rawModel, provider } },
  last10Minutes: [{ requests, promptTokens, completionTokens, cost }],
  activeRequests: [{ model, provider, account, count }],
  recentRequests: [{ timestamp, model, provider, promptTokens, completionTokens, status }],
  errorProvider: string,
  pending: { byModel: { [key]: count }, byAccount: { [connId]: { [modelKey]: count } } }
}

Error responses:
  400: { "error": "Invalid period" }
  500: { "error": "Failed to fetch usage stats" }
```

**Test Strategy:**
- `httptest.NewRequest` with valid/invalid periods
- Compare response shape against Node.js baseline
- Test apiKey filter: same data, filtered result
- Mock DB with seed data for deterministic tests

**Dependencies:** `USAGE-004`

---

### USAGE-008 — `/api/usage/chart` Handler

**Description:**
Implement `GET /api/usage/chart`.

**Input/Output Contract:**
```
GET /api/usage/chart?period={today|24h|7d|30d|60d}&apiKey={optional}

Query params:
  period: string (optional, default "7d", validated — note: "all" is NOT valid here)
  apiKey: string (optional)

Response 200:
[
  { "label": "Jun 3", "tokens": 12345, "cost": 0.50 },  // 7d/30d/60d: daily
  { "label": "14:00", "tokens": 500, "cost": 0.02 },    // today/24h: hourly
  ...
]

Error responses:
  400: { "error": "Invalid period" }
  500: { "error": "Failed to fetch chart data" }
```

**Test Strategy:**
- Test all 5 periods — verify bucket count and label format
- Verify SQLite vs PostgreSQL date formatting (strftime vs date_trunc)
- Test apiKey filter

**Dependencies:** `USAGE-004`

---

### USAGE-009 — `/api/usage/history` Handler

**Description:**
Implement `GET /api/usage/history`.

**Input/Output Contract:**
```
GET /api/usage/history?provider=&model=&connectionId=&apiKeyId=&apiKey=&status=&startDate=&endDate=&limit=&offset=

Query params (all optional):
  provider, model, connectionId, apiKeyId, apiKey, status: filter conditions
  startDate, endDate: ISO timestamp range
  limit: number (optional, default unlimited)
  offset: number (optional, default 0)

Response 200:
[
  {
    timestamp: "2026-06-04T10:00:00.000Z",
    provider: "openai",
    model: "gpt-4",
    connectionId: "uuid",
    apiKey: "sk-...",
    endpoint: "/v1/chat/completions",
    cost: 0.03,
    status: "ok",
    tokens: { "prompt_tokens": 100, "completion_tokens": 50 }
  },
  ...
]

Error responses:
  400: { "error": "Invalid period" } (not used here — no period param)
  500: { "error": "Failed to fetch usage history" }
```

**Note:** `history/route.js` validates period against `VALID_PERIODS` but never passes it to `getUsageHistory()`. This is a dead code path in the current implementation — the period parameter is accepted but ignored.

**Test Strategy:**
- Test all filter combinations
- Test LIMIT/OFFSET pagination
- Test date range filtering
- Verify tokens JSON deserialization

**Dependencies:** `USAGE-004`

---

### USAGE-010 — `/api/usage/stream` SSE Handler

**Description:**
Implement real-time SSE streaming for live usage updates.

**Input/Output Contract:**
```
GET /api/usage/stream

Response: text/event-stream (no status code on success — stream starts immediately)

Event sequence:
  1. Full stats push:
     data: { ...full usageStats object... }\n\n

  2. Subsequent lightweight pushes (on pending changes):
     data: { ...stats with activeRequests, recentRequests, errorProvider ... }\n\n

  3. Ping every 25 seconds:
     data: : ping\n\n


  4. [DONE] marker on close (implicit via stream end)

Headers:
  Content-Type: text/event-stream
  Cache-Control: no-cache
  Connection: keep-alive
```

**Behavior (two-tier push pattern):**
- `send()`: Heavy — full stats recalc + lightweight activeRequests push
  - Emits on: statsEmitter "update" event
  - Pushes lightweight update immediately, then recalculates full stats
- `sendPending()`: Light — only activeRequests + recentRequests + errorProvider
  - Emits on: statsEmitter "pending" event
  - Only runs if cachedStats already exists (skipped on first call)
- `keepalive`: Ping comment every 25 seconds via setInterval

**Context lifecycle:**
```
start:  → start SSE stream, subscribe to emitter events
cancel: → unsubscribe emitter, clear interval, set closed=true
```

**Goroutine management (Fiber):**
```go
c.Set("Content-Type", "text/event-stream")
c.Set("Cache-Control", "no-cache")
c.Set("Connection", "keep-alive")
c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            // flush pending data
            fmt.Fprintf(w, "data: %s\n\n", jsonData)
            w.Flush()
        }
    }
})
```

**Test Strategy:**
- Integration test: SSE client connects, receives first event within 2s
- Integration test: `statsEmitter.SendUpdate()` triggers new event
- Integration test: client disconnect triggers cancel/cleanup
- Integration test: ping arrives every 25s
- Verify goroutine cleanup on client disconnect (no goroutine leak)

**Dependencies:** `USAGE-004`, `USAGE-016` (statsEmitter)

---

### USAGE-011 — `/api/usage/providers` Handler

**Description:**
Implement `GET /api/usage/providers` — returns unique provider list from request details.

**Input/Output Contract:**
```
GET /api/usage/providers

Response 200:
{
  "providers": [
    { "id": "openai", "name": "OpenAI" },
    { "id": "anthropic", "name": "Anthropic" }
  ]
}

Error responses:
  500: { "error": "Failed to fetch providers" }
```

**Logic:**
1. Query `requestDetails` table, get unique provider values
2. Join with `providerNodes` (by node.id = providerId) for display names
3. Fall back to AI_PROVIDERS constant for display name lookup
4. Sort alphabetically by id

**Test Strategy:**
- Mock requestDetails with 3 distinct providers
- Verify provider names resolved from nodeMap or AI_PROVIDERS constant
- Verify alphabetical sort

**Dependencies:** `USAGE-005` (requestDetails repo), providerNodes repo

---

### USAGE-012 — `/api/usage/logs` + `/api/usage/request-logs` Handlers

**Description:**
Both routes return the same data: recent log lines from `usageHistory`.

**Input/Output Contract:**
```
GET /api/usage/logs
GET /api/usage/request-logs

Response 200:
[
  "04-06-2026 14:30:00 | gpt-4 | OPENAI | account-xyz | 100 | 50 | ok",
  "04-06-2026 14:29:55 | claude-3 | ANTHROPIC | account-abc | 200 | 150 | ok",
  ...
]
```
Format: `DD-MM-YYYY HH:MM:SS | model | PROVIDER | account | promptTokens | completionTokens | status`

**Logic:**
1. Query last 200 rows from `usageHistory` (ORDER BY timestamp DESC)
2. Map connectionId → account name via providerConnections
3. Format each line as: `ts | model | provider | account | sent | received | status`

**Test Strategy:**
- Verify format string matches expected pattern
- Verify limit (200) is respected
- Verify empty result returns empty array (not error)

**Dependencies:** `USAGE-004`, `USAGE-002`

---

### USAGE-013 — `/api/usage/request-details` Handler

**Description:**
Implement `GET /api/usage/request-details` — paginated request detail logs.

**Input/Output Contract:**
```
GET /api/usage/request-details?page=1&pageSize=20&provider=&model=&connectionId=&status=&startDate=&endDate=

Query params:
  page: int (default 1, min 1)
  pageSize: int (default 20, range 1-100)
  provider, model, connectionId, status, startDate, endDate: filters

Response 200:
{
  "details": [
    {
      "latency": { "total": 1234, "provider": 1000 },
      "tokens": { "prompt_tokens": 100 },
      "request": { ... },
      ...
    }
  ],
  "pagination": {
    "page": 1,
    "pageSize": 20,
    "totalItems": 150,
    "totalPages": 8,
    "hasNext": true,
    "hasPrev": false
  }
}

Error responses:
  400: { "error": "Page must be >= 1" }
  400: { "error": "PageSize must be between 1 and 100" }
  500: { "error": "Failed to fetch request details" }
```

**Test Strategy:**
- Test page/pageSize validation (400 responses)
- Test pagination math (totalPages, hasNext, hasPrev)
- Test all filter combinations
- Test empty results

**Dependencies:** `USAGE-005`

---

### USAGE-014 — `/api/usage/[connectionId]` Handler

**Description:**
Implement `GET /api/usage/[connectionId]` — fetches usage from the external provider API (e.g., OpenAI usage dashboard, Anthropic API usage).

**Input/Output Contract:**
```
GET /api/usage/[connectionId]  (path param: connectionId)

Response 200: Raw usage data from provider API (provider-specific format)
  - OpenAI: { "total_usage": [...], "object": "list" }
  - Anthropic: { "api_key": { "id": "...", "usage": {...} } }

Error responses:
  401: { "error": "Credential refresh failed: ..." }
  404: { "error": "Connection not found" }
  200: { "message": "Usage not available for this connection" } (apikey non-eligible)
  500: { "error": "..." }
```

**Logic:**
1. Load provider connection by ID
2. Check auth type: OAuth or whitelisted API-key provider (glm, minimax, etc.)
3. For OAuth: call `refreshAndUpdateCredentials()` (refreshes access token if expired)
4. Call `getUsageForProvider(connection, proxyOptions)` — makes HTTP request to provider's usage API
5. If auth-expired pattern detected: force-refresh and retry once
6. Return usage data or error

**OAuth refresh flow (from Node.js `refreshAndUpdateCredentials`):**
```
1. Build credentials from connection { accessToken, refreshToken, expiresAt, providerSpecificData }
2. executor.needsRefresh(credentials) → bool
3. executor.refreshCredentials(credentials, proxyOptions) → newCredentials
4. updateProviderConnection(id, updateData) → persisted
5. getUsageForProvider(updatedConnection, proxyOptions) → usage API response
```

**Auth-expired detection patterns:** "expired", "authentication", "unauthorized", "401", "re-authorize"

**Note:** This endpoint calls external provider APIs — it requires the executor infrastructure (see Phase 3 executors). Implement as a stub returning `{"error": "not implemented"}` in Phase 2, wire up in Phase 3.

**Test Strategy:**
- Mock providerConnection from DB
- Mock executor refresh and usage API
- Test OAuth flow: expired → refresh → retry → success
- Test API-key flow: no refresh needed
- Test 404: connection not found
- Test 401: refresh failure

**Dependencies:** `USAGE-015` (provider connection repo), executor system (Phase 3)

---

### USAGE-015 — `/api/usage/per-key/[keyId]` Handlers

**Description:**
Implement three nested routes under `/api/usage/per-key/[keyId]/`:

#### 15a. `GET /api/usage/per-key/[keyId]` — Full per-key stats
```
Query: period={today|24h|7d|30d|60d|all} (default "7d")

Response 200:
{
  keyId: "uuid",
  keyName: "My API Key",
  keyMasked: "sk-xxxx...xxxx",
  period: "7d",
  stats: { totalRequests, totalPromptTokens, totalCompletionTokens, totalCost },
  byModel: [{ name, requests, promptTokens, completionTokens, cost }],
  chartData: [{ label, tokens, cost }],
  history: [{ timestamp, provider, model, cost, tokens }]
}
```

#### 15b. `GET /api/usage/per-key/[keyId]/history` — Per-key paginated history
```
Query: limit (default 50, max 200), offset (default 0)

Response 200:
{
  keyId: "uuid",
  history: [{ timestamp, provider, model, cost, tokens }],
  limit: 50,
  offset: 0
}
```

#### 15c. `GET /api/usage/per-key/[keyId]/chart` — Per-key chart
```
Query: period={today|24h|7d|30d|60d} (default "7d")

Response 200:
{
  keyId: "uuid",
  period: "7d",
  chartData: [{ label, tokens, cost }]
}
```

**Shared logic:**
1. Load API key by `keyId` via `getApiKeyById(keyId)`
2. If not found: 404
3. All sub-queries pass `filter: { apiKey: key.key }` (NOT keyId — the actual API key string)
4. `getUsageStats()` with `filter.apiKey` recalculates totals from `byApiKey` only

**Note:** The stats struct returned is a transformed subset of `getUsageStats()` — the `byModel` array is derived from `stats.byModel` filtered to entries that match the API key.

**Test Strategy:**
- Test 404 when keyId not found
- Test that filter uses `key.key` (the actual key string), not `keyId`
- Test all 3 sub-routes with seeded data
- Verify period validation (15c rejects "all")

**Dependencies:** `USAGE-004`, API key repository (apiKeysRepo)

---

### USAGE-016 — Usage Tracking Service (Write Queue, Pending, Emitter)

**Description:**
Implement the in-memory state management service for usage tracking. This replaces the global state in Node.js `usageRepo.js`.

**Components:**

#### 16a. StatsEmitter (EventEmitter pattern)
```go
type UsageStatsEmitter struct {
    mu         sync.RWMutex
    listeners  map[chan<- UsageEvent]struct{}
}

func (e *UsageStatsEmitter) OnUpdate(handler func())
func (e *UsageStatsEmitter) SendUpdate()
func (e *UsageStatsEmitter) SendPending()
func (e *UsageStatsEmitter) Off(handler func())
```

#### 16b. PendingRequests state
```go
type PendingTracker struct {
    mu        sync.RWMutex
    byModel   map[string]int    // "gpt-4 (openai)": count
    byAccount map[string]map[string]int  // connectionId → { "gpt-4 (openai)": count }
    timers    map[string]*time.Timer  // "${connId}|${modelKey}" → timer
}
```
- `PENDING_TIMEOUT_MS = 60 * 1000` (stale pending cleanup)
- Thread-safe with sync.RWMutex

#### 16c. RecentRing buffer
```go
type RecentRing struct {
    mu     sync.RWMutex
    items  []UsageHistoryEntry  // max 50 items, LRU-style
}
```
- Stores last 50 usage entries for recentRequests display
- Initialized from DB on first access

#### 16d. WriteQueue + Scheduler
```go
type WriteQueue struct {
    mu       sync.Mutex
    entries  []UsageHistoryEntry
    busy     bool
    timer    *time.Timer
}
const WRITE_BATCH_MAX = 50
const WRITE_BATCH_MS  = 1000  // 1 second
```
- Non-blocking enqueue: adds to queue, schedules flush
- Flush scheduler: if queue empty, arm 1s timer; if queue >= 50, drain immediately
- Background drain: single goroutine drains queue in batch

#### 16e. ConnectionMap cache
```go
type ConnectionMapCache struct {
    mu        sync.RWMutex
    map_      map[string]string  // connectionId → name/email
    ts        int64
}
const CONN_CACHE_TTL_MS = 30 * 1000
```

**Test Strategy:**
- Concurrent test: multiple goroutines call TrackPending simultaneously
- Write queue test: push 100 entries, verify exactly 2 flushes (50+50)
- Timer test: verify pending timeout fires after 60s
- Emitter test: subscribe, emit, verify handler called
- Goroutine leak test: create/destroy SSE connections, verify no orphan goroutines

**Dependencies:** `USAGE-004` (repository uses this service)

---

### USAGE-017 — LLM Handler Integration

**Description:**
Integrate usage tracking into the chat/embeddings handlers. Every LLM request must:
1. Call `TrackPendingRequest(model, provider, connectionId, started=true)` on request start
2. Call `TrackPendingRequest(..., started=false)` on request completion (or error)
3. Call `SaveRequestUsage(entry)` with tokens/cost after response

**Call sites to instrument (from `src/lib/db/repos/usageRepo.js` imports):**
- `src/sse/handlers/chat.js` — chat completions (SSE streaming)
- `src/sse/handlers/embeddings.js` — embeddings
- `src/sse/handlers/fetch.js` — web fetch (Perplexity)
- `src/sse/handlers/search.js` — web search

**Entry fields:**
```go
type UsageEntry struct {
    Timestamp    time.Time  // defaults to now
    Provider     string
    Model        string
    ConnectionId string
    ApiKey       string
    Endpoint     string  // "/v1/chat/completions", "/v1/embeddings", etc.
    Tokens       map[string]int  // {"prompt_tokens": 100, "completion_tokens": 50}
    Status       string  // "ok", "error", etc.
}
```

**Streaming considerations:**
- For SSE responses: track pending on stream start, save usage on stream completion
- Tokens available from the final chunk metadata or accumulated from deltas
- If stream errors mid-way: `SaveRequestUsage()` still called with whatever tokens were received

**Pricing lookup:**
```
cost = calculateCostFromTokens(tokens, pricing)
  → pricing = getPricingForModel(provider, model)
  → getPricingForModel: look up in settings.pricing (JSON) or use defaults
```

**Test Strategy:**
- Integration test: call chat handler, verify usageHistory row inserted
- Integration test: call embeddings handler, verify tokens stored
- Streaming test: partial stream error, verify usage saved with partial tokens
- Error test: provider returns 401, verify status="error" and errorProvider set

**Dependencies:** `USAGE-016`, `USAGE-004`, chat/embeddings handlers (Phase 3)

---

### USAGE-018 — Graceful Shutdown Flush

**Description:**
Ensure pending usage writes are flushed before process shutdown.

**Implementation:**
```go
// In main.go or server graceful shutdown:
func setupGracefulFlush(repo *UsageRepository, service *UsageService) {
    // Flush on SIGINT, SIGTERM, beforeExit
    signals := []os.Signal{os.SIGINT, os.SIGTERM}
    signal.Notify(shutdownChan, signals...)

    go func() {
        <-shutdownChan
        log.Info("Shutting down, flushing usage writes...")
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        repo.FlushAll(ctx)  // drain entire queue, return when done
        cancel()            // signal server to stop
    }()
}
```

**Behavior:**
- Flush ALL remaining queue entries (not just batch of 50)
- Wait up to 5 seconds, then give up
- Log flush completion or timeout

**Test Strategy:**
- Send SIGTERM to server process, verify all queue entries flushed to DB
- Verify DB contains all expected records after graceful shutdown

**Dependencies:** `USAGE-016`

---

### USAGE-019 — Unit Tests for Repository + Service

**Test coverage targets:**

#### Repository tests (USAGE-004, USAGE-005, USAGE-006)
- `TestUsageRepository_SaveRequestUsage`: insert single entry, verify usageHistory row
- `TestUsageRepository_SaveRequestUsage_Batch`: insert 100 entries, verify 2 flushes, verify all written
- `TestUsageRepository_GetUsageHistory`: filter by provider/model/date, paginate, verify order
- `TestUsageRepository_GetUsageStats_Today`: period="today", verify live aggregation
- `TestUsageRepository_GetUsageStats_7d`: period="7d", verify daily aggregate path
- `TestUsageRepository_GetUsageStats_ByApiKey`: filter by apiKey, verify totals recalculated
- `TestUsageRepository_GetUsageStats_Last10Minutes`: verify 10 buckets
- `TestUsageRepository_GetChartData_Today`: verify 24 hourly buckets
- `TestUsageRepository_GetChartData_7d`: verify 7 daily buckets
- `TestUsageRepository_GetChartData_Postgres`: use `:memory:` SQLite default, document Postgres-specific date_trunc test
- `TestUsageRepository_TrackPendingRequest`: start/end/error tracking, verify map state
- `TestUsageRepository_GetActiveRequests`: pending + recent ring
- `TestUsageRepository_DailyAggregation`: aggregate multiple entries, verify merged counts
- `TestRequestDetailsRepository_SaveRequestDetail`: verify sanitization, truncation
- `TestRequestDetailsRepository_GetRequestDetails`: pagination, filters
- `TestRequestDetailsRepository_AutoCleanup`: insert 300, verify 200 remain

#### Service tests (USAGE-016)
- `TestUsageService_WriteQueue_Batch`: 100 entries → 2 flushes
- `TestUsageService_WriteQueue_Timer`: entries sit for WRITE_BATCH_MS, then flush
- `TestUsageService_PendingTracker_Concurrent`: 100 goroutines simultaneous updates
- `TestUsageService_PendingTracker_Timeout`: pending not resolved within 60s → cleared
- `TestUsageService_StatsEmitter`: subscribe/emit/unsubscribe
- `TestUsageService_ConnectionMapCache`: verify cache ttl and invalidation
- `TestUsageService_RecentRing`: verify max 50 items, LRU behavior

**Dependencies:** All model, repository, and service tasks

---

### USAGE-020 — Integration Tests for Usage Handlers

**Test coverage targets:**

| Handler | Test Cases |
|---------|------------|
| `USAGE-007` stats | valid period, invalid period (400), with apiKey filter, 500 error |
| `USAGE-008` chart | all 5 periods, apiKey filter, invalid period (400) |
| `USAGE-009` history | filters (provider, model, date range), pagination, 500 error |
| `USAGE-010` stream | connects, receives first event, receives update on emit, ping, disconnect cleanup |
| `USAGE-011` providers | returns sorted unique providers with names |
| `USAGE-012` logs | returns formatted log lines, respects limit |
| `USAGE-013` request-details | pagination validation (400s), filters, 500 error |
| `USAGE-014` [connectionId] | 404, OAuth flow mock, apikey non-eligible, 401 refresh fail |
| `USAGE-015` per-key | 404, all 3 sub-routes, period validation |

**Test infrastructure:**
```go
func setupTestUsageApp() *fiber.App {
    app := fiber.New()
    db, _ := gorm.Open(sqlite.Open(":memory:"))
    db.AutoMigrate(&model.UsageHistory{}, &model.UsageDaily{}, &model.RequestDetail{})
    repo := repository.NewUsageRepository(db, service.NewUsageService())
    handler.NewUsageHandler(app, repo, service)
    return app
}
```

**Dependencies:** All handler tasks (`USAGE-007` through `USAGE-015`)

---

## Cross-Cutting Concerns

### PostgreSQL-Specific Rollup Tables (Postgres Only)

The Node.js `usageRepo.js` writes to additional rollup tables when `db.driver === "postgres"`:
- `usageDailyByProvider` (date, provider, requestCount, inputTokens, outputTokens, totalTokens, cost)
- `usageDailyByModel` (date, model, ...)
- `usageDailyByApiKey` (date, apiKeyId, ...)
- `usageDailyByAccount` (date, accountId, ...)
- `usageDailyByEndpoint` (date, endpoint, ...)

These use `ON CONFLICT(date, dimCol) DO UPDATE SET ...` for incremental updates. Implement in GORM using `clause.OnConflict`.

For SQLite, these tables do not exist — all aggregation happens via the JSON blob in `usageDaily.data`.

### Pricing Integration

Cost is calculated from `calculateCostFromTokens(tokens, pricing)`:
1. Look up pricing: `getPricingForModel(provider, model)` — reads from settings JSON
2. Compute: `inputTokens * pricing.inputCost + outputTokens * pricing.outputCost`
3. Deduplicated: all unique (provider, model) pairs in a batch are fetched once (parallel Promise.all in Node.js)

In Go, implement as a `map[string]*Pricing` lookup in the flush goroutine, fetching unique pricing entries once per batch.

### In-Memory State vs. DB

The following state is IN-MEMORY (not persisted) and must be recreated on startup:
- `pendingRequests` map — cleared on restart
- `recentRing` buffer — reloaded from last 50 `usageHistory` rows on first access
- `lastErrorProvider` — cleared on restart
- `connectionMapCache` — TTL-cached, re-fetched as needed
- `statsEmitter` listeners — cleared on restart

### Thread Safety

All in-memory state (`pendingRequests`, `recentRing`, `writeQueue`) must use `sync.RWMutex`. The SSE handler reads pending state on every push event; the write goroutine updates it concurrently.

### Observability Toggle

`saveRequestDetail()` is gated by `getObservabilityConfig().enabled` — reads from settings table with 5s cache TTL. Default: `true` if `OBSERVABILITY_ENABLED !== "false"` env var.

---

## File Map (Go Target)

```
internal/
├── model/
│   ├── usage_history.go       # USAGE-001
│   ├── usage_daily.go         # USAGE-002
│   └── request_detail.go       # USAGE-003
├── repository/
│   ├── usage.go                # USAGE-004, USAGE-006
│   └── request_details.go      # USAGE-005
├── service/
│   └── usage.go               # USAGE-016
└── handler/
    └── api/
        ├── usage_stats.go      # USAGE-007
        ├── usage_chart.go      # USAGE-008
        ├── usage_history.go    # USAGE-009
        ├── usage_stream.go     # USAGE-010
        ├── usage_providers.go   # USAGE-011
        ├── usage_logs.go       # USAGE-012
        ├── usage_request_details.go  # USAGE-013
        ├── usage_connection.go  # USAGE-014
        └── usage_per_key.go    # USAGE-015

tests/
├── repository/usage_test.go    # USAGE-019
├── service/usage_test.go       # USAGE-019
└── handler/usage_test.go        # USAGE-020
```
