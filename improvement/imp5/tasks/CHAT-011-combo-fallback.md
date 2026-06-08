---
id: CHAT-011
domain: chat-core
status: DONE
estimate: 2h
title: Combo Fallback
---

## Agent Log
- Started: 2026-06-04 19:25
- Agent: agent-chat

## Description

Execute fallback strategy for combo requests: try models in order, on fallbackable error continue to next, on non-fallbackable error or success return immediately. Tracks earliest retry-after across all attempts.

## Input

- `[]string` (models in combo)
- `func(string) (*Response, error)` (model handler function)
- `*Logger`
- Combo config

## Output

- `*Response` (first success or final error)
- Tracks retry metrics

## Logic

1. For each model in order:
   a. Call handler(model)
   b. On success → return response
   c. On error, check if fallbackable (429, 500, 503, network)
   d. If non-fallbackable (400, 401) → return immediately
   e. If 503/502/504 with cooldown ≤ 5000ms → wait before continuing
   f. Track earliest retry-after
2. If all fail, return last error
3. Aggregate error info for response

## Acceptance Criteria
- [x] First model succeeds returns first
- [x] First fails fallbackable, second succeeds returns second
- [x] All fail returns last error
- [x] Non-fallbackable (400) returns immediately
- [x] Transient 503 with cooldown waits then retries
- [x] Earliest retry-after tracked across attempts

## Agent Log
- Started: 2026-06-04 19:25
- Implemented: 2026-06-04 17:36
- Agent: agent-chat

### Implementation
- Created `internal/chatcore/combo_executor.go` (151 lines).
- `ComboFallback` struct + `NewComboFallback` constructor + `Execute(ctx, models)`.
- `UpstreamError` type carries status/text for decision-making.
- `ComboError` wraps the last error with `Attempt/Total/Retryable`.
- Tracks `EarliestRetryAfter` via `*time.Time` pointer.
- Uses existing `CheckFallbackError` (backoff.go) for decisions.
- Cooldown sleep suppressed when cooldown > `cooldownOn503Ms` ceiling.
- Context cancellation checked before each attempt and during wait.

### Evidence
- `go test -v -run "TestComboFallback" ./internal/chatcore/...`: 10 PASS in 0.547s.
- `go build ./...`: clean.
- Tests cover: first-succeeds, second-succeeds, all-fail, non-fallbackable-stop, transient-wait (503 and 429 with coerced base), earliest-retry-after, context-cancel, nil-exec, empty-models.

