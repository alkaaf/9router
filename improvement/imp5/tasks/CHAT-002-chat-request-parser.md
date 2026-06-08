---
id: CHAT-002
domain: chat-core
status: DONE
estimate: 1h
title: Chat Request Parser
---

## Agent Log
- Started: 2026-06-04 17:45
- Completed: 2026-06-04 18:00
- Agent: agent-chat
- AC-001 verified: TestExtractAPIKey_BearerToken — Bearer extracted
- AC-002 verified: TestExtractAPIKey_XAPIKey — x-api-key fallback
- AC-003 verified: TestExtractAPIKey_NoHeaders + TestExtractAPIKey_NilRequest — returns ""
- AC-004 verified: TestExtractAPIKey_BasicAuth — "Basic" not accepted
- AC-005 verified: TestExtractAPIKey_CaseInsensitive — Set then Get roundtrip; Node.js parity (lower-case "bearer" rejected — see TestExtractAPIKey_LowercaseBearer)

## Description

Extract API key from request headers. Mirrors Node.js `extractApiKey()` — checks `Authorization: Bearer <key>` first, then `x-api-key` header (Anthropic format). Returns empty string if neither is present.

## Input

- `*fiber.Ctx`

## Output

- `string` (API key) or empty string
- Header priority: `Authorization: Bearer` > `x-api-key`

## Logic

1. Check `Authorization` header
2. If present and starts with `Bearer `, extract token after prefix
3. Else check `x-api-key` header
4. Return extracted key or empty string

## Acceptance Criteria
- [x] Bearer token extracted correctly from Authorization header
- [x] x-api-key header extracted correctly as fallback
- [x] No auth header returns empty string
- [x] Authorization: Basic returns empty string (only Bearer supported)
- [x] Header matching is case-insensitive (Go headers are canonical)

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/auth.go
- Tests: internal/chatcore/auth_test.go (9 tests, all pass)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Bearer token | `Authorization: Bearer sk-123` | `sk-123` |
| x-api-key only | `x-api-key: sk-456` | `sk-456` |
| Both headers | Bearer + x-api-key | Bearer value (priority) |
| No auth | No auth headers | empty string |
| Basic auth | `Authorization: Basic abc` | empty string |
| Empty Bearer | `Authorization: Bearer ` | empty string |
