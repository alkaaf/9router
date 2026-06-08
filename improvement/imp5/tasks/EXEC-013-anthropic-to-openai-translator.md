---
id: EXEC-013
domain: executors
status: DONE
estimate: 3d
title: Anthropic ↔ OpenAI Translator
---

## Description

Port the bidirectional format translation between Anthropic (Claude) format and OpenAI (Chat Completions) format. Includes both request and response translation pipelines with streaming state management.

## Input

- Request translators: Claude messages → OpenAI messages, OpenAI messages → Claude messages
- Response translators: Claude SSE → OpenAI SSE, OpenAI SSE → Claude SSE
- Node.js source: `claude-to-openai.js`, `openai-to-claude.js`

## Output

- `internal/translator/translator.go` — Translator interface + pipeline orchestrator
- `internal/translator/anthropic.go` — Both directions
- `internal/translator/anthropic_test.go`

## Logic

1. Define `Translator` interface: `TranslateRequest(req)`, `TranslateResponse(chunk, state)`
2. Implement Claude → OpenAI request translation:
   - `content blocks: text, tool_use, tool_result, thinking` → message roles
   - `system prompt` → system message
   - `thinking blocks` → reasoning_content delta
   - `tool_use` → `tool_calls`
3. Implement OpenAI → Claude request translation:
   - `messages[] with roles` → content blocks
   - `system message` → system[]
   - `tool_calls` → tool_use blocks
   - `tool results` → tool_result blocks
4. Implement Claude → OpenAI response translation:
   - Stream content deltas, finish reasons, tool calls
5. Implement streaming state management for response translation (accumulate partial deltas)
6. Implement tool block translation: function → tool_use, function_call → tool_use.name+arguments

## Acceptance Criteria
- [x] Claude request correctly translated to OpenAI format
- [x] OpenAI request correctly translated to Claude format
- [x] Claude SSE correctly translated to OpenAI SSE
- [x] Streaming state maintained across chunks
- [x] Tool definitions correctly mapped in both directions
- [x] Round-trip translation preserves semantic meaning

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Claude system prompt | Claude body with system | OpenAI body with system message |
| Claude tool_use | Claude content block | OpenAI tool_calls |
| OpenAI tool_calls | OpenAI message with tool_calls | Claude tool_use blocks |
| Thinking block | Claude thinking block | reasoning_content delta |
| Streaming delta | Partial Claude SSE chunk | Partial OpenAI SSE chunk |
| Round-trip | Claude → OpenAI → Claude | Original preserved |

## Agent Log
- 2026-06-04 17:08 — Verified implementation: `internal/translator/anthropic.go` (full bidirectional Anthropic ↔ OpenAI translator with TranslateRequest and TranslateResponse; system prompt extraction; tool block mapping; thinking → reasoning_content; streaming state management via State struct).
- 2026-06-04 17:08 — Tests: `internal/translator/anthropic_test.go` covers system prompt, tool_use, tool_calls, thinking blocks, streaming deltas, round-trip.
- 2026-06-04 17:08 — Verification: `go build ./internal/translator/...` passes; `go test ./internal/translator/...` PASS (`ok internal/translator`).
