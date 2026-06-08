# OAuth & Integration Domain — Task Breakdown

**Domain:** OAuth & Integration
**Criticality:** P2 (Secondary integration layer — auth helpers, MCP, CLI tools, translator backend, media providers)
**Manifest section:** Phase 2.5 (OAuth & Integration, 3 days)

**Scope coverage:**
- OAuth endpoints per provider: GitLab PAT, Cursor, Kiro, Codex, iFlow
- MCP server integration routes
- CLI tools settings CRUD
- Translator backend (UI helpers)
- Media providers TTS/STT voices lookup

**Total tasks:** 21 atomic tasks across 5 sub-areas.

---

## Task Index

### Sub-area A: OAuth Endpoints (OAUTH-A)
- [OAUTH-001](#oauth-001) Generic OAuth framework (provider/action router)
- [OAUTH-002](#oauth-002) GitLab PAT exchange (`/api/oauth/gitlab/pat`)
- [OAUTH-003](#oauth-003) Cursor auto-import (`/api/oauth/cursor/auto-import`)
- [OAUTH-004](#oauth-004) Cursor import (`/api/oauth/cursor/import`)
- [OAUTH-005](#oauth-005) Kiro social authorize (`/api/oauth/kiro/social-authorize`)
- [OAUTH-006](#oauth-006) Kiro social exchange (`/api/oauth/kiro/social-exchange`)
- [OAUTH-007](#oauth-007) Kiro auto-import and import (`/api/oauth/kiro/auto-import`, `/api/oauth/kiro/import`)
- [OAUTH-008](#oauth-008) Codex import token (`/api/oauth/codex/import-token`)
- [OAUTH-009](#oauth-009) iFlow cookie (`/api/oauth/iflow/cookie`)

### Sub-area B: MCP Server Integration (MCP-A)
- [MCP-001](#mcp-001) MCP plugin message handler (`/api/mcp/[plugin]/message`)
- [MCP-002](#mcp-002) MCP plugin SSE handler (`/api/mcp/[plugin]/sse`)

### Sub-area C: CLI Tools Settings (CLI-A)
- [CLI-001](#cli-001) CLI tools list endpoint (`GET /api/cli-tools`)
- [CLI-002](#cli-002) CLI tools update endpoint (`PUT /api/cli-tools`)
- [CLI-003](#cli-003) CLI tools individual item endpoints (`GET/PUT/DELETE /api/cli-tools/[id]`)

### Sub-area D: Translator Backend (TRANSLATOR-A)
- [TRANSLATOR-001](#translator-001) Translator UI state fetch (`GET /api/translator/state`)
- [TRANSLATOR-002](#translator-002) Translator UI config save (`PUT /api/translator/config`)
- [TRANSLATOR-003](#translator-003) Format detection endpoint (`POST /api/translator/detect`)

### Sub-area E: Media Providers (MEDIA-A)
- [MEDIA-001](#media-001) TTS provider voices list (`GET /api/media-providers/tts/[provider]/voices`)
- [MEDIA-002](#media-002) STT provider voices list (`GET /api/media-providers/stt/[provider]/voices`)
- [MEDIA-003](#media-003) Media provider health check (`GET /api/media-providers/[provider]/status`)

---

## Sub-area A: OAuth Endpoints

### OAUTH-001
**Description:** Generic OAuth framework / router for `/api/oauth/[provider]/[action]`. Base handler that dispatches to provider-specific logic based on URL params. Returns 404 for unknown provider/action combos. All OAuth endpoints return JSON responses and use CLI token auth.

**Input/Output Contract:**
- Input: `GET/POST /api/oauth/:provider/:action` (Fiber params)
- Output: JSON response (provider-specific format) or error
- Auth: CLI token (`x-9r-cli-token` header)

**Test Strategy:**
- Unit: Known provider/action combos route correctly
- Unit: Unknown provider returns 404
- Unit: Unknown action returns 404
- Contract: Response format matches Node.js endpoint behavior

**Dependencies:** Auth middleware (Phase 1.4)

---

### OAUTH-002
**Description:** GitLab Personal Access Token exchange endpoint. Exchanges a GitLab authorization code for a PAT, then stores the credential in `providerConnections` table with `provider=gitlab`, `authType=pat`.

**Input/Output Contract:**
- Input: `POST /api/oauth/gitlab/pat` with body `{code, redirect_uri}`
- Output: `{"success": true, "connectionId": "..."}` or error
- Side effect: Creates/updates `providerConnections` row with encrypted token

**Test Strategy:**
- Unit: Valid code exchanges successfully
- Unit: Invalid code returns 400
- Unit: Duplicate PAT updates existing record
- Integration: Verify `providerConnections` row written correctly
- Mock: Mock GitLab token endpoint

**Dependencies:** OAUTH-001, ProviderConnection repository

---

### OAUTH-003
**Description:** Cursor IDE auto-import flow. Initiates the OAuth/PAT import process for Cursor IDE credentials. May initiate a background import job.

**Input/Output Contract:**
- Input: `GET/POST /api/oauth/cursor/auto-import`
- Output: `{"status": "initiated", "jobId": "..."}` or error
- Auth: CLI token

**Test Strategy:**
- Unit: Endpoint returns expected status
- Unit: Background job is created
- Contract: Matches Node.js behavior for Cursor import initiation

**Dependencies:** OAUTH-001

---

### OAUTH-004
**Description:** Cursor IDE import confirmation. Completes the Cursor import flow, storing credentials in `providerConnections` table with `provider=cursor`.

**Input/Output Contract:**
- Input: `POST /api/oauth/cursor/import` with body `{token, metadata}`
- Output: `{"success": true, "connectionId": "..."}` or error
- Side effect: Creates/updates `providerConnections` row

**Test Strategy:**
- Unit: Valid token stored correctly
- Unit: Invalid token returns 400
- Integration: Verify stored credential is encrypted

**Dependencies:** OAUTH-001, ProviderConnection repository

---

### OAUTH-005
**Description:** Kiro social authorize endpoint. Initiates the OAuth2 authorization code flow for Kiro CodeWhisperer (social account linking).

**Input/Output Contract:**
- Input: `GET /api/oauth/kiro/social-authorize` (query params: `client_id, redirect_uri, state`)
- Output: Redirect to Kiro authorization URL, or `{"authUrl": "..."}` for SPA clients
- Auth: CLI token

**Test Strategy:**
- Unit: Auth URL generated correctly with all query params
- Unit: State parameter is CSRF-protected (random string, stored in session/KV)
- Unit: Unknown client_id returns 400
- Integration: Redirect flow tested end-to-end with mock Kiro

**Dependencies:** OAUTH-001, KV repository (state storage)

---

### OAUTH-006
**Description:** Kiro social exchange endpoint. Exchanges the authorization code from Kiro for access/refresh tokens, then stores in `providerConnections` with `provider=kiro`, `authType=oauth`. Handles token refresh logic.

**Input/Output Contract:**
- Input: `POST /api/oauth/kiro/social-exchange` with body `{code, state, redirect_uri}`
- Output: `{"success": true, "connectionId": "..."}` or error
- Side effect: Creates/updates `providerConnections` with encrypted tokens + expiresAt
- Token refresh: Proactively refreshes before expiry (similar to CHAT-D token expiry checker)

**Test Strategy:**
- Unit: Valid code exchanges successfully
- Unit: Invalid code returns 401
- Unit: State mismatch returns 400 (CSRF protection)
- Unit: Token refresh triggered when expiresAt is within threshold
- Integration: KV state consumed and deleted after use

**Dependencies:** OAUTH-001, OAUTH-005, ProviderConnection repository, KV repository

---

### OAUTH-007
**Description:** Kiro auto-import and import endpoints. Handles PAT-based import for Kiro (alternative to OAuth flow). Stores Kiro credentials in `providerConnections` with `provider=kiro`.

**Input/Output Contract:**
- Input: `GET /api/oauth/kiro/auto-import`, `POST /api/oauth/kiro/import`
- Output: `{"status": "initiated", "jobId": "..."}` for auto-import; `{"success": true, "connectionId": "..."}` for import
- Auth: CLI token

**Test Strategy:**
- Unit: Import stores credential correctly
- Unit: Auto-import initiates background job
- Contract: Matches Node.js Kiro import behavior

**Dependencies:** OAUTH-001, ProviderConnection repository

---

### OAUTH-008
**Description:** Codex import token endpoint. Accepts an OpenAI Codex token and stores it as a provider connection.

**Input/Output Contract:**
- Input: `POST /api/oauth/codex/import-token` with body `{token, label}`
- Output: `{"success": true, "connectionId": "..."}` or error
- Side effect: Creates/updates `providerConnections` with `provider=codex`

**Test Strategy:**
- Unit: Valid token stored correctly
- Unit: Token validation (format check) before storage
- Unit: Missing label uses default
- Integration: Verify encrypted storage

**Dependencies:** OAUTH-001, ProviderConnection repository

---

### OAUTH-009
**Description:** iFlow cookie authentication endpoint. Exchanges iFlow credentials/cookies for stored provider connection.

**Input/Output Contract:**
- Input: `POST /api/oauth/iflow/cookie` with body `{cookies, domain}`
- Output: `{"success": true, "connectionId": "..."}` or error
- Side effect: Creates/updates `providerConnections` with `provider=iflow`, `authType=cookie`

**Test Strategy:**
- Unit: Valid cookie stored correctly
- Unit: Cookie format validation
- Integration: iFlow provider connection usable by iflow executor

**Dependencies:** OAUTH-001, ProviderConnection repository

---

## Sub-area B: MCP Server Integration

### MCP-001
**Description:** MCP plugin message handler (JSON-RPC over HTTP). Routes incoming MCP messages to the appropriate plugin's handler. Plugins are dynamically registered. Returns JSON-RPC 2.0 responses.

**Input/Output Contract:**
- Input: `POST /api/mcp/:plugin/message` with JSON-RPC 2.0 body `{jsonrpc, method, params, id}`
- Output: JSON-RPC 2.0 response `{jsonrpc, result, id}` or error `{jsonrpc, error, id}`
- Auth: CLI token
- Plugin resolution: Look up registered plugin by name from settings/plugin registry

**Test Strategy:**
- Unit: Valid JSON-RPC request routed to plugin
- Unit: Unknown plugin returns `-32601 Method not found`
- Unit: Malformed JSON-RPC returns `-32700 Parse error`
- Unit: Plugin returns correct JSON-RPC result structure
- Integration: Real plugin registered and message delivered

**Dependencies:** Plugin registry (shared with executor), Auth middleware

---

### MCP-002
**Description:** MCP plugin SSE handler for streaming responses. Routes SSE connections to the appropriate plugin. Plugin sends events via SSE; Go server proxies to client.

**Input/Output Contract:**
- Input: `GET /api/mcp/:plugin/sse` with query params
- Output: SSE stream (`Content-Type: text/event-stream`)
- Auth: CLI token
- Connection lifecycle: Plugin notified on connect/disconnect

**Test Strategy:**
- Unit: SSE connection established with correct headers
- Unit: Plugin receives connect notification
- Unit: Events streamed to client
- Unit: Client disconnect triggers plugin cleanup
- Unit: Unknown plugin returns 404
- Integration: End-to-end SSE with mock plugin

**Dependencies:** MCP-001, SSE streaming utility (`pkg/response/sse.go`)

---

## Sub-area C: CLI Tools Settings

### CLI-001
**Description:** List all CLI tool settings. Reads tool configurations from KV store (scope=`cli-tools`) and returns as an array.

**Input/Output Contract:**
- Input: `GET /api/cli-tools`
- Output: `{"tools": [{"id": "...", "name": "...", "enabled": true, "config": {...}}, ...]}`
- Auth: CLI token

**Test Strategy:**
- Unit: Empty KV returns empty array
- Unit: All tools returned correctly
- Unit: Config decrypted for display
- Integration: Real KV store read

**Dependencies:** KV repository

---

### CLI-002
**Description:** Bulk update CLI tool settings. Accepts an array of tool configs and upserts them into the KV store (scope=`cli-tools`).

**Input/Output Contract:**
- Input: `PUT /api/cli-tools` with body `{"tools": [...]}`
- Output: `{"success": true, "updated": N}`
- Side effect: Each tool config encrypted and stored as KV row

**Test Strategy:**
- Unit: Valid update succeeds
- Unit: Partial failure rolls back (transaction)
- Unit: Invalid tool config returns 400
- Integration: Verify KV rows written correctly

**Dependencies:** KV repository

---

### CLI-003
**Description:** Individual CLI tool CRUD. Single tool get/update/delete operations for fine-grained control.

**Input/Output Contract:**
- Input: `GET/PUT/DELETE /api/cli-tools/:id`
- Output: GET returns `{tool: {...}}`, PUT returns `{success: true}`, DELETE returns `{success: true}`
- Auth: CLI token

**Test Strategy:**
- Unit: GET existing tool returns config
- Unit: GET non-existent tool returns 404
- Unit: PUT updates and returns updated config
- Unit: DELETE removes and returns success
- Integration: Real KV operations

**Dependencies:** CLI-001

---

## Sub-area D: Translator Backend

### TRANSLATOR-001
**Description:** Fetch translator UI state. Returns the current translator configuration and state (active format, enabled translators, per-translator settings).

**Input/Output Contract:**
- Input: `GET /api/translator/state`
- Output: `{"format": "openai", "translators": {...}, "settings": {...}}`
- Auth: JWT or CLI token
- Source: Reads from `settings` table (JSON `data` column, key=`translator`)

**Test Strategy:**
- Unit: Default state returned when no config saved
- Unit: Saved config returned correctly
- Unit: Config parsed without error
- Integration: Matches frontend translator UI expectations

**Dependencies:** Settings repository

---

### TRANSLATOR-002
**Description:** Save translator UI configuration. Stores translator settings to the `settings` table.

**Input/Output Contract:**
- Input: `PUT /api/translator/config` with body `{format, translators, settings}`
- Output: `{"success": true}`
- Side effect: Settings JSON saved to `settings` table (upsert by ID=1)

**Test Strategy:**
- Unit: Valid config saved successfully
- Unit: Partial config merges with existing
- Unit: Invalid config returns 400
- Integration: Verify settings row updated in DB

**Dependencies:** Settings repository

---

### TRANSLATOR-003
**Description:** Auto-detect request format endpoint. Accepts a sample request body and returns the detected format (openai, anthropic, gemini, etc.). Used by translator UI for preview.

**Input/Output Contract:**
- Input: `POST /api/translator/detect` with body `{sample: {...}}`
- Output: `{"format": "openai", "confidence": 0.95}`

**Test Strategy:**
- Unit: OpenAI format detected correctly
- Unit: Anthropic format detected correctly
- Unit: Gemini format detected correctly
- Unit: Unknown format returns `{format: "unknown", confidence: 0}`
- Unit: Edge cases (empty body, malformed JSON) handled gracefully

**Dependencies:** Format autodetect translator (internal/translator/autodetect.go)

---

## Sub-area E: Media Providers

### MEDIA-001
**Description:** TTS provider voices list. Returns available voices from the specified TTS provider (e.g., OpenAI TTS, Azure TTS, ElevenLabs, etc.). Caches the voice list in-memory with TTL.

**Input/Output Contract:**
- Input: `GET /api/media-providers/tts/:provider/voices`
- Output: `{"provider": "openai", "voices": [{"id": "alloy", "name": "Alloy", "language": "en"}]}`
- Auth: JWT or CLI token
- Caching: Voices cached for 5 minutes per provider

**Test Strategy:**
- Unit: OpenAI voices returned correctly
- Unit: Unknown provider returns 404
- Unit: Cache hit returns without provider call
- Unit: Cache miss fetches and caches
- Integration: Real provider API called (or mocked)

**Dependencies:** HTTP client for provider API, in-memory cache

---

### MEDIA-002
**Description:** STT provider voices list (for consistency with TTS naming, though STT uses "models" or "languages"). Returns available models/languages from the specified STT provider.

**Input/Output Contract:**
- Input: `GET /api/media-providers/stt/:provider/models` (or `voices` for API consistency)
- Output: `{"provider": "openai", "models": [{"id": "whisper-1", "name": "Whisper"}]}`

**Test Strategy:**
- Unit: OpenAI Whisper model returned correctly
- Unit: Unknown provider returns 404
- Integration: Matches TTS pattern (OAUTH-001)

**Dependencies:** HTTP client, in-memory cache

---

### MEDIA-003
**Description:** Media provider health check. Pings the media provider API to verify connectivity and returns status.

**Input/Output Contract:**
- Input: `GET /api/media-providers/:provider/status`
- Output: `{"provider": "openai", "status": "healthy", "latency_ms": 45}` or `{"status": "unhealthy", "error": "..."}`
- Timeout: 5 second timeout per provider

**Test Strategy:**
- Unit: Healthy provider returns `status: "healthy"`
- Unit: Unreachable provider returns `status: "unhealthy"`
- Unit: Timeout returns appropriate error
- Performance: All providers checked in parallel

**Dependencies:** HTTP client

---

## Summary

| Sub-area | Tasks | Criticality | Est. Effort |
|---|---|---|---|
| A: OAuth Endpoints | 9 | P2 | 1.5 days |
| B: MCP Integration | 2 | P2 | 0.5 days |
| C: CLI Tools Settings | 3 | P2 | 0.5 days |
| D: Translator Backend | 3 | P3 | 0.25 days |
| E: Media Providers | 3 | P3 | 0.25 days |
| **Total** | **20** | | **3 days** |

**Cross-cutting concerns:**
- All endpoints use CLI token auth (`x-9r-cli-token`) per manifest auth rules
- OAuth endpoints may need KV store for CSRF state (OAUTH-005)
- Provider credentials stored in `providerConnections` with encrypted `Data` JSON
- Media provider endpoints share HTTP client + cache patterns
- Error responses must match Node.js format byte-for-byte for frontend compatibility
