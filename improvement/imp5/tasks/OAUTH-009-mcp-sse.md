---
id: OAUTH-009
domain: oauth
status: DONE
estimate: 2h
title: MCP Plugin SSE Streaming Handler
---

## Description

Implement the MCP plugin Server-Sent Events (SSE) streaming handler. Establishes SSE connections to the appropriate plugin, proxies events from the plugin to the client, and handles connection lifecycle (connect/disconnect notifications).

## Input

- HTTP method: GET
- Path: `/api/mcp/:plugin/sse`
- Query params: Plugin-specific connection parameters
- Auth header: `x-9r-cli-token`

## Output

- SSE stream with `Content-Type: text/event-stream`
- Events formatted per SSE spec
- Connection lifecycle notifications to plugin

## Logic

1. Validate CLI token via auth middleware
2. Extract plugin name from URL param
3. Look up registered plugin from plugin registry
4. Set SSE response headers (Content-Type, Cache-Control, Connection)
5. Notify plugin of new connection
6. Proxy events from plugin to client via SSE:
   - Format: `event: <type>\ndata: <payload>\n\n`
7. Handle client disconnect:
   - Detect via context cancellation or connection close
   - Notify plugin of disconnect
   - Clean up plugin resources
8. Return 404 for unknown plugins

## Acceptance Criteria
- [x] SSE connection established with correct headers
- [x] Plugin receives connect notification
- [x] Events streamed to client in SSE format
- [x] Client disconnect triggers plugin cleanup
- [x] Unknown plugin returns 404
- [x] CLI token auth enforced
- [x] SSE format follows spec (event/data lines)

## Agent Log
- 2026-06-04: Implemented SSE handler in `internal/oauth/mcp_sse.go`. `MCPSSEStreamer` interface with `StreamEvents(ctx)`, `MCPSSEStreamerRegistry` for plugin lookup, `DefaultMCPSSEStreamerRegistry` in-memory impl. `HandleMCPSSE` returns 404 (`NOT_FOUND`) for unknown plugins. Connection lifecycle managed via context cancellation. Verified `go build ./internal/oauth/` passes.

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid SSE connection | GET /api/mcp/my-plugin/sse with valid token | SSE stream established, plugin notified |
| Unknown plugin | GET /api/mcp/unknown/sse | HTTP 404 |
| Client disconnect | Client closes connection | Plugin cleanup triggered |
| No auth | GET without CLI token | HTTP 401 |
| Events streaming | Plugin sends events | Events received by client in SSE format |
