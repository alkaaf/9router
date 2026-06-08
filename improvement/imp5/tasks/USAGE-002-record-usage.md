---
id: USAGE-002
domain: usage
status: TODO
estimate: 1h
title: UsageDaily GORM Model
---

## Description

Define the `UsageDaily` GORM model for daily aggregate snapshots. Used by `getUsageStats()` as the primary read path for periods >= 7d (avoids scanning millions of history rows).

## Input

- Go struct definition
- JSON data shape for the `data` column

## Output

- GORM struct `UsageDaily` mapped to `usageDaily` table
- dateKey as primary key (format: "YYYY-MM-DD")

## Schema (from schema.js)

```
usageDaily:
  dateKey TEXT PRIMARY KEY   → Go: string  (format: "YYYY-MM-DD")
  data    TEXT NOT NULL      → Go: string  (JSON blob)
```

## JSON data shape (stored in `data` column)

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

## Logic

1. Define `UsageDaily` struct with dateKey as primary key
2. data field stored as JSON string, parsed into a typed struct or map
3. Define helper types for the JSON data: `DailyData`, `ByProviderEntry`, `ByModelEntry`, `ByAccountEntry`, `ByApiKeyEntry`, `ByEndpointEntry`
4. Upsert behavior: INSERT ON CONFLICT (dateKey) DO UPDATE merges data, not replaces
5. Implement `GetDailyData()` method to deserialize JSON data from storage
6. Implement `MergeEntry()` method to aggregate a new UsageHistory entry into existing daily data

## Acceptance Criteria

- [ ] UsageDaily struct compiles and maps to usageDaily table
- [ ] dateKey uniqueness enforced (PRIMARY KEY)
- [ ] JSON data serializes/deserializes correctly
- [ ] Upsert merges data rather than replacing existing aggregates
- [ ] DailyData helper types correctly represent all aggregation dimensions

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Upsert first record | dateKey="2026-06-04", data for 100 requests | Record created with 100 requests |
| Upsert merge | Existing record with 100 requests + new entry with 10 | Record updated to 110 requests |
| byProvider merge | Two entries from same provider "openai" | Provider counts aggregated, not replaced |
| byApiKey merge | Two entries with same apiKey\|model\|provider key | Key-level counts merged correctly |
| JSON roundtrip | DailyData with all dimensions | All fields preserved after serialize/deserialize |
