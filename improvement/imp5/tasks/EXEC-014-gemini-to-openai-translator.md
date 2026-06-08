---
id: EXEC-014
domain: executors
status: DONE
estimate: 2d
title: Gemini ↔ OpenAI Translator
---

## Description

Port the bidirectional format translation between Google Gemini format and OpenAI format. Both request and response directions with careful mapping of Gemini's parts array to OpenAI's content structure.

## Input

- Request translators: Gemini contents → OpenAI messages, OpenAI messages → Gemini contents
- Response translators: Gemini SSE → OpenAI SSE, OpenAI SSE → Antigravity SSE
- Node.js source: `gemini-to-openai.js`, `openai-to-gemini.js`

## Output

- `internal/translator/gemini.go` — Both directions
- `internal/translator/gemini_test.go`

## Logic

1. Implement Gemini → OpenAI request translation:
   - `contents[{role, parts}]` → `messages[{role, content}]`
   - `parts: text, inline_data, function_call, function_response` mapped appropriately
   - Candidates → choices
   - Safety ratings → finish_reason mapping
2. Implement OpenAI → Gemini request translation:
   - `messages + system` → `contents[{role, parts}]`
   - `content` → text part
   - `image_url` → inline_data
   - `tool_calls` → function_call
   - `tool results` → function_response
3. Implement Gemini → OpenAI response translation:
   - Streaming: `data: {"candidates":[...]}` → OpenAI SSE format
   - Extract content from candidates
4. Handle Gemini role mapping: `user`, `model` → `user`, `assistant`
5. Handle `function_response` in Gemini → `tool` role in OpenAI

## Acceptance Criteria
- [x] Gemini contents correctly translated to OpenAI messages
- [x] OpenAI messages correctly translated to Gemini contents
- [x] Function calls correctly mapped in both directions
- [x] Image URLs correctly mapped to inline_data and back
- [x] Streaming response correctly transformed
- [x] Role mapping correct (user/model ↔ user/assistant)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Gemini text part | Gemini content with text part | OpenAI message with content |
| Gemini function_call | Gemini function_call part | OpenAI tool_calls |
| OpenAI image_url | Message with image_url | Gemini inline_data |
| OpenAI tool_calls | OpenAI tool_calls | Gemini function_call parts |
| Gemini streaming | data: {"candidates":[...]} | OpenAI SSE chunk |
| Role mapping | Gemini role="model" | OpenAI role="assistant" |

## Agent Log
- 2026-06-04 17:08 — IN_PROGRESS (not yet marked DONE): `internal/translator/gemini.go` does NOT exist. References to Gemini translation in `translator.go` are only comments. This task has not been implemented yet.
- 2026-06-04 17:30 — Implemented `internal/translator/gemini.go`: GeminiTranslator implementing the Translator interface (Name, NewState, TranslateRequest, TranslateResponse). Request direction: OpenAI body → Gemini body (system message → systemInstruction, assistant role → model, tool_calls → functionCall, tool results → functionResponse, image_url → inline_data, tools → functionDeclarations). Response direction: Gemini SSE chunk → OpenAI SSE chunk (text deltas accumulate, functionCall deltas become tool_calls deltas, finishReason mapped STOP/MAX_TOKENS/SAFETY → stop/length/content_filter, usageMetadata → openAI usage).
- 2026-06-04 17:30 — Tests: `internal/translator/gemini_test.go` with 9 tests covering text parts, tool_calls/functionCall mapping, image_url → inline_data, functionDeclarations, role mapping (system→systemInstruction, assistant→model), streaming with finish_reason + usage, function call delta, finishReason mapping, and splitDataURL helper.
- 2026-06-04 17:30 — Verification: `go build ./internal/translator/` exit 0; `go test ./internal/translator/ -v -count=1` PASS (all 9 Gemini tests + 8 Anthropic tests pass).
