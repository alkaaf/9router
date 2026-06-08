---
id: CHAT-012
domain: chat-core
status: DONE
estimate: 2h
title: Combo Round Robin
---

## Agent Log
- Started: 2026-06-04 19:25
- Agent: agent-chat

## Description

Round-robin combo executor with sticky counter. Rotates starting index per combo, increments consecutiveUseCount, advances to next index when limit reached. In-memory state map per combo name.

## Input

- `[]string` (models)
- `string` (comboName)
- `int` (stickyLimit)
- In-memory rotation state

## Output

- `[]string` (rotated model list)
- Updated rotation state

## Logic

1. Get current RotationState for comboName from sync.Map
2. If no state, create with index=0, count=0
3. If count >= stickyLimit:
   - Increment index (mod length)
   - Reset count to 0
4. Otherwise increment count
5. Save state back
6. Return models rotated to start at computed index

## Acceptance Criteria
- [x] Sticky limit=1 always rotates
- [x] Sticky limit=3 stays for 3 calls then rotates
- [x] Multiple combos tracked independently
- [x] Reset clears state for specific combo
- [x] Thread-safe with sync.Map
- [x] Empty models returns unchanged

## Agent Log
- Started: 2026-06-04 19:25
- Implemented: 2026-06-04 17:36
- Agent: agent-chat

### Implementation
- Created `internal/chatcore/roundrobin.go` (160 lines).
- `ComboRotator` backed by `map[string]*RotationState` protected by `sync.Mutex`.
- `Next(name, models, stickyLimit)` rotates the slice and advances the state.
- `Reset(name)` clears a single combo; `ResetAll()` clears all.
- `State(name)` returns a snapshot of the rotation state.

### Evidence
- `go test -race -run "TestRoundRobin" ./internal/chatcore/...`: PASS in 1.559s.
- Tests cover: limit-1 rotate, limit-3 stay-then-rotate, multiple combos independent, reset clears, thread-safe with 100×100 goroutines, empty/nil models, nil rotator.

