---
id: COMBO-008
domain: combos
status: TODO
estimate: 1h
title: Implement Round-Robin Strategy
---

## Description

Implement `RoundRobinSelector` in `internal/combo/roundrobin.go`. Use a `sync.Map[string, *rrState]` (Go 1.21+ `sync.Map` with typed wrapper) keyed by `comboName` (default `"__default__"`), where `rrState = struct{ index int; consecutiveUseCount int }`. On each `NextOrder`:
1. If `len(models) <= 1`, return `models` unchanged.
2. Read state (or seed `{0,0}`).
3. Compute `rotated = rotateSlice(models, state.index)`.
4. Increment `consecutiveUseCount`; if `>= stickyLimit` -> advance `index` and reset counter; else keep `index` and store new counter.
5. Persist updated state.
6. Return `rotated`.

## Input

`comboName string`, `models []string`

## Output

Rotated `[]string` (new slice).

## Logic

- Use `sync.Map` for thread-safe per-combo state
- Per-combo state: `{index int, consecutiveUseCount int}`
- If `len(models) <= 1`: return unchanged
- Seed state to `{0, 0}` if missing
- Rotate slice starting at `state.index`
- Increment consecutive count; if >= stickyLimit: advance index, reset count
- Store updated state atomically
- Return new rotated slice

## Acceptance Criteria
- [ ] 1 model returns unchanged, no state stored
- [ ] 3 models, sticky=1: call 1 returns `[a,b,c]`, call 2 `[b,c,a]`, call 3 `[c,a,b]`, call 4 `[a,b,c]`
- [ ] 3 models, sticky=3: first 3 calls return `[a,b,c]`, 4th returns `[b,c,a]`
- [ ] Sticky=0 coerced to 1
- [ ] Different combo names maintain independent indices
- [ ] Concurrent calls from 100 goroutines: `go test -race` passes
- [ ] `Reset(comboName)` clears that combo's state
- [ ] `ResetAll()` clears all state

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Single model | `["a"]` | `["a"]`, no state |
| Sticky=1, call 1 | `["a","b","c"]` | `["a","b","c"]` |
| Sticky=1, call 2 | `["a","b","c"]` | `["b","c","a"]` |
| Sticky=1, call 3 | `["a","b","c"]` | `["c","a","b"]` |
| Sticky=1, call 4 | `["a","b","c"]` | `["a","b","c"]` |
| Sticky=3, calls 1-3 | `["a","b","c"]` | All `["a","b","c"]` |
| Sticky=3, call 4 | `["a","b","c"]` | `["b","c","a"]` |
| Sticky=0 | `["a","b"]` | Coerced to 1 |
| Different combo | Combo A vs B | Independent indices |
| Concurrent safety | 100 goroutines | No race, index in [0, len) |
| Reset | After Reset("combo") | State cleared |
| ResetAll | After ResetAll() | All states cleared |