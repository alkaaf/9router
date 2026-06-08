---
id: EXEC-001
domain: executors
status: DONE
estimate: 2d
title: Executor Interface + Base Implementation
---

## Description

Define the Go `Executor` interface and `BaseExecutor` struct that all provider executors will implement. Port the Node.js `BaseExecutor` class, which provides URL building, header construction, request transformation, retry logic with fallback URLs, credential refresh, and error mapping.

## Input

- Node.js `BaseExecutor` source code from `/open-sse/executors/`
- Provider config structure with base URLs, retry config, auth headers

## Output

- `internal/executor/base.go` — interface + base implementation
- `internal/executor/base_test.go` — unit tests

## Logic

1. Define `Executor` interface with `Execute`, `GetProvider`, `RefreshCredentials`, `NeedsRefresh`
2. Define `BaseExecutor` struct with `provider`, `config *ProviderConfig`
3. Implement `BaseUrls()` returning fallback URLs array
4. Implement `BuildUrl(model, stream, urlIndex, creds)` constructing provider-specific URL
5. Implement `BuildHeaders(creds, stream)` constructing auth + custom headers
6. Implement `TransformRequest(model, body, stream, creds)` for request body transformation
7. Implement `ShouldRetry(status, urlIndex)` — 429 + has fallback returns true
8. Implement `Execute(ctx, req)` main loop: iterate URLs, handle retries, return response
9. Implement `ParseError(response, body)` mapping status codes to typed errors
10. Implement `RefreshCredentials(ctx, creds)` as no-op base (override per-provider)
11. Implement `NeedsRefresh(creds)` checking expiresAt timestamp

## Acceptance Criteria
- [x] `Executor` interface defines all required methods
- [x] `BaseExecutor` provides URL building with fallback support
- [x] Retry logic handles 429 with exponential backoff
- [x] Error mapping covers: 400, 401, 403, 429, 500, 502, 503, 504
- [x] Context cancellation aborts in-flight requests
- [x] Unit tests cover BuildUrl, BuildHeaders, TransformRequest, ShouldRetry
- [x] Table-driven tests verify retry behavior across status codes

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| BuildUrl with model and stream | model="gpt-4", stream=true, urlIndex=0 | Correct URL with stream param |
| ShouldRetry 429 with fallback | status=429, urlIndex=0, FallbackCount=2 | true |
| ShouldRetry 500 without fallback | status=500, urlIndex=0, FallbackCount=1 | false |
| ParseError 401 | response status=401, body="invalid token" | ExecutorError{Code:"AuthFailed"} |
| Context cancellation | ctx with deadline exceeded | Request aborted, error returned |
| Fallback URL iteration | First URL returns 503, second returns 200 | Success from second URL |

## Agent Log
- Started: 2026-06-04 16:00
- Completed: 2026-06-04 16:45
- Agent: agent-exec
- AC-001 verified: `Executor` interface with Execute/GetProvider/RefreshCredentials/NeedsRefresh defined at `internal/executor/base.go:131-148`; `var _ Executor = (*BaseExecutor)(nil)` compile-time assertion at `base_test.go:756`.
- AC-002 verified: `TestBaseExecutor_BuildUrl` — 5 sub-cases covering stream/non-stream, explicit paths, and fallback URL index.
- AC-003 verified: `TestBaseExecutor_ShouldRetry` table-driven covers 429/500/502/503/504/4xx/2xx; `BackoffDelay` exported & tested for 250ms→16s exponential schedule.
- AC-004 verified: `TestBaseExecutor_ParseError_StatusCodes` — 13 sub-cases including 400/401/403/404/429/500/502/503/504/418/0.
- AC-005 verified: `TestBaseExecutor_Execute_ContextCancellation` — server blocks, ctx times out, returns CodeTimeout/CodeCanceled.
- AC-006 verified: `TestBaseExecutor_BuildHeaders_Bearer`, `_CustomAuthHeader`, `_RequestHeadersWin`, `_NilCreds`; `TestBaseExecutor_BuildUrl` (5 cases); `TestBaseExecutor_TransformRequest_Noop`; `TestBaseExecutor_ShouldRetry` (12 cases).
- AC-007 verified: `TestBaseExecutor_ShouldRetry` is table-driven across 12 status codes.
- All tests pass: `go test -race ./internal/executor/` → ok in 2.5s.
- `go vet ./internal/executor/` → clean.

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/executor/base.go, internal/executor/base_test.go
- Test runtime: ~1.5s sequential, ~2.5s with -race
