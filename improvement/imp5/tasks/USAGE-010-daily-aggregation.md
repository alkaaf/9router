---
id: USAGE-010
domain: usage
status: TODO
estimate: 1h
title: Daily Aggregation Logic
---

## Description

Extract and port the `aggregateEntryToDay()` helper from Node.js. This is called inside the write flush path to build the `usageDaily` JSON blob.

## Input

- Existing DayAggregate for a specific date
- New UsageHistoryEntry to aggregate

## Output

- Updated DayAggregate with merged entry data
- Incremented counters across all aggregation dimensions

## DayAggregate struct

```go
type DayAggregate struct {
    Date              string
    Requests          int
    PromptTokens      int
    CompletionTokens  int
    Cost              float64
    ByProvider        map[string]ProviderAgg
    ByModel           map[string]ModelAgg
    ByAccount         map[string]AccountAgg
    ByApiKey          map[string]ApiKeyAgg
    ByEndpoint        map[string]EndpointAgg
}
```

## aggregateEntryToDay logic

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

## Aggregation dimension types

### ProviderAgg
```go
type ProviderAgg struct {
    Requests         int
    PromptTokens     int
    CompletionTokens int
    Cost             float64
}
```

### ModelAgg
```go
type ModelAgg struct {
    Requests         int
    PromptTokens     int
    CompletionTokens int
    Cost             float64
    RawModel         string
    Provider         string
}
```

### AccountAgg
```go
type AccountAgg struct {
    Requests         int
    PromptTokens     int
    CompletionTokens int
    Cost             float64
    RawModel         string
    Provider         string
}
```

### ApiKeyAgg
```go
type ApiKeyAgg struct {
    Requests         int
    PromptTokens     int
    CompletionTokens int
    Cost             float64
    RawModel         string
    Provider         string
    ApiKey           string
}
```

### EndpointAgg
```go
type EndpointAgg struct {
    Requests         int
    PromptTokens     int
    CompletionTokens int
    Cost             float64
    Endpoint         string
    RawModel         string
    Provider         string
}
```

## Helper: addToCounter

```go
func addToCounter[K any](m map[string]K, key string, vals CounterVals, meta ...map[string]any) {
    if _, exists := m[key]; !exists {
        m[key] = newCounter[K](meta...)
    }
    m[key].Requests++
    m[key].PromptTokens += vals.PromptTokens
    m[key].CompletionTokens += vals.CompletionTokens
    m[key].Cost += vals.Cost
}
```

## Upsert behavior

When upserting to usageDaily table:
- Read existing record for dateKey
- If exists: merge entry into existing DailyData
- If not exists: create new DailyData with entry
- Write back using INSERT ... ON CONFLICT (dateKey) DO UPDATE SET data = ?

## Logic

1. Define all aggregation types (ProviderAgg, ModelAgg, AccountAgg, ApiKeyAgg, EndpointAgg)
2. Implement NewDayAggregate() constructor
3. Implement AggregateEntry() method on DayAggregate
4. Implement addToCounter helper with proper map initialization
5. Handle composite keys: "model|provider", "apiKey|model|provider", "endpoint|model|provider"
6. Handle empty/blank fields: skip aggregation for empty provider, use "local-no-key" for empty apiKey
7. Implement Serialize() method to convert DayAggregate to JSON for storage
8. Implement Deserialize() method to parse JSON back to DayAggregate

## Acceptance Criteria

- [ ] All aggregation dimensions defined with correct fields
- [ ] aggregateEntryToDay increments all relevant counters
- [ ] Composite keys formatted correctly (model|provider, etc.)
- [ ] Empty provider skipped for byProvider aggregation
- [ ] Empty apiKey uses "local-no-key" for byApiKey key
- [ ] Upsert merges data (not replaces existing aggregates)
- [ ] JSON serialization roundtrip preserves all data
- [ ] Handles concurrent aggregation calls safely

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Single entry | 1 entry with provider="openai", model="gpt-4" | All dimensions updated with counts=1 |
| Multi-provider | 10 entries: 6 openai, 4 anthropic | byProvider["openai"]=6, byProvider["anthropic"]=4 |
| Multi-model | 5 entries: gpt-4, gpt-3.5, claude-3 | 3 keys in byModel with correct counts |
| Composite key format | model="gpt-4", provider="openai" | byModel["gpt-4\|openai"] incremented |
| Empty provider | entry with provider="" | byProvider not updated, others still aggregated |
| Empty apiKey | entry with apiKey="" | byApiKey["local-no-key\|model\|provider"] updated |
| Merge existing | Existing 100 requests + new 10 requests | Total 110 requests |
| Concurrent aggregation | 100 goroutines adding entries | All aggregated correctly, no data race |
| JSON roundtrip | DayAggregate with all dimensions | All fields preserved |
| Multi-dimension update | Single entry updates byProvider, byModel, byApiKey, byEndpoint | All dimensions updated |
