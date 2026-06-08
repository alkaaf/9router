---
id: USAGE-008
domain: usage
status: TODO
estimate: 1h
title: Usage SSE Stream Handler
---

## Description

Implement real-time SSE streaming for live usage updates. The stream pushes full stats on connect, then lightweight updates on pending changes.

## Input

- SSE client connection
- statsEmitter subscriptions

## Output

- text/event-stream response with usage update events

## SSE stream format

```
GET /api/usage/stream

Response: text/event-stream (no status code on success — stream starts immediately)

Event sequence:
  1. Full stats push:
     data: { ...full usageStats object... }\n\n

  2. Subsequent lightweight pushes (on pending changes):
     data: { ...stats with activeRequests, recentRequests, errorProvider ... }\n\n

  3. Ping every 25 seconds:
     data: : ping\n\n\n

Headers:
  Content-Type: text/event-stream
  Cache-Control: no-cache
  Connection: keep-alive
```

## Two-tier push pattern

### send() — Heavy push
- Emits on: statsEmitter "update" event
- Sends lightweight update immediately, then recalculates full stats in background
- Full stats pushed after recalculation

### sendPending() — Light push
- Emits on: statsEmitter "pending" event
- Only sends: activeRequests + recentRequests + errorProvider
- Skipped on first call (no cachedStats yet)

### keepalive
- Ping comment every 25 seconds via ticker

## Context lifecycle

```
start:  → start SSE stream, subscribe to emitter events
cancel: → unsubscribe emitter, clear interval, set closed=true
```

## Fiber goroutine management

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

## Logic

1. Set SSE headers on response
2. Create cancellable context for stream lifetime
3. Subscribe to statsEmitter on connect
4. Push full stats immediately on connect
5. Start keepalive ticker (25s interval)
6. On statsEmitter "update": push lightweight, then recalculate full stats
7. On statsEmitter "pending": push only activeRequests/recentRequests/errorProvider
8. On context cancel: unsubscribe, stop ticker, clean up
9. Handle client disconnect gracefully (context cancellation)

## Acceptance Criteria

- [ ] Stream starts immediately on connect with full stats
- [ ] SSE headers set correctly (Content-Type, Cache-Control, Connection)
- [ ] Ping arrives every 25 seconds
- [ ] statsEmitter.SendUpdate() triggers new push
- [ ] statsEmitter.SendPending() triggers lightweight push
- [ ] Client disconnect unsubscribes and cleans up goroutines
- [ ] No goroutine leak on client disconnect
- [ ] Full stats recalculated after lightweight push

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Connect | HTTP GET /api/usage/stream | 200 OK, stream starts with full stats |
| Full stats on connect | SSE stream established | First event contains full usageStats |
| Ping arrives | Wait 26 seconds | data: : ping received |
| SendUpdate trigger | Call statsEmitter.SendUpdate() | New data event pushed |
| SendPending trigger | Call statsEmitter.SendPending() | Lightweight update pushed |
| Client disconnect | Close SSE connection | Goroutine cleaned up, emitter unsubscribed |
| Multiple clients | 3 concurrent SSE connections | Each receives independent stream |
| Reconnect | Disconnect and reconnect | Full stats sent again |
| Context cancel | Server shutdown | Stream closes gracefully |
