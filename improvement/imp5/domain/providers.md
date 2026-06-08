# Providers Domain — Atomic Task Breakdown

> 9Router backend rewrite. Target: Go + Fiber. Reference: Node.js `src/app/api/providers/`, `src/shared/constants/providers.js`, `src/sse/services/auth.js`, `src/sse/handlers/chat.js`.

---

## Scope Coverage

This domain covers all provider/connection management API endpoints, model resolution (alias → provider/model), credentials selection with fallback and rate-limit tracking, and per-connection testing/validation.

**Source-of-truth files analyzed:**
- `src/app/api/providers/route.js` — GET /api/providers (list), POST /api/providers (create)
- `src/app/api/providers/[id]/route.js` — GET, PUT, DELETE single connection
- `src/app/api/providers/validate/route.js` — API key validation by provider
- `src/app/api/providers/test-batch/route.js` — Batch test by mode (oauth, apikey, free, compatible, all, provider)
- `src/app/api/providers/[id]/test/route.js` — Single connection test
- `src/app/api/providers/suggested-models/route.js` — Fetch and filter suggested free models
- `src/app/api/providers/client/route.js` — Paginated, sanitized provider list for client UI
- `src/shared/constants/providers.js` — All provider definitions, aliases, auth types, service kinds, media configs
- `src/sse/services/auth.js` — getProviderCredentials, markAccountUnavailable, clearAccountError, extractApiKey
- `src/sse/handlers/chat.js` — Credential rotation loop, token refresh, model lock

**Domain context from manifesto:** Sections 3.1 (provider registry), 3.2 (model resolution), 3.3 (credential selection), 4.1 (GORM models for connections).

---

## Group A: Provider Model Resolution

---

## PROV-001 — Model String Parsing (modelInfo Extraction)

**Description**
Parse a model string like `"claude-3-haiku-20240307"`, `"anthropic/claude-sonnet-4-20250514"`, or `"openai/gpt-4o-mini"` into `{ provider, model }`. Support three formats: bare model (implicit provider from settings), `provider/model` (explicit), and `provider:model` (alias for OpenAI-compatible style). Validate that resolved provider is in the registry.

**Input**
- Model string: `"claude-3-haiku-20240307"` or `"anthropic/claude-sonnet-4-20250514"`
- Provider registry map
- Default provider from settings (fallback for bare models)

**Output**
- `modelInfo = { provider: "anthropic", model: "claude-sonnet-4-20250514" }` or `null` if unknown

**Test Strategy**
- `"provider/model"` → correct split
- `"provider:model"` → correct split
- Bare model uses default provider from settings
- Unknown provider returns null with error
- Model-only input with no default provider returns error

**Dependencies**
- PROV-002 (alias resolution)

---

## PROV-002 — Provider Alias Resolution (ALIAS_TO_ID)

**Description**
Implement `resolveProviderId(aliasOrId string) string` using the provider registry ALIAS_TO_ID map. If input is already a valid provider ID, return it as-is. Support alias strings like `"cc"` → `"claude"`, `"ds"` → `"deepseek"`. Also handle special prefixes: `openai-compatible-*`, `anthropic-compatible-*`, `custom-embedding-*`.

**Input**
- `aliasOrId` string, e.g. `"ds"`, `"anthropic"`, `"openai-compatible-node-abc123"`

**Output**
- Provider ID string, e.g. `"deepseek"`, `"anthropic"`, `"openai-compatible-node-abc123"`
- Error if alias not found in registry

**Test Strategy**
- `"ds"` → `"deepseek"`
- `"anthropic"` (valid ID) → `"anthropic"`
- `"openai-compatible-abc"` → `"openai-compatible-abc"`
- Unknown alias `"xyz"` → error

**Dependencies**
- None (base utility)

---

## PROV-003 — Provider Registry Validation

**Description**
Validate that a provider ID is registered. Check against combined AI_PROVIDERS map (FREE_PROVIDERS + FREE_TIER_PROVIDERS + OAUTH_PROVIDERS + APIKEY_PROVIDERS + WEB_COOKIE_PROVIDERS). Also validate service kind compatibility when a specific kind is requested (e.g., "tts" only returns TTS-capable providers).

**Input**
- Provider ID string
- Optional service kind filter (`"llm"`, `"tts"`, `"embedding"`, `"webSearch"`, `"image"`, etc.)

**Output**
- `providerDef` object (with all fields: id, alias, name, authType, serviceKinds, config) or `null`

**Test Strategy**
- Valid provider ID returns provider def
- Unknown ID returns null
- With `kind: "tts"` filter: returns only TTS-capable providers
- Hidden providers filtered out by default
- Providers with `hiddenKinds` for a specific kind are excluded

**Dependencies**
- PROV-002

---

## PROV-004 — Default Model Resolution per Provider

**Description**
Implement `getDefaultModel(providerId string) string`. Return the server-configured default model for a provider. Sources: hardcoded model maps (e.g., openai → `"gpt-4o"`, anthropic → `"claude-sonnet-4-20250514"`, groq → `"llama-4-scout-17b-16e-instruct"`). For providers with `modelsFetcher` (openrouter, opencode-free), fetches from external URL with caching.

**Input**
- Provider ID string

**Output**
- Default model string, e.g. `"gpt-4o"`
- `null` if no default configured

**Test Strategy**
- Known providers return correct default model
- Unknown provider returns null
- External fetcher caches results; second call uses cache
- Invalid fetcher URL handled gracefully

**Dependencies**
- PROV-003

---

## PROV-005 — Combo Model Resolution

**Description**
Resolve a combo model name (e.g., `"best:4o-mini,grok-3,claude-3.5"`) into its constituent model list. Parse `:` and `,` separators. Support strategy suffix in model name (e.g., `model@strategy`). For each sub-model, resolve via PROV-001 to get `{ provider, model }`.

**Input**
- Combo model string: `"best:4o-mini,grok-3,claude-3.5"` or `"fallback:gpt-4o,gemini-2.5-flash"`
- Global + combo-specific fallback strategy from settings

**Output**
- `{ models: [{ provider, model, comboName }], strategy: "fallback"|"round-robin"|"fill-first" }` or `null`

**Test Strategy**
- Valid combo string parses into correct model list
- Unknown sub-model within combo returns error
- Strategy extracted from model name suffix `@strategy`
- Empty combo string returns null

**Dependencies**
- PROV-001, PROV-004

---

## Group B: Provider CRUD Endpoints

---

## PROV-006 — List Providers (GET /api/providers)

**Description**
Return all provider connections as JSON array. Hide sensitive fields (`apiKey`, `accessToken`, `refreshToken`, `idToken`). For compatible providers (`openai-compatible-*`, `anthropic-compatible-*`), enrich `name` from node registry or `providerSpecificData.nodeName`. Supports optional `isActive` filter.

**Input**
- Query params: none (list all) or filter `{ isActive: bool }`

**Output**
- `{ connections: [{ id, provider, authType, name, priority, globalPriority, defaultModel, testStatus, lastError, ... }] }`
- Sensitive fields omitted

**Test Strategy**
- Returns all connections when no filter
- Returns only active when `isActive=true`
- Sensitive fields absent in response
- Compatible provider name enriched from node registry

**Dependencies**
- DB-xxx (connections table read), PROV-003 (provider def lookup)

---

## PROV-007 — Create Provider (POST /api/providers)

**Description**
Create a new provider connection. Validate: provider ID in registry, required fields present (name, provider, authType based on provider type), apiKey required unless noAuth provider or `ollama-local`. Build `providerSpecificData` from request body, including proxy config and proxy pool ID resolution. Set `authType` automatically: `cookie` for web-cookie providers, `apikey` otherwise. Return created connection (without sensitive fields).

**Input**
- Request body: `{ provider, apiKey, name, displayName, priority, globalPriority, defaultModel, testStatus, proxyPoolId, connectionProxyEnabled, connectionProxyUrl, connectionNoProxy, providerSpecificData }`

**Output**
- `{ connection: { ... } }` with status 201
- 400 if validation fails (invalid provider, missing apiKey, etc.)
- 404 if proxyPoolId references non-existent pool

**Test Strategy**
- Valid request creates connection and returns 201
- Invalid provider returns 400
- Missing required apiKey returns 400
- Proxy pool ID validated against DB
- Proxy config (enabled + URL) validated: URL required when enabled
- Compatible providers (openai-compatible-*, anthropic-compatible-*) resolve node data into providerSpecificData

**Dependencies**
- DB-xxx (connection insert), PROV-002, PROV-003

---

## PROV-008 — Get Provider by ID (GET /api/providers/[id])

**Description**
Return a single provider connection by ID. Hide sensitive fields same as PROV-006.

**Input**
- URL param: `id` (connection UUID)

**Output**
- `{ connection: { ... } }` with status 200
- 404 if not found

**Test Strategy**
- Existing ID returns connection
- Non-existent ID returns 404
- Sensitive fields absent in response

**Dependencies**
- DB-xxx (connection read)

---

## PROV-009 — Update Provider (PUT /api/providers/[id])

**Description**
Partial update of a provider connection. Allow updating: `name`, `priority`, `globalPriority`, `defaultModel`, `isActive`, `apiKey` (only for apikey auth type), `testStatus`, `lastError`, `lastErrorAt`, `providerSpecificData`. Normalize proxy config and proxy pool ID same as create. Merge `providerSpecificData` with existing (preserve fields not in update). Only allow apiKey update if `authType === "apikey"`.

**Input**
- URL param: `id`
- Request body (partial): any subset of connection fields

**Output**
- `{ connection: { ... } }` with status 200
- 404 if not found
- 400 if proxy URL required but missing, or proxy pool not found

**Test Strategy**
- Partial update changes only specified fields
- Unchanged fields preserve existing values
- Proxy config update merges correctly
- apiKey update only allowed for apikey authType
- Non-existent ID returns 404

**Dependencies**
- DB-xxx (connection update), PROV-002

---

## PROV-010 — Delete Provider (DELETE /api/providers/[id])

**Description**
Delete a provider connection by ID.

**Input**
- URL param: `id`

**Output**
- `{ message: "Connection deleted successfully" }` with status 200
- 404 if not found

**Test Strategy**
- Existing ID deleted and returns 200
- Non-existent ID returns 404

**Dependencies**
- DB-xxx (connection delete)

---

## Group C: Model Testing and Validation

---

## PROV-011 — Test Single Connection (POST /api/providers/test)

**Description**
Test a single provider connection by ID. Proxy test runs first if proxy is configured. Then dispatch to `testApiKeyConnection` (apikey/cookie auth) or `testOAuthConnection` (oauth auth). On completion, update connection in DB: `testStatus` → `"active"` or `"error"`, `lastError`, `lastErrorAt`. If token was refreshed, persist new tokens and new `expiresAt`. Return result object.

**Input**
- URL param: `id` (connection UUID)

**Output**
- `{ valid: bool, error: string|null, refreshed: bool }` with status 200
- 404 if not found
- 500 on failure

**Test Strategy**
- Valid API key returns `valid: true`
- Invalid API key returns `valid: false` with error
- Token refresh: `refreshed: true` and tokens persisted to DB
- Proxy failure surfaces as error before endpoint test
- Non-existent ID returns 404

**Dependencies**
- DB-xxx, PROV-012, PROV-013, PROV-015, PROV-016

---

## PROV-012 — API Key Probe by Provider (testApiKeyConnection)

**Description**
Per-provider API key validation using provider-specific endpoints and auth headers. Dispatch by provider ID to appropriate probe:
- OpenAI-compatible: `GET /models` with `Bearer` auth
- Anthropic-compatible: `GET /models` with `x-api-key` + `anthropic-version`
- Standard providers (openai, anthropic, deepseek, groq, etc.): each has specific endpoint and auth scheme
- cloudflare-ai: requires `accountId` from providerSpecificData; POST to Cloudflare Workers AI
- azure: requires `azureEndpoint`, `deployment`, `apiVersion`; POST to Azure endpoint
- grok-web, perplexity-web: cookie-based auth probe (distinct from apikey path)
- ollama-local: no auth, resolve host from providerSpecificData or env
- xiaomi-tokenplan: resolve base URL from region in providerSpecificData

Use `connectionProxyEnabled` + `connectionProxyUrl` when making requests.

**Input**
- Connection object with `{ provider, apiKey, providerSpecificData }`
- Effective proxy config

**Output**
- `{ valid: bool, error: string|null }`

**Test Strategy**
- Each provider type: valid key → valid, invalid → invalid
- cloudflare-ai: missing accountId → invalid with specific error
- azure: invalid endpoint → error, missing deployment → error
- grok-web: valid SSO cookie → valid, expired → invalid
- perplexity-web: valid session cookie → valid
- Unknown provider returns error "Provider test not supported"
- Network errors return invalid with error message

**Dependencies**
- PROV-003, PROV-014, PROV-015

---

## PROV-013 — OAuth Connection Test with Token Refresh

**Description**
Test OAuth-based provider connections. Per-provider test config (`OAUTH_TEST_CONFIG`):
- `checkExpiry`: verify `expiresAt` is not in the past (5-minute buffer), no HTTP probe needed (claude, qwen, kiro, qoder)
- `tokenExists`: for cursor — only verify token presence, no endpoint probe
- HTTP probe: for codex, github, gemini-cli, antigravity, kilocode, cline, gitlab — specific endpoint + expected status codes
- `refreshable`: if initial test fails with 401 and provider supports refresh, attempt token refresh then retry

Token refresh implementations per provider:
- gemini-cli / antigravity: Google OAuth2 refresh token endpoint
- codex: OpenAI token endpoint
- claude: Claude token endpoint (JSON body)
- kiro: Amazon Cognito or social refresh endpoint
- qwen: OAuth2 refresh with client_id
- cline: Cline refresh endpoint

**Input**
- Connection object with `{ provider, accessToken, refreshToken, expiresAt, providerSpecificData }`
- Effective proxy config

**Output**
- `{ valid: bool, error: string|null, refreshed: bool, newTokens: { accessToken, refreshToken, expiresIn }|null }`

**Test Strategy**
- Valid non-expired token → valid
- Expired token + successful refresh → valid with `refreshed: true`
- Expired token + failed refresh → invalid
- Revoked token → invalid with specific error
- `checkExpiry` providers: expired without refreshable → invalid
- cursor (tokenExists) → valid if token present
- cline: retries on 401 with refresh token

**Dependencies**
- PROV-002, PROV-003, token refresh per provider (OAuth domain)

---

## PROV-014 — Compatible Provider Test (OpenAI / Anthropic / Custom Embedding)

**Description**
Test OpenAI-compatible, Anthropic-compatible, and custom-embedding provider connections by calling their `/models` endpoint. For OpenAI-compatible: `GET /models` with `Authorization: Bearer <apiKey>`. For Anthropic-compatible: `GET /models` with `x-api-key` + `anthropic-version`. For custom-embedding: try `GET /models` first, then fallback to `POST /embeddings` with test input. Remove trailing `/messages` from base URL if present.

**Input**
- Connection with `providerSpecificData.baseUrl` and `apiKey`

**Output**
- `{ valid: bool, error: string|null }`

**Test Strategy**
- Valid API key → res.ok → valid
- 401/403 → invalid
- Missing baseUrl → invalid with specific error
- Custom embedding: /models returns 200 → valid; 401/403 → invalid; otherwise probe /embeddings

**Dependencies**
- PROV-003

---

## PROV-015 — Cookie-Based Provider Test (Grok Web, Perplexity Web)

**Description**
Test grok-web and perplexity-web providers using session cookie authentication:
- **grok-web**: Extract token from `"sso="` prefix, POST to `https://grok.com/rest/app-chat/conversations/new` with browser-like headers (User-Agent, Origin, Cookie, traceparent, x-xai-request-id). Any non-401/403 response means cookie is accepted.
- **perplexity-web**: Extract `__Secure-next-auth.session-token` from cookie value, GET `https://www.perplexity.ai/api/auth/session`. Valid session returns JSON with `user` field.

**Input**
- Connection with `apiKey` (cookie value) and `provider`

**Output**
- `{ valid: bool, error: string|null }`

**Test Strategy**
- grok-web: valid sso= cookie → valid; invalid → invalid
- perplexity-web: valid session token → `{ user: {...} }` → valid; expired → invalid with specific error

**Dependencies**
- None (self-contained)

---

## Group D: Test Batch Endpoint

---

## PROV-016 — Test Batch by Mode (POST /api/providers/test-batch)

**Description**
Test multiple connections by filter mode. Modes: `"oauth"` (authType=oauth), `"free"` (authType=free), `"apikey"` (authType=apikey), `"compatible"` (openai-compatible-*, anthropic-compatible-*), `"all"`, `"provider"` (specific provider ID). For each matching connection, call `testSingleConnection`. Return results array with per-connection: `provider`, `connectionId`, `connectionName`, `authType`, `valid`, `latencyMs`, `error`, `diagnosis`, `statusCode`, `testedAt`. Summary: `{ total, passed, failed }`.

**Input**
- Request body: `{ mode: "oauth"|"free"|"apikey"|"compatible"|"all"|"provider", providerId?: string }`

**Output**
- `{ mode, providerId, results: [...], summary: { total, passed, failed }, testedAt }`
- Empty array if no connections match mode
- 400 if mode invalid

**Test Strategy**
- `"all"` mode tests every active connection
- `"oauth"` mode filters to OAuth authType only
- `"compatible"` mode filters to compatible provider prefixes
- `"provider"` mode with valid provider ID tests only matching connections
- Empty result returns `{ results: [], summary: { total: 0, passed: 0, failed: 0 } }`
- Invalid mode returns 400

**Dependencies**
- PROV-011, PROV-002, PROV-003

---

## Group E: Provider Credentials Management

---

## PROV-017 — Get Provider Credentials (getProviderCredentials)

**Description**
Select an active connection for a provider from the connections table. Filter by `provider` ID and `isActive=true`. Exclude connection IDs in `excludeSet` (for retry loop). Exclude model-locked connections (modelLock_${model} or modelLock___all with future expiry). Apply fallback strategy from settings: `"fill-first"` (use priority-sorted first) or `"round-robin"` (least-recently-used with sticky round-robin limit). Resolve proxy config from `providerSpecificData.proxyPoolId` via proxy pool or direct config.

For no-auth free providers (opencode with noAuth=true), return a virtual "noauth" credentials object with proxy config resolved from settings.

Return credentials object:
```
{ authType, apiKey, accessToken, refreshToken, projectId, connectionName, providerSpecificData (with resolved proxy), connectionId, testStatus, lastError, _connection }
```

Selection mutex: prevent race conditions when multiple concurrent requests select accounts.

**Input**
- Provider ID string
- `excludeConnectionIds` (Set or string or null) — connection IDs to skip
- `model` string (for per-model lock check)
- `options.preferredConnectionId` (optional pin)

**Output**
- `credentials` object or `null` if no connections available
- `{ allRateLimited: true, retryAfter, retryAfterHuman, lastError, lastErrorCode }` if all accounts locked

**Test Strategy**
- Provider with one active connection returns that connection
- Provider with multiple: fill-first returns highest priority
- Round-robin: alternates between connections every `stickyLimit` uses
- Excluded connection IDs are not selected
- Model-locked connections excluded (with retryAfter on all locked)
- Noauth provider returns virtual credentials without DB read
- All accounts unavailable returns `allRateLimited: true`

**Dependencies**
- DB-xxx (connection read), PROV-002, PROV-003, PROV-022 (model lock check)

---

## PROV-018 — Mark Account Unavailable (markAccountUnavailable)

**Description**
Lock a connection+model after a failed request (429, 401, 5xx, etc.). Compute cooldown with exponential backoff (`checkFallbackError`) based on status code and error text. If provider sends `resetsAtMs` (e.g., codex usage_limit_reached resets_at), use that as precise cooldown. Set `modelLock_${model}` (or `modelLock___all`) with expiry timestamp in the DB. Also set `testStatus: "unavailable"`, `lastError`, `errorCode`, `lastErrorAt`, `backoffLevel`. Return whether the caller should fall back to next account.

**Input**
- `connectionId`, `status` (HTTP code), `errorText`, `provider`, `model`, `resetsAtMs` (optional)

**Output**
- `{ shouldFallback: bool, cooldownMs: number }`

**Test Strategy**
- 429 error → shouldFallback=true with computed cooldown
- 401 → shouldFallback=true (auth error, no retry)
- 500 → shouldFallback=true with backoff
- 400 (bad request to upstream) → shouldFallback=false (don't lock)
- resetsAtMs overrides backoff with precise timer
- Model lock key written to DB with correct expiry

**Dependencies**
- DB-xxx (connection update)

---

## PROV-019 — Clear Account Error on Success (clearAccountError)

**Description**
On successful request, clear the model lock for the model that just succeeded. Lazy-clean any other expired `modelLock_*` keys. Reset error state (`testStatus`, `lastError`, `lastErrorAt`, `backoffLevel`) only if no active locks remain.

**Input**
- `connectionId`, `currentConnection` (credentials object with `_connection` or raw connection), `model` (the model that succeeded)

**Output**
- DB update with cleared lock keys + optional error state reset
- No-op if no active locks and no error state

**Test Strategy**
- Successful model request clears that model's lock
- All expired locks cleaned
- Error state reset only when no active locks remain
- No-op when nothing to clear

**Dependencies**
- DB-xxx (connection update)

---

## PROV-020 — OAuth Token Refresh (checkAndRefreshToken)

**Description**
Check if OAuth token needs refresh (within 5-minute buffer before expiry). If needed, call provider-specific refresh endpoint. Persist refreshed tokens via `updateProviderCredentials`. Used in the chat handler after account selection.

**Input**
- `provider` ID, `credentials` object with `{ accessToken, refreshToken, expiresAt, connectionId, providerSpecificData }`

**Output**
- Updated credentials object (same structure as input, with potentially new `accessToken`, `refreshToken`, `expiresAt`, `projectId`)
- Unchanged if no refresh needed

**Test Strategy**
- Non-expired token within buffer: no refresh
- Expired token: refresh called, new tokens returned
- Refresh failure: return original credentials (caller handles downstream)
- Supported providers: all OAuth providers with refresh endpoints

**Dependencies**
- PROV-013 (refresh implementations), DB-xxx (updateProviderCredentials)

---

## Group F: Account Priority and Rate-Limit Tracking

---

## PROV-021 — Account Selection Strategy (fill-first vs round-robin)

**Description**
Implement two account selection strategies:
- **fill-first**: Return the first available connection (already sorted by `priority` in DB query). Simple, deterministic.
- **round-robin (sticky)**: Keep using the same account for `stickyLimit` consecutive requests. Track via `lastUsedAt` and `consecutiveUseCount` in DB. After limit, switch to least-recently-used account and reset count.

Strategy resolved from settings: global `fallbackStrategy` setting with per-provider override `providerStrategies[providerId].fallbackStrategy`.

Also support `preferredConnectionId` pin: if set and connection is available, always select it (skip strategy).

**Input**
- `availableConnections` list, `strategy` ("fill-first"|"round-robin"), `stickyLimit`, `preferredConnectionId`

**Output**
- Selected connection object
- Side effect: `lastUsedAt` and `consecutiveUseCount` updated in DB

**Test Strategy**
- Fill-first always returns first connection by priority
- Round-robin sticky: same connection for N requests, then switches
- Round-robin LRU: selects least recently used after sticky limit
- Pin takes precedence over strategy

**Dependencies**
- DB-xxx (connection update for timestamp/count)

---

## PROV-022 — Model Lock Management (per-model rate-limit tracking)

**Description**
Implement model lock check (`isModelLockActive`) and lock key management. Model lock keys: `modelLock_${model}` for per-model locks, `modelLock___all` for account-level locks. Check: if lock key value is a future timestamp, the connection is locked for that model. Use `getEarliestModelLockUntil` to find the minimum remaining lock time across all connections (for retry-after calculation).

**Input**
- Connection object, model string (or null for account-level)

**Output**
- `isModelLockActive(connection, model) bool` — true if any active lock for this model
- `getEarliestModelLockUntil(connections, model) timestamp|null` — earliest expiry across connections

**Test Strategy**
- Lock key with future expiry: isModelLockActive → true
- Lock key with past expiry: isModelLockActive → false (expired)
- No lock keys: isModelLockActive → false
- Mixed locks (some expired, some active): only active considered
- getEarliestModelLockUntil returns earliest active lock timestamp

**Dependencies**
- PROV-018, PROV-019

---

## PROV-023 — Rate-Limit Cooldown with Exponential Backoff

**Description**
Implement `checkFallbackError(status, errorText, backoffLevel)` to determine if an error should trigger account fallback and compute cooldown. Backoff levels 0-3: 30s, 2m, 10m, 30m. Errors that always trigger fallback: 401, 403, 429 (with cooldown), 500-599. Errors that do not trigger fallback: 400 (bad request from us), 413, 422. Backoff level increments on repeated errors, resets to 0 after successful request (via PROV-019).

**Input**
- HTTP status code, error text, current backoff level (0-3)

**Output**
- `{ shouldFallback: bool, cooldownMs: number, newBackoffLevel: number }`

**Test Strategy**
- 429 → shouldFallback with computed cooldown
- 401/403 → shouldFallback with no cooldown (immediate fallback)
- 500 → shouldFallback with cooldown
- 400 → shouldFallback=false (don't lock)
- Backoff level increments on repeated errors
- MAX_RATE_LIMIT_COOLDOWN_MS cap (prevent excessive wait times)

**Dependencies**
- PROV-018, PROV-019

---

## PROV-024 — Exclude Connection IDs from Selection (retry loop)

**Description**
Implement the retry loop in chat handler: maintain `excludeConnectionIds` as a Set. When `markAccountUnavailable` returns `shouldFallback=true`, add the failed connectionId to the exclude set and loop. Reset when `clearAccountError` succeeds (or on success). Handle concurrent requests via mutex.

**Input**
- Current `excludeSet`, failed `connectionId`, `shouldFallback` bool

**Output**
- Updated `excludeSet` (add failed connection if shouldFallback)
- Loop continues while `availableConnections` is empty but `allRateLimited` is false

**Test Strategy**
- Failed connection excluded from next selection
- Multiple consecutive failures: all failed connections excluded
- Success: exclude set cleared for next request
- allRateLimited: short-circuit loop with retryAfter response

**Dependencies**
- PROV-017, PROV-018, PROV-019

---

## Group G: Suggested Models Endpoint

---

## PROV-025 — Suggested Models Endpoint (GET /api/providers/suggested-models)

**Description**
Fetch and filter free/subsidized models from external URLs based on type. Types:
- `"openrouter-free"`: Fetch `https://openrouter.ai/api/v1/models`, filter to models with `pricing.prompt === "0"` and `pricing.completion === "0"` and `context_length >= 200000`. Return `[{ id, name, contextLength }]` sorted by context length desc.
- `"opencode-free"`: Fetch `https://opencode.ai/zen/v1/models`, filter to models with `id.endsWith("-free")` or known free model IDs. Return `[{ id, name }]`.

Results cached. Non-2xx responses return empty array.

**Input**
- Query params: `url` (fetch URL), `type` (`"openrouter-free"` | `"opencode-free"`)

**Output**
- `{ data: [{ id, name, contextLength? }] }`

**Test Strategy**
- Valid type returns filtered model list
- Unknown type returns 400
- Missing url/type returns 400
- Non-2xx fetch returns `{ data: [] }`
- openrouter-free: filters out paid models correctly
- opencode-free: includes known free model IDs even without `-free` suffix

**Dependencies**
- None (external fetch only)

---

## Cross-Cutting Notes

- Provider definitions (constants) live in `shared/constants/providers.go` — structured as Go maps equivalent to the Node.js `AI_PROVIDERS` object
- Auth type detection: `authType` field on connection; if absent, derive from provider constants (FREE → free, OAUTH → oauth, APIKEY → apikey, WEB_COOKIE → cookie)
- Compatible provider IDs are generated at node creation time and stored as `openai-compatible-<nodeId>` etc. — no alias lookup needed for these
- OAuth token refresh uses the OAuth domain (oauth-integration.md) — PROV-013 delegates token refresh to that domain
- All proxy resolution (`resolveConnectionProxyConfig`) delegated to the tunnel domain
- Model lock keys stored as columns on the connection row (avoids JSON column complexity)
- The `providerSpecificData` JSON column holds provider-specific config: proxy settings, Azure config, Cloudflare accountId, region, etc.

## Domain Structure

```
domain/providers/
  model_resolution.go     # PROV-001, PROV-002, PROV-003, PROV-004, PROV-005
  crud.go                 # PROV-006, PROV-007, PROV-008, PROV-009, PROV-010
  validation.go           # PROV-012, PROV-013, PROV-014, PROV-015
  testing.go              # PROV-011, PROV-016, PROV-025
  credentials.go          # PROV-017, PROV-018, PROV-019, PROV-020
  selection.go            # PROV-021, PROV-022, PROV-023, PROV-024
  constants.go            # Provider registry, aliases, auth types, media configs
  integration_test.go     # Full end-to-end flow tests
```

## Summary Table

| ID | Task | Group | Key Input | Key Output |
|---|---|---|---|---|
| PROV-001 | Model string parsing | Model Resolution | `"anthropic/claude-sonnet-4"` | `{ provider, model }` |
| PROV-002 | Provider alias resolution | Model Resolution | `"ds"` | `"deepseek"` |
| PROV-003 | Provider registry validation | Model Resolution | Provider ID | Provider def or null |
| PROV-004 | Default model resolution | Model Resolution | Provider ID | Default model string |
| PROV-005 | Combo model resolution | Model Resolution | `"best:4o-mini,grok-3"` | `{ models, strategy }` |
| PROV-006 | List providers | CRUD | - | `[{ connection }]` |
| PROV-007 | Create provider | CRUD | Request body | `201 { connection }` |
| PROV-008 | Get provider by ID | CRUD | `id` | `{ connection }` or 404 |
| PROV-009 | Update provider | CRUD | `id`, partial body | `{ connection }` or 404 |
| PROV-010 | Delete provider | CRUD | `id` | `200` or 404 |
| PROV-011 | Test single connection | Testing | `id` | `{ valid, error, refreshed }` |
| PROV-012 | API key probe by provider | Testing | Connection | `{ valid, error }` |
| PROV-013 | OAuth connection test | Testing | Connection (oauth) | `{ valid, error, refreshed }` |
| PROV-014 | Compatible provider test | Testing | Connection (compatible) | `{ valid, error }` |
| PROV-015 | Cookie provider test | Testing | Connection (cookie) | `{ valid, error }` |
| PROV-016 | Test batch by mode | Testing | `{ mode, providerId? }` | `{ results, summary }` |
| PROV-017 | Get provider credentials | Credentials | Provider ID + excludeSet | Credentials or null |
| PROV-018 | Mark account unavailable | Credentials | connectionId + error | `{ shouldFallback, cooldownMs }` |
| PROV-019 | Clear account error | Credentials | connectionId + model | DB update |
| PROV-020 | OAuth token refresh | Credentials | Provider + credentials | Updated credentials |
| PROV-021 | Account selection strategy | Selection | Connections + strategy | Selected connection |
| PROV-022 | Model lock management | Selection | Connection + model | bool + earliest lock |
| PROV-023 | Rate-limit backoff | Selection | Status + backoff level | `{ shouldFallback, cooldownMs }` |
| PROV-024 | Exclude connection IDs | Selection | excludeSet + failed ID | Updated excludeSet |
| PROV-025 | Suggested models | Testing | `url` + `type` | `{ data: [...] }` |