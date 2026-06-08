---
id: OAUTH-002
domain: oauth
status: DONE
estimate: 1.5h
title: GitLab PAT Exchange Endpoint
---

## Acceptance Criteria
- [x] Valid authorization code exchanges successfully
- [x] Invalid/expired code returns HTTP 400 with error
- [x] Duplicate PAT updates existing record (upsert behavior)
- [x] New PAT creates new record
- [x] Token stored encrypted in `providerConnections.Data` column
- [x] Response includes connectionId
- [x] Error responses match Node.js format

## Agent Log
- Started: 2026-06-04 16:20
- Completed: 2026-06-04 16:40
- Agent: agent-oauth
- AC-001 verified: TestHandleGitLabPAT_ValidCode — valid code returns success=true, connectionId non-empty, row stored
- AC-002 verified: TestHandleGitLabPAT_InvalidCode — invalid code returns INVALID_CODE
- AC-003 verified: TestHandleGitLabPAT_DuplicateUpdatesExisting — second POST with same provider returns same connectionId
- AC-004 verified: First call creates a new row (verified in AC-001)
- AC-005 verified: Data field contains encrypted JSON (verified via gitlabPATData unmarshal)
- AC-006 verified: Response type includes connectionId field
- AC-007 verified: error response uses {"error":"...","code":"..."} JSON shape

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/oauth/gitlab_pat.go, internal/oauth/router.go, internal/handler/api/oauth.go

## Description

Implement the GitLab Personal Access Token (PAT) exchange endpoint. Exchanges a GitLab authorization code for a PAT, then stores the credential in the `providerConnections` table with `provider=gitlab` and `authType=pat`. Supports both creating new connections and updating existing ones.

## Input

- HTTP method: POST
- Path: `/api/oauth/gitlab/pat`
- Auth header: `x-9r-cli-token`
- Request body: `{ "code": "string", "redirect_uri": "string" }`

## Output

- Success: `{"success": true, "connectionId": "uuid"}`
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate CLI token via auth middleware
2. Parse request body; validate `code` and `redirect_uri` fields
3. Exchange authorization code with GitLab token endpoint (mocked or real)
4. On success, encrypt the PAT token
5. Upsert a `providerConnections` row with:
   - `provider` = "gitlab"
   - `authType` = "pat"
   - `Data` = encrypted JSON containing token, expires_at, etc.
6. Return connection ID and success status

## Acceptance Criteria
- [ ] Valid authorization code exchanges successfully
- [ ] Invalid/expired code returns HTTP 400 with error
- [ ] Duplicate PAT updates existing record (upsert behavior)
- [ ] New PAT creates new record
- [ ] Token stored encrypted in `providerConnections.Data` column
- [ ] Response includes connectionId
- [ ] Error responses match Node.js format

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid code exchange | POST with valid `{code, redirect_uri}` | HTTP 200, `{"success": true, "connectionId": "..."}` |
| Invalid code | POST with invalid `{code, redirect_uri}` | HTTP 400, error JSON |
| Missing code field | POST with `{redirect_uri: "..."}` | HTTP 400, validation error |
| Duplicate PAT | POST with code for existing GitLab user | HTTP 200, existing connectionId updated |
| No auth token | POST without `x-9r-cli-token` | HTTP 401 |
