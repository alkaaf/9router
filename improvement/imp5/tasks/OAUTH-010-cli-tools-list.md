---
id: OAUTH-010
domain: oauth
status: DONE
estimate: 1h
title: CLI Tools List Endpoint
---

## Description

Implement the CLI tools list endpoint. Reads tool configurations from the KV store (scope=`cli-tools`) and returns them as an array. Handles decryption of sensitive config fields for display.

## Input

- HTTP method: GET
- Path: `/api/cli-tools`
- Auth header: `x-9r-cli-token`

## Output

- Success: `{"tools": [{"id": "string", "name": "string", "enabled": boolean, "config": {}}, ...]}`
- Empty: `{"tools": []}`

## Logic

1. Validate CLI token via auth middleware
2. Query KV store for all entries with scope=`cli-tools`
3. For each entry:
   - Decrypt the stored config data
   - Parse JSON config
   - Build tool object with id, name, enabled, config
4. Return array of tool objects
5. Return empty array if no tools configured

## Acceptance Criteria
- [x] Returns all configured CLI tools
- [x] Empty KV store returns empty array
- [x] Config fields decrypted for display
- [x] Response structure matches expected format
- [x] CLI token auth enforced
- [x] Handles malformed config gracefully

## Agent Log
- 2026-06-04: Implemented `HandleCLIToolsList` in `internal/oauth/cli_tools.go`. Reads KV entries with scope `cli-tools`, decodes via `parseStoredCLITool`, applies `decryptValue` for config decryption. Returns `{tools: [...]}` or `{tools: []}`. Malformed entries skipped gracefully. `KVListing` interface allows list operation. Verified `go build ./internal/oauth/` passes.

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Tools configured | GET with tools in KV store | HTTP 200, `{"tools": [...]}` |
| No tools | GET with empty KV store | HTTP 200, `{"tools": []}` |
| Encrypted config | GET with encrypted tool configs | HTTP 200, decrypted configs in response |
| No auth | GET without CLI token | HTTP 401 |
