---
id: COMBO-006
domain: combos
status: TODO
estimate: 1h
title: Define Strategy Interface
---

## Description

Create `internal/combo/strategy.go` with:
```go
type Selector interface {
    // NextOrder returns the models in the order they should be tried.
    NextOrder(comboName string, models []string) []string
    // Reset clears any per-combo state for comboName (no-op for fallback).
    Reset(comboName string)
    // ResetAll clears all state.
    ResetAll()
}
```
Plus a `Kind` enum: `KindFallback`, `KindRoundRobin`. `NewSelector(kind string, stickyLimit int) (Selector, error)` returns the right implementation or an error for unknown kinds. `stickyLimit <= 0` is normalized to `1` (matching `normalizeStickyLimit` in combo.js).

## Input

Strategy kind string, sticky limit (any numeric).

## Output

`Selector` ready for use; error if kind is unknown.

## Logic

- Define `Selector` interface with `NextOrder`, `Reset`, `ResetAll` methods
- Define `Kind` constants: `KindFallback = "fallback"`, `KindRoundRobin = "round-robin"`
- `NewSelector(kind, stickyLimit)` factory returns appropriate implementation
- `stickyLimit <= 0` normalized to `1`
- Unknown kind returns error

## Acceptance Criteria
- [ ] `Selector` interface defines all required methods
- [ ] `Kind` enum constants defined
- [ ] `NewSelector("fallback", 3)` returns FallbackSelector
- [ ] `NewSelector("round-robin", 0)` normalizes sticky to 1
- [ ] `NewSelector("nonsense", 1)` returns error
- [ ] Factory pattern implemented correctly

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Fallback selector | `NewSelector("fallback", 3)` | FallbackSelector instance |
| Round-robin selector | `NewSelector("round-robin", 1)` | RoundRobinSelector instance |
| Zero sticky limit | `NewSelector("round-robin", 0)` | Normalized to 1 |
| Negative sticky | `NewSelector("round-robin", -5)` | Normalized to 1 |
| Unknown kind | `NewSelector("unknown", 1)` | Error returned |