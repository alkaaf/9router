---
id: USAGE-007
domain: usage
status: TODO
estimate: 1h
title: Usage Tracking Service — Pending & Write Queue
---

## Description

Implement the usage tracking integration into chat/embeddings handlers. Every LLM request must be tracked with pending state, usage saved, and stats emitted.

## Input

- LLM request metadata (model, provider, connectionId, apiKey, endpoint)
- LLM response tokens (prompt_tokens, completion_tokens)
- LLM request status (ok, error, etc.)

## Output

- Integrated usage tracking in all LLM handler call sites
- Automatic cost calculation and storage

## Entry fields for SaveRequestUsage

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

## Call sites to instrument

- `src/sse/handlers/chat.js` — chat completions (SSE streaming)
- `src/sse/handlers/embeddings.js` — embeddings
- `src/sse/handlers/fetch.js` — web fetch (Perplexity)
- `src/sse/handlers/search.js` — web search

## Integration points

### On request start

1. Call `TrackPendingRequest(model, provider, connectionId, started=true)`
2. This increments in-memory pending counters

### On request completion (success or error)

1. Call `TrackPendingRequest(model, provider, connectionId, started=false)`
2. Call `SaveRequestUsage(entry)` with tokens/cost
3. Call `statsEmitter.SendUpdate()`

### For SSE streaming responses

1. Track pending on stream start
2. Save usage on stream completion (final chunk)
3. If stream errors mid-way: still call `SaveRequestUsage()` with partial tokens

## Pricing lookup

```
cost = calculateCostFromTokens(tokens, pricing)
  → pricing = getPricingForModel(provider, model)
  → getPricingForModel: look up in settings.pricing (JSON) or use defaults
```

## Logic

1. Instrument each handler with TrackPending on request start
2. After LLM response received, extract tokens from response
3. Look up pricing for (provider, model) pair
4. Calculate cost: `inputTokens * pricing.inputCost + outputTokens * pricing.outputCost`
5. Build UsageEntry and call SaveRequestUsage
6. For streaming: accumulate tokens from final chunk metadata, save on stream close
7. On error: set status="error", still save usage entry with partial tokens if available
8. Error providers update `errorProvider` state for live display

## Acceptance Criteria

- [ ] All 4 handler call sites instrumented (chat, embeddings, fetch, search)
- [ ] Pending tracking starts on request initiation
- [ ] Pending tracking ends on request completion
- [ ] Usage saved with correct tokens and cost after each request
- [ ] Streaming responses track pending at start, save at completion
- [ ] Partial stream errors still save usage with available tokens
- [ ] Provider errors set status="error" and update errorProvider
- [ ] Cost calculation uses correct pricing lookup

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Chat success | Full chat completion | usageHistory row with tokens and cost |
| Embeddings success | Full embeddings request | usageHistory row with tokens and cost |
| Chat stream complete | SSE stream finishes | Usage saved with accumulated tokens |
| Chat stream error mid-way | Stream errors after partial response | Usage saved with partial tokens |
| Provider API error | 401 response from provider | status="error", errorProvider set |
| Empty tokens | Response with no tokens | cost = 0, usage still saved |
| Multiple concurrent requests | 10 simultaneous chat requests | All 10 tracked correctly |
| Request timeout | Request exceeds timeout | status="error", usage saved |
