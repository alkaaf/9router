---
id: EXEC-003
domain: executors
status: DONE
estimate: 2d
title: OpenAI Executor (Reference Implementation)
---

## Description

Port the OpenAI executor, which is the simplest and most standardized executor. The Node.js codebase uses `DefaultExecutor` configured with openai provider config. This Go implementation serves as the reference pattern for all other executors.

## Input

- HTTP POST to `https://api.openai.com/v1/chat/completions`
- Auth: Bearer token (apiKey)
- Standard SSE response stream
- Body passthrough (no transformation needed)

## Output

- `internal/executor/openai.go` — OpenAIExecutor struct extending BaseExecutor
- Minimal overrides since base handles standard Bearer auth

## Logic

1. Define `OpenAIExecutor` struct extending `BaseExecutor`
2. Override `BuildUrl` — constructs `https://api.openai.com/v1/chat/completions` with stream param
3. Override `BuildHeaders` — `Authorization: Bearer {apiKey}`, `Content-Type: application/json`
4. Passthrough `TransformRequest` — no body modification needed
5. SSE parsing via `bufio.Scanner` — extract `data:` lines, parse JSON chunks
6. Extract `usage` from final SSE chunk, pass to tracking service
7. Accumulate tool_call deltas, emit complete tool_call on finish_reason

## Acceptance Criteria
- [ ] Bearer token correctly set in Authorization header
- [ ] SSE chunks correctly parsed from response stream
- [ ] Usage tokens extracted from final chunk
- [ ] Tool calls accumulated across deltas and emitted on completion
- [ ] Error mapping: 400, 401, 429, 500 → typed ExecutorError
- [ ] Integration test with mock HTTP server returning SSE chunks

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Bearer auth | BuildHeaders with apiKey | Authorization: Bearer {key} header |
| SSE parsing | SSE stream with 3 chunks | 3 parsed JSON objects |
| Tool call accumulation | Multiple delta chunks | Complete tool_call object |
| Usage extraction | Final chunk with usage | Usage field populated |
| Error 401 | Response status 401 | ExecutorError{Code:"AuthFailed"} |
| Error 429 | Response status 429 | ExecutorError{Code:"RateLimited"} |

## Agent Log
- Started: 2026-06-04 17:00
- Completed: 2026-06-04 17:20
- Agent: agent-exec
- AC-001 verified: `TestOpenAIExecutor_BearerAuth` — Authorization = "Bearer sk-abc".
- AC-002 verified: `TestParseSSEStream_ThreeChunks` — 3 SSE chunks parsed into 1 concatenated Content="ABC".
- AC-003 verified: `TestStreamAccumulator_UsageCallback` — usage field extracted, callback fires with correct token counts.
- AC-004 verified: `TestStreamAccumulator_ToolCallDelta` — 4 chunks assembled into a single ToolCall with ID, Type, Name, Arguments.
- AC-005 verified: `TestOpenAIExecutor_ParseError_Table` — 4 cases (400/401/429/500) all map to correct ErrorCode.
- AC-006 verified: `TestOpenAIExecutor_Execute_StreamsUsage` — full integration: httptest server streams SSE → executor → ParseSSEStream → accumulator; auth header, content, finish_reason, and usage all extracted.
- All tests pass: `go test -race ./internal/executor/` → ok in 2.5s.

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/executor/openai.go, internal/executor/openai_test.go
- Test runtime: ~1.5s sequential, ~2.5s with -race
