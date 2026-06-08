---
id: EXEC-012
domain: executors
status: DONE
estimate: 4d
title: Kiro Executor (AWS EventStream)
---

## Description

Port the `KiroExecutor`. Handles Amazon CodeWhisperer's binary AWS EventStream protocol with binary frame parsing, AWS SDK headers, multiple event types, and OAuth token refresh.

## Input

- HTTP POST to Kiro's AWS EventStream endpoint
- Auth: Bearer token (Kiro OAuth access token)
- Headers: `Amz-Sdk-Request`, `Amz-Sdk-Invocation-Id` (UUID per request)
- Response: Binary AWS EventStream frames (not SSE, not JSON)
- Custom retry configuration per status code

## Output

- `internal/executor/kiro.go` — KiroExecutor struct
- Full override of `Execute()` — custom retry logic + binary stream parsing
- AWS EventStream frame parser (`parseEventFrame`)
- Binary → SSE transform

## Logic

1. Define `KiroExecutor` struct
2. Implement `parseEventFrame` — parse AWS EventStream binary format:
   - 4 bytes: total length (big-endian)
   - 4 bytes: headers length (big-endian)
   - Headers: name-length + name + type + value-length + value
   - Payload: JSON string
   - 4 bytes: CRC
3. Implement event type parsing: `assistantResponseEvent`, `reasoningContentEvent`, `codeEvent`, `toolUseEvent`, `messageStopEvent`, `contextUsageEvent`, `meteringEvent`, `metricsEvent`
4. Implement binary → OpenAI SSE transformation
5. Implement token estimation from event metadata
6. Implement `refreshKiroToken` — OAuth token refresh
7. Implement AWS SDK headers (`Amz-Sdk-Request`, `Amz-Sdk-Invocation-Id`)

## Acceptance Criteria
- [x] AWS EventStream frame correctly parsed (all fields extracted)
- [x] Each event type correctly mapped to SSE chunk
- [x] Token estimation accurate (content length / 4 for output, contextUsagePercentage × 200000 for input)
- [x] AWS SDK headers correctly generated per request
- [x] OAuth refresh works correctly
- [x] Custom retry logic per status code implemented

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Frame parsing | Binary EventStream data | Parsed frame with headers and payload |
| assistantResponseEvent | Event data | OpenAI SSE chunk with content |
| reasoningContentEvent | Event data | reasoning_content delta chunk |
| toolUseEvent | Event data | tool_calls delta chunk |
| Token estimation | metricsEvent with content_length=4000 | ~1000 output tokens |
| AWS headers | BuildHeaders | Amz-Sdk-Request, Amz-Sdk-Invocation-Id |
| OAuth refresh | Expired token | New valid token |

## Agent Log
- 2026-06-04 17:08 — Verified implementation: `internal/executor/kiro.go` (KiroExecutor; parseEventFrame handles AWS EventStream binary format with all event types: assistantResponseEvent, reasoningContentEvent, codeEvent, toolUseEvent, messageStopEvent, contextUsageEvent, meteringEvent, metricsEvent; AWS SDK headers generated per request; token refresh in `internal/oauth/kiro_exchange.go` and `internal/oauth/kiro_authorize.go`).
- 2026-06-04 17:08 — Tests: `internal/executor/kiro_test.go` covers frame parsing, event type mapping, token estimation, AWS headers, OAuth refresh.
- 2026-06-04 17:08 — Verification: `go build ./internal/executor/...` passes; `go test ./internal/executor/...` PASS.
