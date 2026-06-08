---
id: USAGE-004
domain: usage
status: TODO
estimate: 2h
title: Usage Repository
---

## Description

Implement the main `UsageRepository` with all query methods. This is the largest single task in the Usage domain — the Node.js `usageRepo.js` is ~500 lines of complex aggregation logic.

## Input

- HistoryFilter, StatsFilter, ChartFilter structs
- UsageHistoryEntry, UsageStats structs
- Pricing lookup from settings

## Output

- UsageRepository struct with all CRUD methods
- SQLite and PostgreSQL compatible queries

## Key methods to implement

### 4a. SaveRequestUsage(entry UsageHistoryEntry)

```
- Input: entry with {timestamp, provider, model, connectionId, apiKey, endpoint, tokens, status}
- Writes to: usageHistory (individual row)
- Writes to: usageDaily (upsert daily aggregation JSON)
- Increments: _meta.totalRequestsLifetime
- For PostgreSQL: also writes to usageDailyByProvider/Model/ApiKey/Account/Endpoint rollup tables
- Uses: write queue with batch flush (WRITE_BATCH_MAX=50, WRITE_BATCH_MS=1000)
- Cost calculation: look up pricing by (provider, model) → calculateCostFromTokens(tokens, pricing)
```

### 4b. GetUsageHistory(filter HistoryFilter)

```
- Input: filter {provider, model, connectionId, apiKeyId, apiKey, status, startDate, endDate, limit, offset}
- Output: []HistoryEntry — ordered by timestamp DESC, id DESC
- LIMIT/OFFSET pagination (not cursor-based)
- Reads from: usageHistory
```

### 4c. GetUsageStats(period string, filter StatsFilter)

```
- Input: period in {today, 24h, 7d, 30d, 60d, all}, filter {apiKey}
- Output: UsageStats struct with totals, byProvider, byModel, byAccount, byApiKey, byEndpoint, last10Minutes, activeRequests, recentRequests, errorProvider, pending
- Logic:
  - period == "today" | "24h": read live from usageHistory (query cutoff timestamp)
  - period == "7d" | "30d" | "60d" | "all": read from usageDaily aggregates (much faster)
  - activeRequests + recentRequests: in-memory state (pendingRequests map + recent ring buffer)
  - last10Minutes: 10-bucket minute-by-minute aggregation from usageHistory
  - For PostgreSQL: use GORM scopes with date_trunc() for time buckets
```

### 4d. GetChartData(period string, filter ChartFilter)

```
- Input: period in {today, 24h, 7d, 30d, 60d}, filter {apiKey}
- Output: []ChartBucket with label, tokens, cost
  - today:  24 buckets × hourly  (label: "HH:MM")
  - 24h:    24 buckets × hourly  (label: "HH:MM")
  - 7d:     7  buckets × daily   (label: "Jun 3")
  - 30d:    30 buckets × daily   (label: "Jun 3")
  - 60d:    60 buckets × daily   (label: "Jun 3")
- SQLite: use strftime() for bucket grouping
- PostgreSQL: use date_trunc() for bucket grouping
- Read from: usageHistory (SUM promptTokens+completionTokens, SUM cost) GROUP BY bucket
```

### 4e. TrackPendingRequest(model, provider, connectionId string, started bool, error bool)

```
- Updates: global pendingRequests map { byModel: {}, byAccount: {} }
- 60-second timeout per request (stale pending cleanup)
- Emits: statsEmitter.SendPending()
- Logs: [PENDING] START/END with provider, model
```

### 4f. GetActiveRequests()

```
- Reads: pendingRequests map + recentRing (last 50 entries from usageHistory)
- Deduplicates recentRequests by (model|provider|promptTokens|completionTokens|minute)
- Returns: { activeRequests, recentRequests, errorProvider }
```

### 4g. FlushWriteQueue()

```
- Drains writeQueue up to WRITE_BATCH_MAX (50) entries
- Computes cost for each entry via pricing lookup
- Writes to usageHistory in single transaction
- Updates usageDaily aggregates
- Calls pushToRing() for each entry (updates recent ring)
- Emits: statsEmitter.SendUpdate()
- Re-schedules if queue not empty
```

## Logic

1. Initialize repository with GORM DB, UsageService, PricingService
2. Implement dialect-aware queries: check `db.Dialect().GetName()` for sqlite vs postgres
3. For PostgreSQL: use date_trunc() for bucket grouping and time filters
4. For SQLite: use strftime() for bucket grouping and time filters
5. Batch pricing lookups: collect all unique (provider, model) pairs in a flush batch, look up once
6. Use `sync.RWMutex` for thread-safe access to pendingRequests and recentRing
7. Implement exponential backoff on flush errors (max 3 retries)

## Acceptance Criteria

- [ ] SaveRequestUsage writes to usageHistory and updates usageDaily atomically
- [ ] GetUsageStats reads from daily aggregates for 7d/30d/60d periods
- [ ] GetChartData produces correct bucket counts (24 for hourly, 7/30/60 for daily)
- [ ] TrackPendingRequest updates in-memory state and emits events
- [ ] FlushWriteQueue batches writes efficiently (max 50 per transaction)
- [ ] PostgreSQL dialect uses date_trunc() instead of strftime()
- [ ] Thread-safe concurrent access to pendingRequests and recentRing

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| SaveRequestUsage single | One entry | usageHistory row + daily upserted |
| SaveRequestUsage batch | 50 entries | 1 transaction flush |
| GetUsageStats today | period="today" | Live aggregation from usageHistory |
| GetUsageStats 7d | period="7d" | Daily aggregate path (fast) |
| GetUsageStats byApiKey | filter by apiKey | Totals recalculated from byApiKey |
| GetChartData today | period="today" | 24 hourly buckets |
| GetChartData 7d | period="7d" | 7 daily buckets |
| TrackPendingRequest start | started=true | pendingRequests map incremented |
| TrackPendingRequest end | started=false | pendingRequests map decremented |
| FlushWriteQueue partial | 25 entries (below batch max) | Waits for timer or next entry |
| FlushWriteQueue full | 50 entries (at batch max) | Immediate flush |
| Empty result | GetUsageStats with no data | Zero values, empty maps |
