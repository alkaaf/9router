---
id: CHAT-015
domain: chat-core
status: DONE
estimate: 1h
title: Client Disconnect
---

## Description

Detect client disconnect mid-stream. Uses Fiber context Done() channel. When triggered, abort upstream executor via context cancellation and trigger cleanup callback.

## Input

- `context.Context`
- `*StreamController`
- `func()` (onDisconnect callback)

## Output

- Triggers callback and cancels upstream request

## Logic

1. Listen on `ctx.Done()` channel
2. When channel closes (client disconnected):
   a. Call onDisconnect callback
   b. Cancel upstream request via signal.Cancel()
   c. Stop processing further chunks
3. Handle cleanup in callback (usage tracking, etc.)
4. Return 499 status code if still processing

## Acceptance Criteria
- [x] Client disconnect triggers callback
- [x] Upstream request cancelled
- [x] Server-side close does not trigger callback
- [x] Normal completion does not trigger callback
- [x] Cleanup runs on disconnect

## Agent Log
- Started: 2026-06-04 19:25
- Implemented: 2026-06-04 17:36
- Agent: agent-chat

### Implementation
- Created `internal/chatcore/disconnect.go` (145 lines).
- `DisconnectWatcher` observes ctx.Done() and a server-side done channel.
- Calls `onDisconnect` callback + `cancelUp` exactly once.
- `MarkComplete()` signals normal finish, preventing spurious callbacks.
- `Wait(timeout)` blocks until disconnect or completion.
- `IsDisconnectErr` helper recognises `ErrClientDisconnect` and `context.Canceled`.

### Evidence
- `go test -race -count=5 -run "TestDisconnect" ./internal/chatcore/...`: PASS every iteration (2.8s total).
- Tests cover: fires-on-cancel (verify callback + upstream cancel), server-close-does-not-fire, normal-finish, only-once, IsDisconnectErr.

