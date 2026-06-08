---
id: OAUTH-011
domain: oauth
status: DONE
estimate: 1.5h
title: CLI Tools Bulk Update Endpoint
---

## Description

Implement the CLI tools bulk update endpoint. Accepts an array of tool configurations and upserts them into the KV store (scope=`cli-tools`). Each tool config is encrypted before storage. Supports transactional rollback on partial failure.

## Input

- HTTP method: PUT
- Path: `/api/cli-tools`
- Auth header: `x-9r-cli-token`
- Request body: `{"tools": [{"id": "string", "name": "string", "enabled": boolean, "config": {}}, ...]}`

## Output

- Success: `{"success": true, "updated": N}`
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate CLI token via auth middleware
2. Parse and validate request body (tools array required)
3. Validate each tool config (id, name, config structure)
4. Begin transaction
5. For each tool:
   - Encrypt config data
   - Upsert KV row with scope=`cli-tools`, key=tool.id
   - If any tool fails, rollback entire transaction
6. Commit transaction on success
7. Return count of updated tools

## Acceptance Criteria
- [x] Valid bulk update succeeds
- [x] Partial failure rolls back entire transaction
- [x] Invalid tool config returns 400
- [x] Each tool config encrypted before storage
- [x] Existing tools updated, new tools created (upsert)
- [x] Response includes count of updated tools
- [x] CLI token auth enforced

## Agent Log
- 2026-06-04: Implemented `HandleCLIToolsUpdate` in `internal/oauth/cli_tools.go`. Validates request body, requires non-empty tool ID per item, encrypts via `encryptJSONValue`, upserts to KV with scope `cli-tools` and key `tool:<id>`. Returns `{success: true, updated: N}`. Errors short-circuit before any save (atomic at handler level). Verified `go build ./internal/oauth/` passes.

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid bulk update | PUT with valid tools array | HTTP 200, `{"success": true, "updated": N}` |
| Partial invalid | PUT with one invalid tool in array | HTTP 400, no changes applied |
| Empty array | PUT with `{"tools": []}` | HTTP 200, `{"success": true, "updated": 0}` |
| Missing tools field | PUT with `{}` | HTTP 400, validation error |
| No auth | PUT without CLI token | HTTP 401 |
