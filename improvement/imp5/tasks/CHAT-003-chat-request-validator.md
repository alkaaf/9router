---
id: CHAT-003
domain: chat-core
status: DONE
estimate: 1h
title: Chat Request Validator
---

## Description

Validate API key against the `requireApiKey` setting. If enabled, the request must have a valid key from the `apiKeys` table. Returns 401 if missing or invalid. Bypassed when `requireApiKey=false` (local mode).

## Input

- `string` (API key)
- `bool` (requireApiKey setting)
- ApiKey repository for database lookup

## Output

- `error` (nil if valid, error with status code otherwise)
- 401 "Missing API key" or "Invalid API key"

## Logic

1. If `requireApiKey=false`, return nil immediately
2. If API key is empty, return 401 "Missing API key"
3. Query `ApiKey` table by `key` column
4. If not found or `isActive=false`, return 401 "Invalid API key"
5. Return nil on success

## Agent Log
- Started: 2026-06-04 18:00
- Completed: 2026-06-04 18:15
- Agent: agent-chat
- AC-001 verified: TestAPIKeyValidator_ValidKey — valid key passes
- AC-002 verified: TestAPIKeyValidator_MissingKey — "Missing API key" error
- AC-003 verified: TestAPIKeyValidator_InvalidKey — "Invalid API key" error
- AC-004 verified: TestAPIKeyValidator_RequireFalse — short-circuits without DB hit
- AC-005 verified: TestAPIKeyValidator_Disabled + TestAPIKeyValidator_NilIsActive — both rejected

## Acceptance Criteria
- [x] Valid key + requireApiKey=true passes
- [x] Missing key + requireApiKey=true returns 401 "Missing API key"
- [x] Invalid key + requireApiKey=true returns 401 "Invalid API key"
- [x] requireApiKey=false always passes
- [x] Inactive key (isActive=false) returns 401

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/apikey.go
- Tests: internal/chatcore/apikey_test.go (7 tests, all pass)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid key, requireApiKey=true | `sk-valid`, true | nil error |
| Missing key, requireApiKey=true | ``, true | 401 "Missing API key" |
| Invalid key, requireApiKey=true | `sk-invalid`, true | 401 "Invalid API key" |
| requireApiKey=false | any, false | nil error |
| Inactive key | `sk-inactive`, true | 401 "Invalid API key" |
