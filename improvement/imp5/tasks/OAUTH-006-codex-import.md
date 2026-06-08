---
id: OAUTH-006
domain: oauth
status: DONE
estimate: 1.5h
title: Codex Import Token Endpoint
---

## Acceptance Criteria
- [x] Valid token stored correctly in providerConnections
- [x] Token format validated before storage
- [x] Missing label uses default value
- [x] Invalid token format returns 400
- [x] Token encrypted before storage
- [x] Duplicate imports update existing record
- [x] Response includes connectionId
- [x] Error responses match Node.js format

## Agent Log
- Started: 2026-06-04 16:20
- Completed: 2026-06-04 16:40
- Agent: agent-oauth
- AC-001 verified: TestHandleCodexImportToken_ValidToken — stored codexData with token/label
- AC-002 verified: codex_import.go calls codexValidator before storage
- AC-003 verified: TestHandleCodexImportToken_DefaultLabel — missing label defaults to "Codex"
- AC-004 verified: TestHandleCodexImportToken_InvalidFormat — invalid token returns INVALID_TOKEN
- AC-005 verified: encryptJSONValue called before Upsert
- AC-006 verified: upsertProviderConnection (shared) merges into existing row
- AC-007 verified: codexImportResponse includes connectionId
- AC-008 verified: error envelope via oauthErrorResponse

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/oauth/codex_import.go

## Description

Implement the OpenAI Codex token import endpoint. Accepts a Codex API token with an optional label, validates the token format, and stores it as a provider connection in `providerConnections` with `provider=codex`.

## Input

- HTTP method: POST
- Path: `/api/oauth/codex/import-token`
- Auth header: `x-9r-cli-token`
- Request body: `{ "token": "string", "label": "string (optional)" }`

## Output

- Success: `{"success": true, "connectionId": "uuid"}`
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate CLI token via auth middleware
2. Parse request body; validate `token` field present
3. Validate token format (Codex/OpenAI token pattern check)
4. Use default label if not provided
5. Encrypt the token
6. Upsert `providerConnections` row with:
   - `provider` = "codex"
   - `Data` = encrypted JSON containing token, label
7. Return connection ID

## Acceptance Criteria
- [ ] Valid token stored correctly in providerConnections
- [ ] Token format validated before storage
- [ ] Missing label uses default value
- [ ] Invalid token format returns 400
- [ ] Token encrypted before storage
- [ ] Duplicate imports update existing record
- [ ] Response includes connectionId
- [ ] Error responses match Node.js format

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid token import | POST with `{token: "sk-...", label: "my-codex"}` | HTTP 200, `{"success": true, "connectionId": "..."}` |
| Missing label | POST with `{token: "sk-..."}` | HTTP 200, default label used |
| Invalid token format | POST with `{token: "invalid"}` | HTTP 400 |
| Missing token | POST with `{label: "test"}` | HTTP 400 |
| No auth | POST without CLI token | HTTP 401 |
