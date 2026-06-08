---
id: COMBO-009
domain: combos
status: TODO
estimate: 1h
title: Implement Sticky-Session Strategy
---

## Description

Implement `StickySelector` in `internal/combo/sticky.go` that wraps `RoundRobinSelector` but maintains stickiness at the **session** level. Session key derives from a request-scoped `SessionKey` interface (e.g. `apiKey` + `clientIP`). For the initial port, the chat handler passes `comboStickyRoundRobinLimit` from settings, and the sticky selector sticks to the same `models[0]` for that many requests before rotating. Internally store `map[comboName]map[sessionKey]int` and look up using the session key passed via a `NextOrderWithSession` variant.

## Input

`comboName string`, `models []string`, `sessionKey string`

## Output

Rotated `[]string` (a one-element shift that puts the sticky model first).

## Logic

- Wrap `RoundRobinSelector` internally
- Store per-combo per-session counters: `map[comboName]map[sessionKey]int`
- On `NextOrderWithSession(comboName, models, sessionKey)`:
  - Get/initialize session counter for (comboName, sessionKey)
  - If counter < stickyLimit: rotate so `models[0]` is first (no actual rotation needed)
  - If counter >= stickyLimit: advance to next model index, reset counter
  - Increment and store counter
- `Reset(comboName)` clears all sessions for that combo
- `ResetAll()` clears all state

## Acceptance Criteria
- [ ] Sticky=3, models=[a,b,c]: first 3 calls return `[a,b,c]`, `[a,b,c]`, `[a,b,c]`
- [ ] After session switch, first 3 calls return `[b,c,a]` etc. (independent counter)
- [ ] `Reset(comboName)` clears only that combo's sessions
- [ ] `ResetAll()` clears everything
- [ ] `NextOrderWithSession` variant exposed
- [ ] Thread-safe via sync.RWMutex

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Sticky=3, session A, call 1 | `["a","b","c"]`, "sessionA" | `["a","b","c"]` |
| Sticky=3, session A, call 2 | `["a","b","c"]`, "sessionA" | `["a","b","c"]` |
| Sticky=3, session A, call 3 | `["a","b","c"]`, "sessionA" | `["a","b","c"]` |
| Sticky=3, session A, call 4 | `["a","b","c"]`, "sessionA" | `["b","c","a"]` |
| Session B independent | After 3 calls, new session B | Returns `["a","b","c"]` |
| Reset clears combo | After Reset("combo") | Session counters cleared |
| ResetAll clears all | After ResetAll() | All counters cleared |
| Single model | `["a"]` | `["a"]` unchanged |