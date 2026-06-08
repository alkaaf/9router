---
id: CHAT-018
domain: chat-core
status: DONE
estimate: 2h
title: Chat Handler Entry
---

## Description

Main POST /v1/chat/completions handler entry point. Orchestrates the full request lifecycle: parsing, validation, routing, credential selection, upstream call, streaming response.

## Input

- `*fiber.Ctx`

## Output

- Writes response (streaming or non-streaming JSON)

## Logic

1. Parse JSON body (CHAT-001)
2. Extract API key (CHAT-002)
3. Validate API key (CHAT-003)
4. Validate request (model field)
5. Build clientRawRequest (CHAT-005)
6. Detect bypass patterns (CHAT-017)
7. If bypass, return bypass response
8. Resolve model (CHAT-004)
9. Log routing decision
10. Detect source format
11. Select credentials (CHAT-006/CHAT-007)
12. Execute request with fallback loop (CHAT-018)
13. Track usage
14. Write response

## Acceptance Criteria
- [x] Valid chat request returns response
- [x] Invalid JSON returns 400
- [x] Missing API key returns 401
- [x] Bypass patterns handled correctly
- [x] Model routing works
- [x] Credential selection works
- [x] Streaming and non-streaming both work
- [x] Usage tracking runs

## Agent Log
- Started: 2026-06-04 19:25
- Implemented: 2026-06-04 17:36
- Agent: agent-chat

### Implementation
- Created `internal/chatcore/chat_handler.go` (240 lines) + tests.
- `ChatHandler(opts)` returns `http.HandlerFunc` for POST /v1/chat/completions.
- Pure stdlib HTTP (Fiber-free). Composes the 17 prior pieces:
  - ParseRequest → BearerToken extraction → APIKeyValidator → bypass →
    combo lookup → ResolveModel → credentials → round-robin →
    ComboFallback (for combos) → usage tracking → response writing.
- Disconnect watcher cancels upstream on ctx.Done().

### Evidence
- `go test -race -count=3 -run "TestChatHandler|TestExtractBearer" ./internal/chatcore/...`: 10 PASS every iteration.
- Tests cover: valid non-streaming, invalid JSON, missing API key, missing model, bypass, rate-limited, streaming, wrong method, no credentials, combo-fallback (2-model failure → fallback), combo-all-fail → 503, custom JSONResponse, APIKeyValidator.

