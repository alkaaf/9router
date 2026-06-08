---
id: EXEC-015
domain: executors
status: DONE
estimate: 1.5d
title: RTK Caveman Context Compression
---

## Description

Port the RTK caveman compression logic. This is a context compression system that reduces LLM agent call costs by compressing long `tool_result` content in request bodies before they are sent to the provider.

## Input

- `compressMessages(body, enabled)` function
- Multiple message shapes to handle:
  1. OpenAI tool response: `{role: "tool", content: "string"}`
  2. Claude tool_result string: `{content: [{type: "tool_result", content: "string"}]}`
  3. Claude tool_result array: `{content: [{type: "tool_result", content: [{type: "text", text: "..."}]}]}`
  4. OpenAI Responses: `{type: "function_call_output", output: string | array}`
  5. Kiro format: `conversationState.history[].userInputMessage...toolResults[].content[].text`
- `MIN_COMPRESS_SIZE` threshold
- `RAW_CAP` maximum compression length
- Auto-detect filter selection per block

## Output

- `internal/rtk/rtk.go` — Main `CompressMessages` function
- `internal/rtk/caveman.go` — Compression strategy (placeholder for algorithm)
- Returns statistics: `{BytesBefore, BytesAfter, Hits[{Shape, Filter, Saved}]}`

## Logic

1. Define `CompressMessages(body []byte, enabled bool)` function
2. Implement shape detection for all 5 message formats
3. Implement auto-detect filter selection via `autoDetectFilter(text)`
4. Implement safe application via `safeApply(fn, text)` — panic recovery with defer/recover
5. Implement statistics tracking: `{BytesBefore, BytesAfter, Hits}`
6. Implement compression thresholds: skip if below MIN_COMPRESS_SIZE, cap at RAW_CAP
7. Implement Kiro-specific compression path
8. Strategy pattern for compression algorithm (caveman as placeholder)

## Acceptance Criteria
- [x] All 5 message shapes correctly detected
- [x] Compression only applied above MIN_COMPRESS_SIZE
- [x] Compressed output never exceeds RAW_CAP
- [x] `safeApply` never returns empty string
- [x] Statistics accurately track compression savings
- [x] Panic safety: broken filter doesn't crash process
- [x] Kiro format correctly compressed via separate path

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| OpenAI tool response | Tool message with long content | Compressed message |
| Claude tool_result string | Claude result block | Compressed content |
| Claude tool_result array | Nested Claude result | Compressed nested content |
| Below threshold | Content shorter than MIN_COMPRESS_SIZE | Original unchanged |
| Exceeds RAW_CAP | Very long content | Capped at RAW_CAP |
| Statistics | Compressed content | Accurate BytesBefore/After |
| Safe apply | Intentional panic in filter | Recovered, original returned |
| Kiro format | Kiro conversationState | Compressed via Kiro path |

## Agent Log
- 2026-06-04 17:08 — Verified implementation: `internal/rtk/rtk.go` (CompressMessages handles all 5 message shapes; autoDetectFilter selects filter per block; safeApply with defer/recover panic safety; statistics tracking with BytesBefore/After/Hits; Kiro path via `conversationState.history[].userInputMessage...toolResults[]`).
- 2026-06-04 17:08 — Tests: `internal/rtk/rtk_test.go` covers all 5 shapes, threshold/RAW_CAP, safeApply panic recovery, statistics accuracy, Kiro format.
- 2026-06-04 17:08 — Verification: `go build ./internal/rtk/...` passes; `go test ./internal/rtk/...` PASS (`ok internal/rtk`).
