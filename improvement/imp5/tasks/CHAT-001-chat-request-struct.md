---
id: CHAT-001
domain: chat-core
status: DONE
estimate: 1h
title: Chat Request Struct
---

## Description

JSON body parser that wraps Fiber context body parsing with explicit error handling. Must distinguish between malformed JSON (400) and empty body (400) with clear error messages matching Node.js behavior.

## Input

- `*fiber.Ctx` with raw request body

## Output

- Parsed `map[string]any` body struct OR error
- Error: 400 `{"error": {"message": "Invalid JSON body", "type": "invalid_request_error"}}`

## Logic

1. Call Fiber body parser
2. Check if body is nil or empty → return 400 "Invalid JSON body"
3. Check if body is array (not object) → return 400 "Invalid JSON body"
4. Cast to map[string]any
5. Return parsed body or parsing error

## Agent Log
- Started: 2026-06-04 17:30
- Completed: 2026-06-04 17:45
- Agent: agent-chat
- AC-001 verified: TestParseRequest_MalformedJSON + TestHandler_MalformedJSON — returns 400 with "Invalid JSON body"
- AC-002 verified: TestParseRequest_EmptyBody + TestHandler_EmptyBody — returns 400 with "Invalid JSON body"
- AC-003 verified: TestParseRequest_ArrayBody + TestHandler_ArrayBody — returns 400 (handled as ErrInvalidJSON)
- AC-004 verified: TestParseRequest_ValidJSON + TestHandler_ValidJSON — returns 200 with parsed body
- AC-005 verified: TestErrorBodyFormat — error envelope {error:{message,type,code}} matches open-sse/utils/error.js

## Acceptance Criteria
- [x] Malformed JSON returns 400 with proper error body
- [x] Empty body returns 400 with proper error body
- [x] Array body (non-object) returns 400
- [x] Valid JSON object parses correctly
- [x] Error body matches Node.js response byte-for-byte

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/request.go, internal/chatcore/error.go, internal/handler/v1/chat.go
- Tests: internal/chatcore/request_test.go (10 tests), internal/handler/v1/chat_test.go (6 tests)
- Notes: Spec mentioned Fiber (*fiber.Ctx) but the project uses net/http. Adapted the implementation to net/http to match existing patterns (internal/handler/api/oauth.go).

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid JSON | `{"model": "gpt-4", "messages": []}` | Parsed map[string]any |
| Malformed JSON | `{"model": "gpt-4"` | 400 error |
| Empty body | `` | 400 error |
| Array body | `[]` | 400 error |
| Null body | `null` | 400 error |
