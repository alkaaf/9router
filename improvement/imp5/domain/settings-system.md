# Settings & System Domain — Atomic Task Breakdown

## Scope Coverage

This domain covers all settings, system, model management, and infrastructure configuration endpoints for the Go backend rewrite.

**Source-of-truth files analyzed:**
- `src/app/api/settings/route.js` — GET/PATCH app settings, password hash, OIDC config, outbound proxy env
- `src/app/api/settings/database/route.js` — DB export (GET) / import (POST)
- `src/app/api/settings/proxy-test/route.js` — Proxy URL connectivity test (POST)
- `src/app/api/settings/require-login/route.js` — GET auth/access settings
- `src/app/api/health/route.js` — Lightweight health check
- `src/app/api/init/route.js` — Initialization marker (GET, returns "Initialized")
- `src/app/api/shutdown/route.js` — Dev-only POST shutdown (Bearer token auth)
- `src/app/api/version/route.js` — GET current + latest npm version
- `src/app/api/version/update/route.js` — POST auto-update (production only, kills locks)
- `src/app/api/version/shutdown/route.js` — POST shutdown for manual update
- `src/app/api/locale/route.js` — POST set locale cookie (1-year max-age)
- `src/app/api/tags/route.js` — GET list of Ollama model tags (CORS-enabled)
- `src/app/api/pricing/route.js` — GET/PATCH/DELETE pricing config
- `src/app/api/models/route.js` — GET/PUT model list with aliases (legacy alias set)
- `src/app/api/models/alias/route.js` — GET/PUT/DELETE model alias CRUD
- `src/app/api/models/custom/route.js` — GET/POST/DELETE custom model entries
- `src/app/api/models/disabled/route.js` — GET/POST/DELETE disabled-model per provider
- `src/app/api/models/availability/route.js` — GET model cooldowns/unavailable; POST clear cooldown
- `src/app/api/models/test/route.js` — POST ping model via internal completions or embeddings
- `src/app/api/provider-nodes/route.js` — GET/POST provider node list and create
- `src/app/api/provider-nodes/[id]/route.js` — PUT/DELETE provider node update and delete
- `src/app/api/provider-nodes/validate/route.js` — POST validate API key against base URL
- `src/app/api/proxy-pools/route.js` — GET/POST proxy pool list and create
- `src/app/api/proxy-pools/[id]/route.js` — GET/PUT/DELETE single proxy pool
- `src/app/api/proxy-pools/cloudflare-deploy/route.js` — POST deploy Cloudflare Workers relay
- `src/app/api/proxy-pools/vercel-deploy/route.js` — POST deploy Vercel Edge relay
- `src/app/api/proxy-pools/deno-deploy/route.js` — POST deploy Deno Deploy relay

**Storage layer** (via `src/lib/localDb.js` → `src/lib/db/index.js`):
- `settings` table — single-row JSON, fields: password, oidcIssuerUrl, oidcClientId, oidcClientSecret, requireLogin, outboundProxyEnabled, outboundProxyUrl, outboundNoProxy, comboStrategy, comboStickyRoundRobinLimit, comboStrategies, tunnelDashboardAccess, tunnelUrl, tailscaleUrl
- `kv` table — key-value store for model aliases, disabled models, custom models, pricing overrides
- Provider connections and proxy pools via their own tables

---

## High-Level Architecture

```
Client (Dashboard / Admin UI)
  │
  ├─ GET/PATCH  /api/settings          → settings row (secret redaction)
  ├─ GET/POST   /api/settings/database  → full DB export / import
  ├─ POST       /api/settings/proxy-test → test proxy connectivity
  ├─ GET        /api/settings/require-login → auth/access flags
  │
  ├─ GET        /api/health             → { ok: true }
  ├─ GET        /api/init               → "Initialized" (app startup ping)
  ├─ GET        /api/version            → current + latest npm version
  ├─ POST       /api/version/update     → auto-update (production)
  ├─ POST       /api/version/shutdown   → shutdown for manual update
  ├─ POST       /api/shutdown           → dev-only graceful exit
  ├─ POST       /api/locale             → set locale cookie
  ├─ GET        /api/tags               → Ollama model tags
  │
  ├─ GET/PATCH/DELETE  /api/pricing     → pricing config CRUD
  │
  ├─ GET/PUT    /api/models             → model list with aliases (legacy)
  ├─ GET/PUT/DELETE /api/models/alias    → alias CRUD
  ├─ GET/POST/DELETE /api/models/custom  → custom model CRUD
  ├─ GET/POST/DELETE /api/models/disabled → per-provider disabled models
  ├─ GET/POST    /api/models/availability → cooldown/unavailable status
  ├─ POST        /api/models/test       → ping model (self-call completions)
  │
  ├─ GET/POST   /api/provider-nodes     → list and create nodes
  ├─ GET/PUT/DELETE /api/provider-nodes/[id] → single node CRUD
  ├─ POST       /api/provider-nodes/validate → API key + base URL validation
  │
  ├─ GET/POST   /api/proxy-pools        → list and create pools
  ├─ GET/PUT/DELETE /api/proxy-pools/[id] → single pool CRUD
  ├─ POST       /api/proxy-pools/cloudflare-deploy → Cloudflare Workers relay
  ├─ POST       /api/proxy-pools/vercel-deploy    → Vercel Edge relay
  └─ POST       /api/proxy-pools/deno-deploy       → Deno Deploy relay

Go Backend (Fiber HTTP)
  ├─ Settings Handler → Settings Repository → settings / kv tables
  ├─ Pricing Handler → Pricing Repository → kv table (pricing key)
  ├─ Models Handler   → KV table (aliases, disabled, custom)
  ├─ ProviderNodes Handler → provider_nodes table
  ├─ ProxyPools Handler   → proxy_pools table
  └─ Deploy Handlers      → Cloudflare/Vercel/Deno third-party APIs
```

---

## Task Groups

| Group | Prefix | Count | Scope |
|-------|--------|-------|-------|
| A | SYS-001–006 | 6 | Settings core |
| B | SYS-007–012 | 6 | Health, init, version, shutdown |
| C | SYS-013–014 | 2 | Locale and tags |
| D | SYS-015–017 | 3 | Pricing configuration |
| E | SYS-018–025 | 8 | Models management |
| F | SYS-026–029 | 4 | Provider nodes |
| G | SYS-030–035 | 6 | Proxy pools and deploy |

---

## Group A: Settings Core

---

### SYS-001: GET /api/settings — Read settings with secret redaction

**Description:** Return the current settings row with sensitive fields stripped. Reads from the settings singleton table. Adds computed flags `oidcConfigured`, `enableRequestLogs`, `enableTranslator`, and `hasPassword`. Cached with `Cache-Control: no-store`.

**Input contract:**
- No path parameters or query params
- Auth: optional (dashboard auth or API key)

**Output contract:**
```json
{
  "requireLogin": true,
  "outboundProxyEnabled": false,
  "outboundProxyUrl": "",
  "outboundNoProxy": "",
  "comboStrategy": "round-robin",
  "comboStickyRoundRobinLimit": 10,
  "comboStrategies": [...],
  "oidcIssuerUrl": "",
  "oidcClientId": "",
  "oidcConfigured": false,
  "tunnelDashboardAccess": true,
  "tunnelUrl": "",
  "tailscaleUrl": "",
  "enableRequestLogs": false,
  "enableTranslator": false,
  "hasPassword": false
}
```

**Test strategy:**
- GET returns 200 with redacted fields absent (`password`, `oidcClientSecret`)
- `oidcConfigured` is `false` when any OIDC field is missing
- `hasPassword` reflects whether password is set
- Env-driven flags (`ENABLE_REQUEST_LOGS`, `ENABLE_TRANSLATOR`) are reflected in response

**Dependencies:** SYS-002 (settings repository), DB-001 (settings table)

---

### SYS-002: PATCH /api/settings — Update settings with side effects

**Description:** Merge-patch settings row. Handles password change with bcrypt hashing and current-password verification. Detects OIDC secret clear (empty string deletes the field). Triggers two runtime side effects on specific field changes:
1. `outboundProxyEnabled` / `outboundProxyUrl` / `outboundNoProxy` → calls `applyOutboundProxyEnv()`
2. `comboStrategy` / `comboStickyRoundRobinLimit` / `comboStrategies` → calls `resetComboRotation()`

Returns same redacted shape as GET.

**Input contract:**
```json
{
  "requireLogin": true,
  "outboundProxyEnabled": true,
  "outboundProxyUrl": "http://proxy:8080",
  "comboStrategy": "sticky",
  "newPassword": "s3cr3t",
  "currentPassword": "old",
  "oidcClientSecret": ""
}
```

**Output contract:** Same redacted shape as SYS-001 GET.

**Test strategy:**
- Password change: requires `newPassword`; if password exists, requires correct `currentPassword`; first-time password allows "123456" as default
- OIDC secret: setting to empty string or whitespace-only deletes the field from DB
- Side effect: changing outbound proxy fields calls `applyOutboundProxyEnv()` (mock/spy verification)
- Side effect: changing combo fields calls `resetComboRotation()` (mock/spy verification)
- Returns 400 if `currentPassword` missing when updating existing password
- Returns 401 if `currentPassword` incorrect

**Dependencies:** SYS-001, DB-001

---

### SYS-003: GET /api/settings/database — Export full database

**Description:** Serialise the entire database (all tables) into a JSON payload suitable for re-import. Uses the `exportDb()` repository function.

**Input contract:** None.

**Output contract:**
```json
{
  "version": 1,
  "exportedAt": "2026-06-04T12:00:00Z",
  "settings": { ... },
  "providerConnections": [ ... ],
  "providerNodes": [ ... ],
  "proxyPools": [ ... ],
  "apiKeys": [ ... ],
  "combos": [ ... ],
  "modelAliases": { "provider/model": "alias" },
  "disabledModels": { "openai": ["gpt-4"] },
  "customModels": [ ... ],
  "pricing": { ... }
}
```

**Test strategy:**
- GET returns 200 with valid JSON payload
- Payload contains all known table keys
- Round-trip: export → import → export yields equivalent data

**Dependencies:** DB-001 (settings table), DB-002 (provider connections table), DB-003 (proxy pools table), DB-004 (model KV tables)

---

### SYS-004: POST /api/settings/database — Import full database

**Description:** Replace entire database content from JSON payload. After import, re-applies outbound proxy env settings. Uses `importDb()` repository function.

**Input contract:** Same shape as SYS-003 output.

**Output contract:**
```json
{ "success": true }
```

**Test strategy:**
- Valid payload returns `{ success: true }` with 200
- Invalid payload returns `{ error: "..." }` with 400
- After import, proxy env is re-applied (mock/spy on `applyOutboundProxyEnv`)
- Round-trip: export → import → export yields equivalent data

**Dependencies:** SYS-003, DB-001, DB-002, DB-003, DB-004

---

### SYS-005: POST /api/settings/proxy-test — Test proxy connectivity

**Description:** Accept a proxy URL and optional test URL + timeout, then attempt an HTTP request through the proxy. Uses `testProxyUrl()` service. Distinguishes timeout (`AbortError`) from other errors.

**Input contract:**
```json
{
  "proxyUrl": "http://user:pass@proxy:8080",
  "testUrl": "https://api.openai.com/v1/models",
  "timeoutMs": 10000
}
```

**Output contract (success):**
```json
{ "ok": true, "latencyMs": 150, "status": 200 }
```
**Output contract (failure):**
```json
{ "ok": false, "error": "Connection refused" }
```

**Test strategy:**
- Valid proxy returns `{ ok: true, latencyMs }`
- Timeout returns `{ ok: false, error: "Proxy test timed out" }`
- Invalid proxy URL returns `{ ok: false, error: "..." }` with 400

**Dependencies:** None (standalone service call)

---

### SYS-006: GET /api/settings/require-login — Read auth and access flags

**Description:** Return a subset of settings related to authentication and tunnel access. Defaults to `requireLogin: true` if settings read fails.

**Input contract:** None.

**Output contract:**
```json
{
  "requireLogin": true,
  "tunnelDashboardAccess": true,
  "tunnelUrl": "",
  "tailscaleUrl": ""
}
```

**Test strategy:**
- Returns correct defaults when settings row is missing/corrupt
- Returns actual values from settings when present

**Dependencies:** SYS-001

---

## Group B: Health, Init, Version, Shutdown

---

### SYS-007: GET /api/health — Health check

**Description:** Lightweight check. Returns `{ ok: true }` with CORS headers (`Access-Control-Allow-Origin: *`). No DB calls.

**Input contract:** None.

**Output contract:**
```json
{ "ok": true }
```
Headers: `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Methods: GET, OPTIONS`

**Test strategy:**
- Returns 200 with `{ ok: true }`
- CORS preflight OPTIONS returns 204

**Dependencies:** None

---

### SYS-008: GET /api/init — Initialization marker

**Description:** Called automatically on app startup. Returns plain text `"Initialized"` with status 200. No DB calls.

**Input contract:** None.

**Output contract:** Plain text `"Initialized"`, HTTP 200.

**Test strategy:**
- Returns 200 with text body `"Initialized"`

**Dependencies:** None

---

### SYS-009: GET /api/version — Get version info

**Description:** Fetch latest version from npm registry (`https://registry.npmjs.org/9router/latest`) with 4s timeout, compare with current version from `package.json`. Returns `hasUpdate: true` if latest > current.

**Input contract:** None.

**Output contract:**
```json
{
  "currentVersion": "1.2.3",
  "latestVersion": "1.3.0",
  "hasUpdate": true
}
```

**Test strategy:**
- Returns 200 with all three fields
- `latestVersion` is `null` if npm fetch fails/times out
- `hasUpdate` is `false` when latestVersion is null or equal

**Dependencies:** None (reads package.json directly)

---

### SYS-010: POST /api/version/update — Auto-update

**Description:** Production-only endpoint. Kills sibling processes (cloudflared, MITM, next-server) to release file locks, then spawns detached updater and exits current process.

**Input contract:** None.

**Output contract:**
```json
{ "success": true, "message": "Updater started. This app will exit shortly." }
```

**Test strategy:**
- Returns 403 in non-production (`NODE_ENV !== "production"`)
- Returns 200 in production before exiting
- `killAppProcesses()` is called (mock/spy)
- `spawnUpdaterAndExit()` is called (mock/spy)

**Dependencies:** None

---

### SYS-011: POST /api/version/shutdown — Shutdown for manual update

**Description:** Kills sibling processes then exits process after 500ms delay. Used to release file locks before a manual update.

**Input contract:** None.

**Output contract:**
```json
{ "success": true, "message": "Shutting down for manual update..." }
```

**Test strategy:**
- Calls `killAppProcesses()` (best-effort, errors ignored)
- Exits after 500ms
- Returns 200 before exit

**Dependencies:** None

---

### SYS-012: POST /api/shutdown — Development shutdown

**Description:** Development-only graceful shutdown. Requires `SHUTDOWN_SECRET` env var and `Authorization: Bearer <secret>` header. Exits after 500ms.

**Input contract:** `Authorization: Bearer <token>` header.

**Output contract:**
```json
{ "success": true, "message": "Shutting down..." }
```

**Test strategy:**
- Returns 403 in production
- Returns 401 if token missing or mismatched
- Returns 200 in dev with correct token before exit

**Dependencies:** None

---

## Group C: Locale and Tags

---

### SYS-013: POST /api/locale — Set locale cookie

**Description:** Validate locale against supported list, normalise it (e.g., `EN` → `en`), and set a 1-year `locale` cookie.

**Input contract:**
```json
{ "locale": "en" }
```

**Output contract:**
```json
{ "success": true, "locale": "en" }
```

**Test strategy:**
- Valid locale returns 200 with normalized locale
- Invalid locale returns 400
- Cookie is set with `maxAge: 60*60*24*365` and `path: /`

**Dependencies:** None

---

### SYS-014: GET /api/tags — List Ollama model tags

**Description:** Return the static list of Ollama model tags from the config module. CORS-enabled.

**Input contract:** None.

**Output contract:** JSON array of Ollama model tag strings.

**Test strategy:**
- Returns 200 with array
- CORS headers present

**Dependencies:** None

---

## Group D: Pricing Configuration

---

### SYS-015: GET /api/pricing — Get merged pricing configuration

**Description:** Return current pricing config (user overrides merged with defaults) from the pricing key in the kv table.

**Input contract:** None.

**Output contract:**
```json
{
  "openai": {
    "gpt-4o": { "input": 2.5, "output": 10.0, "cached": 1.25, "reasoning": null },
    "gpt-4o-mini": { "input": 0.15, "output": 0.6, "cached": 0.075 }
  },
  "anthropic": {
    "claude-3-5-sonnet": { "input": 3.0, "output": 15.0, "cached": 3.75, "reasoning": 3.75 }
  }
}
```

**Test strategy:**
- Returns 200 with valid pricing object
- Empty user overrides returns defaults only

**Dependencies:** DB-004 (pricing kv entry)

---

### SYS-016: PATCH /api/pricing — Update pricing overrides

**Description:** Validate structure, then merge-patch the pricing overrides in the kv table. Valid fields per model: `input`, `output`, `cached`, `reasoning`, `cache_creation`. All values must be non-negative numbers.

**Input contract:**
```json
{
  "openai": {
    "gpt-4o": { "input": 2.5, "output": 10.0 }
  }
}
```

**Output contract:** Same shape as GET (merged result).

**Test strategy:**
- Valid patch returns 200 with merged pricing
- Invalid field name returns 400 with field name in error
- Negative value returns 400
- Non-number value returns 400

**Dependencies:** SYS-015

---

### SYS-017: DELETE /api/pricing — Reset pricing overrides

**Description:** Reset pricing to defaults. Supports three scopes via query params:
- No params → reset all (`resetAllPricing()`)
- `?provider=openai` → reset entire provider
- `?provider=openai&model=gpt-4o` → reset specific model

**Input contract:** Query params: `provider`, `model`.

**Output contract:** Returns full pricing config after reset (same as GET).

**Test strategy:**
- No params resets all pricing
- Provider param resets only that provider
- Model param resets only that model
- Returns updated pricing

**Dependencies:** SYS-015

---

## Group E: Models Management

---

### SYS-018: GET /api/models — List models with aliases (legacy)

**Description:** Return AI_MODELS list, filtering out disabled models, applying aliases from kv table. Enriches each model with `fullModel` (provider/model) and `alias` fields.

**Input contract:** None.

**Output contract:**
```json
{
  "models": [
    { "provider": "openai", "model": "gpt-4o", "fullModel": "openai/gpt-4o", "alias": "GPT-4o" }
  ]
}
```

**Test strategy:**
- Returns all non-disabled models with aliases applied
- Disabled models are excluded from list
- Alias not set returns original model name as alias

**Dependencies:** DB-004 (model aliases + disabled models kv entries)

---

### SYS-019: PUT /api/models — Update model alias (legacy path)

**Description:** Legacy alias setter. Checks for duplicate alias across different models before writing.

**Input contract:**
```json
{ "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

**Output contract:**
```json
{ "success": true, "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

**Test strategy:**
- Sets alias and returns success
- Duplicate alias for different model returns 400

**Dependencies:** SYS-018

---

### SYS-020: GET /api/models/alias — Get all model aliases

**Description:** Return the full model-alias map from the kv table.

**Input contract:** None.

**Output contract:**
```json
{ "aliases": { "openai/gpt-4o": "GPT-4o", "anthropic/claude-3-5-sonnet": "Claude Sonnet" } }
```

**Test strategy:**
- Returns 200 with alias map (may be empty object)

**Dependencies:** DB-004 (model aliases kv entry)

---

### SYS-021: PUT /api/models/alias — Set model alias

**Description:** Set a model alias (key-value: alias → model). Returns 400 if alias already maps to a different model.

**Input contract:**
```json
{ "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

**Output contract:**
```json
{ "success": true, "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

**Test strategy:**
- Sets alias, confirms with GET
- Duplicate alias returns 400

**Dependencies:** SYS-020

---

### SYS-022: DELETE /api/models/alias — Delete model alias

**Description:** Delete a model alias by alias value (not by model).

**Input contract:** Query param: `alias=xxx`.

**Output contract:**
```json
{ "success": true }
```

**Test strategy:**
- Missing alias param returns 400
- Deleted alias no longer appears in GET

**Dependencies:** SYS-020

---

### SYS-023: GET/POST/DELETE /api/models/custom — Custom model entries

**Description:** Manage user-defined custom model entries stored in kv table. Custom models are keyed by `providerAlias + id + type`.

**Input contract (POST):**
```json
{ "providerAlias": "openai", "id": "my-model", "type": "llm", "name": "My Model" }
```

**Output contract (GET):**
```json
{ "models": [ { "providerAlias": "openai", "id": "my-model", "type": "llm", "name": "My Model" } ] }
```

**Output contract (DELETE):** Query params: `providerAlias`, `id`, `type`.

**Test strategy:**
- POST creates entry, GET confirms
- DELETE removes entry
- Missing required fields return 400

**Dependencies:** DB-004 (custom models kv entry)

---

### SYS-024: GET/POST/DELETE /api/models/disabled — Per-provider disabled models

**Description:** Manage per-provider disabled model lists. Stored in kv table keyed by provider alias.

**Input contract (POST):**
```json
{ "providerAlias": "openai", "ids": ["gpt-3.5-turbo"] }
```

**Output contract (GET):** Without param returns full disabled map. With `?providerAlias=openai` returns only that provider's list.
```json
{ "ids": ["gpt-3.5-turbo"] }
```

**Test strategy:**
- POST disables listed models, GET confirms
- DELETE without model param clears entire provider list
- DELETE with model param removes only that model from list
- Filtering by provider works correctly

**Dependencies:** DB-004 (disabled models kv entry)

---

### SYS-025: GET/POST /api/models/availability — Model cooldown and availability status

**Description:** Read current model cooldowns and unavailable status from provider connections. POST clears cooldown for a specific model.

**Output contract (GET):**
```json
{
  "models": [
    { "provider": "openai", "model": "gpt-4o", "status": "cooldown", "until": 1750000000000, "connectionId": "...", "connectionName": "...", "lastError": "rate limited" },
    { "provider": "openai", "model": "__all", "status": "unavailable", "connectionId": "...", "connectionName": "...", "lastError": "API key invalid" }
  ],
  "unavailableCount": 2
}
```

**Input contract (POST):**
```json
{ "action": "clearCooldown", "provider": "openai", "model": "gpt-4o" }
```

**Test strategy:**
- GET returns locks filtered by expiry (only active locks returned)
- Connections with `testStatus: unavailable` show `__all` model
- POST clears lock and resets `testStatus` to `active`
- Invalid action returns 400

**Dependencies:** DB-002 (provider connections table)

---

### SYS-026: POST /api/models/test — Ping model via self-call

**Description:** Self-call the internal completions or embeddings endpoint to test model connectivity. Uses an active API key for auth and machine ID for CLI token bypass. Measures latency.

**Input contract:**
```json
{ "model": "openai/gpt-4o", "kind": "chat" }
```
`kind` can be `"chat"` (default) or `"embedding"`.

**Output contract (success):**
```json
{ "ok": true, "latencyMs": 320, "error": null, "status": 200 }
```
**Output contract (failure):**
```json
{ "ok": false, "latencyMs": 320, "error": "HTTP 429: Rate limit exceeded", "status": 429 }
```

**Test strategy:**
- Valid model returns `{ ok: true, latencyMs }`
- Invalid model returns `{ ok: false, error: "..." }`
- Timeout (15s) handled gracefully
- Embedding kind routes to `/api/v1/embeddings`

**Dependencies:** SYS-018, DB-002 (apiKeys table)

---

## Group F: Provider Nodes

---

### SYS-027: GET/POST /api/provider-nodes — List and create provider nodes

**Description:** List all provider nodes or create a new one. Supports three node types with distinct ID prefixes and base URL sanitisation:
- `openai-compatible`: requires `apiType` ("chat" or "responses"), ID prefix `openai-compatible-chat-` or `openai-compatible-responses-`
- `anthropic-compatible`: strips trailing slash and `/messages` from base URL
- `custom-embedding`: strips trailing slash and `/embeddings` from base URL

**Input contract (POST):**
```json
{
  "name": "My Gateway",
  "prefix": "my",
  "type": "anthropic-compatible",
  "baseUrl": "https://api.example.com/v1"
}
```

**Output contract (GET):**
```json
{ "nodes": [ { "id": "anthropic-compatible-abc123", "type": "anthropic-compatible", "name": "My Gateway", "prefix": "my", "baseUrl": "https://api.example.com/v1" } ] }
```

**Test strategy:**
- GET returns all nodes
- POST creates node, ID prefix matches type
- POST with invalid type returns 400
- POST with missing name/prefix returns 400
- POST sanitises base URL correctly per type
- Duplicate name/prefix handled gracefully

**Dependencies:** DB-002 (provider_nodes table)

---

### SYS-028: GET/PUT/DELETE /api/provider-nodes/[id] — Single provider node CRUD

**Description:** Read, update, or delete a single provider node. On update, cascades prefix/apiType/baseUrl changes to all bound provider connections.

**Input contract (PUT):**
```json
{ "name": "Updated Name", "prefix": "upd", "apiType": "chat", "baseUrl": "https://new.example.com/v1" }
```

**Output contract (PUT):**
```json
{ "node": { "id": "...", "name": "Updated Name", ... } }
```

**Test strategy:**
- GET returns 404 for unknown ID
- PUT updates node and returns updated node
- PUT cascades updates to bound connections
- DELETE removes node and all its connections
- DELETE returns 404 for unknown ID

**Dependencies:** SYS-027, DB-002

---

### SYS-029: POST /api/provider-nodes/validate — Validate API key against base URL

**Description:** Test API key validity by hitting the provider's `/models` endpoint or `/chat/completions` fallback. Handles custom embedding, anthropic-compatible, and openai-compatible patterns. Returns user-friendly error messages for common network errors (ECONNREFUSED, ETIMEDOUT, SSL errors).

**Input contract:**
```json
{ "baseUrl": "https://api.example.com/v1", "apiKey": "sk-...", "type": "anthropic-compatible", "modelId": "claude-3-5-sonnet-20241022" }
```

**Output contract (success):**
```json
{ "valid": true }
```
or
```json
{ "valid": true, "method": "chat", "dimensions": 1536 }
```

**Test strategy:**
- Valid key returns `{ valid: true }`
- Auth failure returns `{ valid: false, error: "API key unauthorized" }`
- Network errors map to user-friendly messages
- Fallback to chat when /models fails but modelId provided
- Embedding validation extracts dimensions from response

**Dependencies:** None (external HTTP calls)

---

## Group G: Proxy Pools and Deploy

---

### SYS-030: GET/POST /api/proxy-pools — List and create proxy pools

**Description:** List proxy pools with optional `isActive` filter and `includeUsage` flag. `includeUsage=true` enriches each pool with `boundConnectionCount` from provider connections.

**Query params:** `?isActive=true`, `?includeUsage=true`

**Input contract (POST):**
```json
{
  "name": "us-east-proxy",
  "proxyUrl": "http://user:pass@proxy.example.com:8080",
  "type": "http",
  "noProxy": "localhost,127.0.0.1",
  "isActive": true,
  "strictProxy": false
}
```

**Output contract (GET):**
```json
{ "proxyPools": [ { "id": "...", "name": "us-east-proxy", "proxyUrl": "...", "type": "http", "isActive": true, "boundConnectionCount": 0 } ] }
```

**Test strategy:**
- GET without params returns all pools
- GET with isActive filter returns filtered list
- GET with includeUsage adds boundConnectionCount
- POST creates pool with defaults (type: http, isActive: true)
- POST without name returns 400
- POST without proxyUrl returns 400

**Dependencies:** DB-003 (proxy_pools table)

---

### SYS-031: GET/PUT/DELETE /api/proxy-pools/[id] — Single proxy pool CRUD

**Description:** Read, update, or delete a single proxy pool. DELETE is protected: returns 409 Conflict if any provider connections are bound to this pool.

**Input contract (PUT):** Partial update — only fields present in body are updated.
```json
{ "isActive": false, "name": "renamed" }
```

**Output contract (DELETE failure):**
```json
{ "error": "Proxy pool is currently in use", "boundConnectionCount": 3 }
```

**Test strategy:**
- GET returns 404 for unknown ID
- PUT applies partial update
- PUT validates fields (name, proxyUrl, type, isActive, strictProxy, noProxy)
- DELETE with bound connections returns 409
- DELETE without bound connections returns 200

**Dependencies:** SYS-030, DB-003

---

### SYS-032: POST /api/proxy-pools/cloudflare-deploy — Deploy Cloudflare Workers relay

**Description:** Deploy a Cloudflare Workers relay script to the user's account. Steps:
1. PUT upload Worker script + metadata (multipart/form-data) to `/workers/scripts/{projectName}`
2. POST enable workers.dev subdomain
3. GET workers subdomain
4. Create proxy pool entry with type `cloudflare`

Requires `accountId` and `apiToken`.

**Input contract:**
```json
{
  "accountId": "abc123...",
  "apiToken": " Cloudflare API token",
  "projectName": "my-relay"
}
```

**Output contract:**
```json
{
  "proxyPool": { "id": "...", "name": "my-relay", "proxyUrl": "https://my-relay.xxx.workers.dev", "type": "cloudflare" },
  "deployUrl": "https://my-relay.xxx.workers.dev"
}
```

**Test strategy:**
- Missing accountId/apiToken returns 400
- Cloudflare API errors returned with appropriate status
- Worker script correctly bundled (RELAY_WORKER_CODE inlined)
- Returns 201 with created proxy pool

**Dependencies:** SYS-030

---

### SYS-033: POST /api/proxy-pools/vercel-deploy — Deploy Vercel Edge relay

**Description:** Deploy a Vercel Edge Function relay via Vercel Deployments API v13. Files: `api/relay.js`, `package.json`, `vercel.json`. Polls until READY (max 2 minutes). Disables Vercel authentication on project. Creates proxy pool with type `vercel`.

**Input contract:**
```json
{
  "vercelToken": "Vercel API token",
  "projectName": "my-relay"
}
```

**Output contract:**
```json
{
  "proxyPool": { "id": "...", "name": "my-relay", "proxyUrl": "https://my-relay.vercel.app", "type": "vercel" },
  "deployUrl": "https://my-relay.vercel.app"
}
```

**Test strategy:**
- Missing vercelToken returns 400
- Deployment failure returns 400 with error message
- Timeout (>2 min) returns 500
- Polling correctly handles READY, ERROR, CANCELED states
- SsoProtection disabled after deploy

**Dependencies:** SYS-030

---

### SYS-034: POST /api/proxy-pools/deno-deploy — Deploy Deno Deploy relay

**Description:** Create a Deno Deploy app via `/apps` endpoint, deploy via `/apps/{id}/deploy`, poll until `succeeded` (max 60s), then create proxy pool with type `deno`. On failure, rollback (DELETE app).

**Input contract:**
```json
{
  "denoToken": "Deno API token",
  "orgDomain": "myorg.deno.dev",
  "projectName": "my-relay"
}
```

**Output contract:**
```json
{
  "proxyPool": { "id": "...", "name": "my-relay", "proxyUrl": "https://my-relay.myorg.deno.net", "type": "deno" },
  "deployUrl": "https://my-relay.myorg.deno.net"
}
```

**Test strategy:**
- Missing orgDomain or denoToken returns 400
- Duplicate project name (409) returned as 409
- Build failure rolls back app creation
- Timeout (>60s polling) returns 500
- App slug derived correctly from projectName

**Dependencies:** SYS-030

---

## Cross-Cutting Concerns

| Concern | Tasks |
|---------|-------|
| Auth middleware | SYS-001, SYS-002, SYS-003, SYS-004, SYS-005, SYS-006, SYS-013, SYS-015, SYS-016, SYS-017, SYS-018–SYS-026, SYS-027–SYS-029, SYS-030–SYS-034 |
| DB transactions | SYS-002, SYS-003, SYS-004, SYS-015, SYS-016, SYS-017, SYS-020–SYS-024, SYS-027, SYS-028, SYS-030, SYS-031 |
| CORS | SYS-007, SYS-014 |
| No-store cache headers | SYS-001, SYS-002 |
| Error shape consistency | All (`{ error: string }`) |
| Production-only guards | SYS-010, SYS-012 |

---

## Migration Notes

- All settings are currently stored in a single `settings` row (JSON string column). Go rewrite must preserve this — use a JSON-typed column or TEXT with parse/stringify helpers.
- Proxy pool deploy endpoints (SYS-032–SYS-034) make third-party HTTP calls; the Go implementation should use an HTTP client with configurable timeouts.
- `testProxyUrl()` in SYS-005 is a network I/O function — must be testable with mocked HTTP responses.
- The `resetComboRotation()` side effect in SYS-002 is a module-level call; the Go equivalent should use an injected service or event bus to avoid tight coupling.
- Model availability (SYS-025) reads from provider connections and writes lock state back — ensure the Go implementation uses atomic operations or optimistic locking for the `modelLock_*` fields.
