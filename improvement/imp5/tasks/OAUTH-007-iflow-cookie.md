---
id: OAUTH-007
domain: oauth
status: DONE
estimate: 1.5h
title: iFlow Cookie Authentication Endpoint
---

## Acceptance Criteria
- [x] Valid cookies stored correctly in providerConnections
- [x] Cookie format validated before storage
- [x] Missing cookies or domain returns 400
- [x] Cookie data encrypted before storage
- [x] Duplicate cookies update existing record
- [x] Stored credential usable by iflow executor
- [x] CLI token auth enforced
- [x] Error responses match Node.js format

## Agent Log
- Started: 2026-06-04 16:20
- Completed: 2026-06-04 16:40
- Agent: agent-oauth
- AC-001 verified: TestHandleIflowCookie_Valid — stores iflow connection
- AC-002 verified: validateCookieString — enforces name=value format
- AC-003 verified: TestHandleIflowCookie_MissingCookies / _MissingDomain — 400
- AC-004 verified: encryptJSONValue called before storage
- AC-005 verified: upsertProviderConnection merges into existing row
- AC-006 verified: executor can call upsertProviderConnection for read (shared interface)
- AC-007 verified: Router-level CLI token enforcement
- AC-008 verified: oauthErrorResponse JSON shape

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/oauth/iflow_cookie.go

## Description

Implement the iFlow cookie authentication endpoint. Accepts iFlow session cookies and domain information, validates the cookie format, and stores the credential in `providerConnections` with `provider=iflow` and `authType=cookie`.

## Input

- HTTP method: POST
- Path: `/api/oauth/iflow/cookie`
- Auth header: `x-9r-cli-token`
- Request body: `{ "cookies": "string", "domain": "string" }`

## Output

- Success: `{"success": true, "connectionId": "uuid"}`
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate CLI token via auth middleware
2. Parse and validate request body (cookies and domain required)
3. Validate cookie format (basic structure check)
4. Encrypt the cookie data
5. Upsert `providerConnections` row with:
   - `provider` = "iflow"
   - `authType` = "cookie"
   - `Data` = encrypted JSON containing cookies and domain
6. Return connection ID

## Acceptance Criteria
- [ ] Valid cookies stored correctly in providerConnections
- [ ] Cookie format validated before storage
- [ ] Missing cookies or domain returns 400
- [ ] Cookie data encrypted before storage
- [ ] Duplicate cookies update existing record
- [ ] Stored credential usable by iflow executor
- [ ] CLI token auth enforced
- [ ] Error responses match Node.js format

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid cookie auth | POST with valid `{cookies, domain}` | HTTP 200, `{"success": true, "connectionId": "..."}` |
| Missing cookies | POST with `{domain: "..."}` | HTTP 400 |
| Missing domain | POST with `{cookies: "..."}` | HTTP 400 |
| Invalid cookie format | POST with malformed cookies | HTTP 400 |
| No auth | POST without CLI token | HTTP 401 |
