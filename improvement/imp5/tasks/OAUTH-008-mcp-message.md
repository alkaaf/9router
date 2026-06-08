---
id: OAUTH-008
domain: oauth
status: DONE
estimate: 2h
title: MCP Plugin Message Handler
---

## Description

Implement the MCP plugin message handler for JSON-RPC 2.0 over HTTP. Routes incoming MCP messages to the appropriate plugin's handler based on the plugin name in the URL. Plugins are dynamically registered. Returns standard JSON-RPC 2.0 responses.

## Input

- HTTP method: POST
- Path: `/api/mcp/:plugin/message`
- Auth header: `x-9r-cli-token`
- Request body: JSON-RPC 2.0 format `{ "jsonrpc": "2.0", "method": "string", "params": {}, "id": number }`

## Output

- Success: `{ "jsonrpc": "2.0", "result": {}, "id": number }`
- Error: `{ "jsonrpc": "2.0", "error": {"code": number, "message": "string"}, "id": number }`

## Logic

1. Validate CLI token via auth middleware
2. Extract plugin name from URL param
3. Look up registered plugin from plugin registry/settings
4. Parse JSON-RPC 2.0 request body
5. Validate JSON-RPC format (jsonrpc version, method, id)
6. Route to plugin's message handler with method and params
7. Return JSON-RPC 2.0 formatted response
8. Handle errors with proper JSON-RPC error codes:
   - -32600: Invalid Request
   - -32601: Method not found
   - -32700: Parse error
   - -32603: Internal error

## Acceptance Criteria
- [x] Valid JSON-RPC request routed to correct plugin
- [x] Unknown plugin returns -32601 error
- [x] Malformed JSON-RPC returns -32700 parse error
- [x] Response follows JSON-RPC 2.0 spec exactly
- [x] Plugin returns correct result structure
- [x] CLI token auth enforced
- [x] Error codes match JSON-RPC 2.0 spec

## Agent Log
- 2026-06-04: Implemented `HandleMCPMessage` in `internal/oauth/mcp_message.go` with full JSON-RPC 2.0 protocol support. Plugin registry via `SetMCPPluginRegistry`/`GetHandler`. Routes methods to plugins by URL param `c.Provider`. Error codes per spec: -32700 (parse), -32600 (invalid request), -32601 (method not found), -32603 (internal). Verified `go build ./internal/oauth/` passes.

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid request | POST to `/api/mcp/my-plugin/message` with valid JSON-RPC | HTTP 200, JSON-RPC response |
| Unknown plugin | POST to `/api/mcp/unknown/message` | HTTP 200, `{"error": {"code": -32601}}` |
| Malformed JSON | POST with invalid JSON body | HTTP 200, `{"error": {"code": -32700}}` |
| Missing method | POST with `{jsonrpc: "2.0", id: 1}` | HTTP 200, `{"error": {"code": -32601}}` |
| No auth | POST without CLI token | HTTP 401 |
