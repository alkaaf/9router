---
id: COMBO-007
domain: combos
status: TODO
estimate: 30m
title: Implement Fallback Strategy
---

## Description

Implement `FallbackSelector` in `internal/combo/fallback.go`. `NextOrder` returns `models` unchanged. `Reset` and `ResetAll` are no-ops. Behaviour must match the Node.js fallback: try index 0, then 1, ... in input order.

## Input

`models []string`

## Output

Same slice (or a defensive copy — pick a copy to keep the contract safe for mutating callers).

## Logic

- `NextOrder(comboName, models)` returns models in input order
- Returns a defensive copy to prevent caller mutation
- `Reset(comboName)` is no-op
- `ResetAll()` is no-op
- No internal state maintained

## Acceptance Criteria
- [ ] `NextOrder` returns input order for arbitrary slices
- [ ] Returns a new slice (defensive copy), not the original
- [ ] No internal state: 1000 calls return identical results
- [ ] `Reset` is no-op
- [ ] `ResetAll` is no-op
- [ ] Single model returns unchanged

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Three models | `["a","b","c"]` | `["a","b","c"]` |
| Single model | `["a"]` | `["a"]` |
| Empty slice | `[]` | `[]` |
| 1000 calls identity | `["a","b"]` | Always `["a","b"]` |
| Reset no-op | After Reset | Same behavior as before |
| ResetAll no-op | After ResetAll | Same behavior as before |