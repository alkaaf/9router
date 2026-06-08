---
id: EXEC-010
domain: executors
status: DONE
estimate: 3d
title: Perplexity Web Executor
---

## Description

Port the `PerplexityWebExecutor`. Interfaces with Perplexity's web SSE API with cookie-based auth, custom non-standard SSE format with `blocks` array structure, session cache with TTL, and thinking mode support.

## Input

- HTTP POST to Perplexity web SSE endpoint
- Auth: Cookie `__Secure-next-auth.session-token` or Bearer token
- Body: Perplexity-specific JSON with `query_str`, `params`, `search_focus`, `model_preference`
- Response: Perplexity's block-based SSE (not standard OpenAI SSE)

## Output

- `internal/executor/perplexity.go` — PerplexityWebExecutor struct
- Full override of `Execute()` — reads non-standard SSE, extracts blocks, emits OpenAI SSE
- Session cache with 1-hour TTL, max 200 entries
- Thinking mode support (search queries streamed as reasoning_content)

## Logic

1. Define `PerplexityWebExecutor` struct with session cache
2. Implement `MODEL_MAP` and `THINKING_MAP` for model resolution
3. Implement `parseOpenAIMessages` — system message extraction, history building
4. Implement `buildQuery` — construct Perplexity-specific payload
5. Implement `readPplxSseEvents` → Go channel generator
6. Implement `extractContent` → Go channel pipeline with block parsing
7. Implement session cache with `sync.RWMutex` + TTL cleanup via `time.Ticker`
8. Implement thinking mode — search queries as reasoning_content delta
9. Implement `cleanResponse` — XML decl, citation marks, grok tags cleanup

## Acceptance Criteria
- [x] Session cache correctly implements TTL and max entries eviction
- [x] Perplexity SSE blocks correctly parsed
- [x] Blocks transformed to OpenAI SSE format
- [x] Thinking mode streams search queries as reasoning_content
- [x] Cookie or Bearer auth correctly handled
- [x] Response cleanup removes XML decl, fixes citation marks

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Session cache TTL | Entry older than 1 hour | Entry evicted on access |
| Session cache max | 201st entry | Oldest entry evicted |
| Block parsing | Perplexity SSE with blocks | OpenAI SSE chunks |
| Thinking mode | Search query event | reasoning_content delta |
| Model mapping | "pplx-sonar" | Correct perplexity model |
| Response cleanup | "<?xml ...>" prefix | Prefix removed |

## Agent Log
- 2026-06-04 17:08 — Verified implementation: `internal/executor/perplexity.go` (PerplexityWebExecutor, perplexitySessionCache with sync.RWMutex, 1-hour TTL, max 200 entries, FIFO eviction; MODEL_MAP and THINKING_MAP defined; cleanResponse strips XML decl).
- 2026-06-04 17:08 — Tests: `internal/executor/perplexity_test.go` covers session cache TTL/max eviction, block parsing, thinking mode deltas, model mapping, response cleanup.
- 2026-06-04 17:08 — Verification: `go build ./internal/executor/...` passes; `go test ./internal/executor/... ./internal/translator/... ./internal/rtk/...` all PASS (`ok internal/executor`, `ok internal/translator`, `ok internal/rtk`).
