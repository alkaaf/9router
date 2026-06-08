---
id: USAGE-009
domain: usage
status: TODO
estimate: 1h
title: Write Queue Service
---

## Description

Implement the write queue service for buffered batch writes to the database. This replaces synchronous writes with an async batching layer.

## Input

- UsageHistoryEntry items to be written
- Flush configuration (batch size, flush interval)

## Output

- Buffered async writes with configurable batch sizes
- Automatic flush on batch full or timer expiry

## Configuration constants

```go
const WRITE_BATCH_MAX = 50       // Flush immediately when queue reaches this size
const WRITE_BATCH_MS  = 1000     // Flush after this many milliseconds if queue not full
```

## WriteQueue implementation

```go
type WriteQueue struct {
    mu       sync.Mutex
    entries  []UsageHistoryEntry
    busy     bool
    timer    *time.Timer
}

type WriteQueueService struct {
    queue       *WriteQueue
    repo        *UsageRepository
    pricingSvc  *PricingService
    emitter     *UsageStatsEmitter
    stopChan    chan struct{}
}
```

## Core methods

### Enqueue(entry UsageHistoryEntry)

```
- Add entry to queue (thread-safe via mutex)
- If queue size >= WRITE_BATCH_MAX: trigger immediate flush
- If queue was empty: arm timer for WRITE_BATCH_MS
- Return immediately (non-blocking)
```

### flush()

```
- Lock queue mutex
- If busy: return (another flush in progress)
- Set busy = true
- Copy entries to flush (up to WRITE_BATCH_MAX)
- Clear entries being flushed from queue
- Unlock mutex

- For each entry: calculate cost via pricing lookup
- Write all to usageHistory in single transaction
- Update usageDaily aggregates
- Push each to recent ring buffer
- Emit statsEmitter.SendUpdate()

- Re-arm timer if queue still has entries
- Set busy = false
```

### Start()

```
- Initialize queue
- Start background goroutine waiting for flush signals
- Return
```

### Stop()

```
- Signal stop
- Drain remaining entries (all of them, not just batch)
- Flush to database
- Stop background goroutine
```

## Pricing lookup optimization

In the flush goroutine:
1. Collect all unique (provider, model) pairs from entries
2. Fetch pricing for all pairs in one batch call
3. Compute cost for each entry using cached pricing
4. This avoids N pricing lookups for N entries

## Logic

1. Enqueue is always non-blocking — adds to queue and schedules
2. Timer only armed when queue transitions from empty to non-empty
3. Timer cancelled when queue drained to empty
4. If flush() is called while busy: the enqueueing call handles it
5. Stop() must drain ALL remaining entries, not just one batch
6. Use atomic bool or mutex for busy flag
7. On error during flush: log error, retry up to 3 times with backoff
8. Background drain goroutine: single goroutine that processes flushes

## Acceptance Criteria

- [ ] Enqueue is non-blocking and returns immediately
- [ ] Queue flushes immediately when reaching WRITE_BATCH_MAX (50)
- [ ] Queue flushes after WRITE_BATCH_MS (1000ms) if not full
- [ ] Concurrent Enqueue calls are thread-safe
- [ ] Multiple flushes don't overlap (busy flag works)
- [ ] Stop() drains ALL remaining entries
- [ ] Stop() waits for final flush to complete
- [ ] Pricing lookups batched per flush (not per entry)
- [ ] Errors during flush don't crash the service
- [ ] No entries lost on shutdown (Stop drains queue)

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Enqueue non-blocking | 100 entries, one at a time | Each returns immediately |
| Timer flush | 1 entry, wait 1100ms | Flushed after timer expires |
| Batch flush | 50 entries | Immediate flush triggered |
| Partial batch + timer | 25 entries, wait 1100ms | Flushed after timer |
| Concurrent enqueue | 10 goroutines, 10 entries each | All 100 written correctly |
| Overlapping flushes | Flush called while busy | Second flush waits, not overlaps |
| Empty queue flush | Flush called with 0 entries | No-op |
| Stop drains all | 100 entries in queue, Stop() | All 100 flushed to DB |
| Stop waits | Stop() called during flush | Waits for flush to complete |
| Error retry | DB write fails | Retries up to 3 times |
| Pricing batch | 50 entries with 5 unique model/provider pairs | 5 pricing lookups, not 50 |
| No goroutine leak | Enqueue/Stop cycle 100 times | Constant number of goroutines |
