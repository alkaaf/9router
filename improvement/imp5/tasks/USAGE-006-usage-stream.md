---
id: USAGE-006
domain: usage
status: TODO
estimate: 1h
title: Usage Tracking Service
---

## Description

Implement the in-memory state management service for usage tracking. This replaces the global state in Node.js `usageRepo.js` — includes StatsEmitter, PendingTracker, RecentRing, WriteQueue, and ConnectionMapCache.

## Input

- Write queue entries (UsageHistoryEntry)
- Pending request tracking data
- SSE event subscriptions

## Output

- Thread-safe usage service with all in-memory components
- Event emitter for SSE stream updates

## Components

### StatsEmitter (EventEmitter pattern)

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

### PendingRequests state

```go
type PendingTracker struct {
    mu        sync.RWMutex
    byModel   map[string]int    // "gpt-4 (openai)": count
    byAccount map[string]map[string]int  // connectionId → { "gpt-4 (openai)": count }
    timers    map[string]*time.Timer  // "${connId}|${modelKey}" → timer
}
const PENDING_TIMEOUT_MS = 60 * 1000
```

### RecentRing buffer

```go
type RecentRing struct {
    mu     sync.RWMutex
    items  []UsageHistoryEntry  // max 50 items, LRU-style
}
```

### WriteQueue + Scheduler

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

### ConnectionMapCache

```go
type ConnectionMapCache struct {
    mu        sync.RWMutex
    map_      map[string]string  // connectionId → name/email
    ts        int64
}
const CONN_CACHE_TTL_MS = 30 * 1000
```

## Logic

1. **StatsEmitter**: Implement subscribe/unsubscribe pattern with channel-based events
2. **PendingTracker**: Use RWMutex for thread-safe map access, track timers for 60s timeout cleanup
3. **RecentRing**: Fixed-size circular buffer, oldest entries evicted when > 50
4. **WriteQueue**: Non-blocking enqueue, arm timer on first entry, flush when batch full or timer fires
5. **ConnectionMapCache**: TTL-based cache, fetch from DB when expired
6. All components must be thread-safe using sync.RWMutex or sync.Mutex
7. Implement graceful cleanup: stop all timers, drain queues on shutdown

## Acceptance Criteria

- [ ] StatsEmitter notifies all listeners on SendUpdate/SendPending
- [ ] PendingTracker increments/decrements counts correctly
- [ ] PendingTracker clears stale requests after 60s timeout
- [ ] RecentRing maintains max 50 items, oldest evicted
- [ ] WriteQueue flushes on batch size (50) or timer (1s)
- [ ] WriteQueue handles concurrent enqueue safely
- [ ] ConnectionMapCache respects 30s TTL
- [ ] All components thread-safe under concurrent access
- [ ] Goroutines cleaned up on service shutdown

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Emitter single listener | Subscribe + SendUpdate | Handler called once |
| Emitter multiple listeners | 3 listeners + SendUpdate | All 3 handlers called |
| Emitter unsubscribe | Subscribe, unsubscribe, SendUpdate | Handler not called |
| Pending start | TrackPending(model, conn, started=true) | byModel[model] incremented |
| Pending end | TrackPending(model, conn, started=false) | byModel[model] decremented |
| Pending timeout | Start pending, wait 60s | Entry removed from map |
| Pending concurrent | 100 goroutines simultaneous | Correct counts, no race |
| Ring add first | Push entry when ring empty | 1 item in ring |
| Ring full | Push 51st entry | 50 items, oldest removed |
| Queue timer | Add 1 entry, wait 1001ms | Flushed after timer |
| Queue batch | Add 50 entries | Immediate flush |
| Queue concurrent | 10 goroutines enqueue | All entries written correctly |
| Cache hit | Query within 30s TTL | DB not called |
| Cache miss | Query after 30s TTL | DB called, cache updated |
| Shutdown drain | 100 entries in queue + shutdown | All 100 flushed |
