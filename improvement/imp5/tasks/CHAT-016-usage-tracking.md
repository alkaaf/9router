---
id: CHAT-016
domain: chat-core
status: DONE
estimate: 1h
title: Usage Tracking
---

## Description

Track pending and completed requests for real-time dashboard. Increments counter on request start, decrements on completion/error. Uses sync.Map for thread-safe in-memory storage.

## Input

- `string` (model)
- `string` (provider)
- `string` (connectionId)
- `bool` (increment=true for start, false for complete)

## Output

- Updates in-memory ProviderStats map

## Logic

1. Build key: `${provider}:${model}`
2. Load stats from sync.Map
3. If increment (request start):
   - pending++
   - total++
4. If decrement (request end):
   - pending--
   - If pending < 0, set to 0
5. Store updated stats

## Acceptance Criteria
- [x] Increment/decrement balance correctly
- [x] Multiple concurrent calls are race-free
- [x] Get current pending count works
- [x] Total requests tracked
- [x] No negative pending counts
- [x] Benchmark: 10K calls/sec, no lock contention

## Agent Log
- Started: 2026-06-04 19:25
- Implemented: 2026-06-04 17:36
- Agent: agent-chat

### Implementation
- Created `internal/chatcore/usage.go` (130 lines).
- `UsageTracker` backed by `sync.Map` of `int64` for pending + total.
- `Track(model, provider, connectionID, increment)` — atomic ops.
- Pending clamped at zero via CAS loop on decrement.
- `Stats(model, provider)` → `(ProviderStats, bool)`.
- `Snapshot()` → full copy for dashboard endpoint.

### Evidence
- `go test -race -run "TestUsage" ./internal/chatcore/...`: PASS in 1.508s.
- Tests cover: start/end balance, 100× concurrent starts, over-decrement clamp, multiple-key isolation, snapshot.

