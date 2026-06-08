# Chat/Core Domain — Task Breakdown

**Domain:** Chat/Core
**Criticality:** P0 (MOST CRITICAL — 9Router's primary endpoint)
**Source files analyzed:**
- `src/sse/handlers/chat.js` (main chat handler)
- `src/sse/handlers/fetch.js` (similar pattern, web fetch)
- `src/mitm/handlers/base.js` (fetchRouter, pipeSSE)
- `src/app/api/v1/chat/completions/route.js` (Next.js route)
- `open-sse/handlers/chatCore.js` (core chat handler)
- `open-sse/services/combo.js` (combo orchestration)
- `src/sse/services/auth.js` (credential rotation)
- `open-sse/utils/bypassHandler.js` (bypass/naming)
- `open-sse/utils/error.js` (error response)

**Total tasks:** 38 atomic tasks across 8 sub-areas.

---

## Task Index

### Sub-area A: Request Parsing & Validation (CHAT-A)
- [CHAT-001](#chat-001) JSON body parser with error handling
- [CHAT-002](#chat-002) API key extraction (Authorization + x-api-key)
- [CHAT-003](#chat-003) API key validation (requireApiKey enforcement)
- [CHAT-004](#chat-004) Request validation (model field, required fields)
- [CHAT-005](#chat-005) Request enrichment (clientRawRequest, masked logging)

### Sub-area B: Model Resolution & Routing (CHAT-B)
- [CHAT-006](#chat-006) Model string parser (provider/model format)
- [CHAT-007](#chat-007) Model alias resolver (local DB)
- [CHAT-008](#chat-008) Provider-node prefix matching
- [CHAT-009](#chat-009) Combo detection (is this a combo name?)
- [CHAT-010](#chat-010) Routing logger (alias → actual model)

### Sub-area C: Bypass & Naming Handler (CHAT-C)
- [CHAT-011](#chat-011) Bypass pattern detector (warmup, count, skip)
- [CHAT-012](#chat-012) CC naming request handler
- [CHAT-013](#chat-013) Bypass response builder (streaming + non-streaming)

### Sub-area D: Credential Rotation & Fallback (CHAT-D)
- [CHAT-014](#chat-014) Account selection (fill-first strategy)
- [CHAT-015](#chat-015) Account selection (round-robin with sticky limit)
- [CHAT-016](#chat-016) Mutex-based selection race-condition guard
- [CHAT-017](#chat-017) Token expiry checker (proactive refresh)
- [CHAT-018](#chat-018) Fallback loop controller
- [CHAT-019](#chat-019) Mark account unavailable (backoff calculation)
- [CHAT-020](#chat-020) Clear account error (on success)
- [CHAT-021](#chat-021) Free provider (no-auth) injection

### Sub-area E: Combo Orchestration (CHAT-E)
- [CHAT-022](#chat-022) Combo strategy selector (global vs combo-specific)
- [CHAT-023](#chat-023) Fallback strategy executor
- [CHAT-024](#chat-024) Round-robin strategy with sticky counter
- [CHAT-025](#chat-025) Combo error aggregator (last error, earliest retry)
- [CHAT-026](#chat-026) Combo state persistence (in-memory rotation map)

### Sub-area F: Streaming SSE Response Writer (CHAT-F)
- [CHAT-027](#chat-027) SSE writer primitive (chunk format)
- [CHAT-028](#chat-028) Fiber SetBodyStreamWriter integration
- [CHAT-029](#chat-029) Client disconnect detection
- [CHAT-030](#chat-030) OpenAI → Anthropic/Gemini response translation
- [CHAT-031](#chat-031) Forced SSE-to-JSON converter

### Sub-area G: Usage Tracking Per Request (CHAT-G)
- [CHAT-032](#chat-032) Pending request tracker
- [CHAT-033](#chat-033) Usage history recorder (tokens, cost)
- [CHAT-034](#chat-034) Request detail logger (full metadata)

### Sub-area H: Handler Entry Point & Plumbing (CHAT-H)
- [CHAT-035](#chat-035) POST /v1/chat/completions Fiber route
- [CHAT-036](#chat-036) CORS preflight handler (OPTIONS)
- [CHAT-037](#chat-037) MITM integration (forward to Go server)
- [CHAT-038](#chat-038) Source format detector (by endpoint + body)

---

## Sub-area A: Request Parsing & Validation

### CHAT-001
**Description:** JSON body parser that wraps `c.BodyParser()` with explicit error handling. Must distinguish between malformed JSON (400) and empty body (400) with clear error messages matching the Node.js behavior (`"Invalid JSON body"`).

**Input/Output Contract:**
- Input: `*fiber.Ctx` with raw request body
- Output: Parsed `map[string]any` body struct OR error
- Error: 400 `{"error": {"message": "Invalid JSON body", "type": "invalid_request_error"}}`

**Test Strategy:**
- Unit: Valid JSON parses correctly
- Unit: Malformed JSON returns 400
- Unit: Empty body returns 400
- Unit: Array body (non-object) returns 400
- Contract: Error body matches Node.js response byte-for-byte

**Dependencies:** None (foundational)

---

### CHAT-002
**Description:** Extract API key from request headers. Mirrors Node.js `extractApiKey()` — checks `Authorization: Bearer <key>` first, then `x-api-key` header (Anthropic format). Returns `null` if neither is present.

**Input/Output Contract:**
- Input: `*fiber.Ctx`
- Output: `string` (API key) or `nil`
- Header priority: `Authorization: Bearer` > `x-api-key`

**Test Strategy:**
- Unit: Bearer token extracted correctly
- Unit: x-api-key header extracted correctly
- Unit: No auth header returns nil
- Unit: `Authorization: Basic ...` returns nil (only Bearer supported)
- Unit: Case-insensitive header matching (Go headers are canonical)

**Dependencies:** None

---

### CHAT-003
**Description:** Validate API key against the `requireApiKey` setting. If enabled, the request must have a valid key from the `apiKeys` table. Returns 401 if missing or invalid. Bypassed when `requireApiKey=false` (local mode).

**Input/Output Contract:**
- Input: `string` (API key), `bool` (requireApiKey setting)
- Output: `error` (nil if valid, error with status code otherwise)
- Lookup: `ApiKey` table by `key` column, check `isActive=true`

**Test Strategy:**
- Unit: Valid key + requireApiKey=true → passes
- Unit: Missing key + requireApiKey=true → 401 "Missing API key"
- Unit: Invalid key + requireApiKey=true → 401 "Invalid API key"
- Unit: requireApiKey=false → always passes
- Unit: Inactive key (isActive=false) → 401
- Integration: Real ApiKey table in SQLite

**Dependencies:** CHAT-002, ApiKey repository (Phase 1.3)

---

### CHAT-004
**Description:** Validate request body has required fields. Currently requires `model` field (string, non-empty). Returns 400 with clear message.

**Input/Output Contract:**
- Input: `map[string]any` (parsed body)
- Output: `error` (nil if valid)
- Required: `body["model"]` must be a non-empty string

**Test Strategy:**
- Unit: Missing model field → 400 "Missing model"
- Unit: Empty model string → 400
- Unit: Numeric model → 400
- Unit: Valid model → passes
- Unit: Null model → 400

**Dependencies:** CHAT-001

---

### CHAT-005
**Description:** Build a `clientRawRequest` struct for downstream logging. Captures endpoint path, parsed body, and sanitized headers. Used by MITM loggers, request detail records, and usage tracking.

**Input/Output Contract:**
- Input: `*fiber.Ctx`, `map[string]any` (parsed body)
- Output: `*ClientRawRequest` struct
- Fields: `endpoint string`, `body map[string]any`, `headers map[string]string`
- Sanitization: strip `Authorization`, `x-api-key` from headers (security)

**Test Strategy:**
- Unit: Endpoint extracted from URL path
- Unit: Body included verbatim
- Unit: Sensitive headers stripped
- Unit: Empty headers handled gracefully

**Dependencies:** None

---

## Sub-area B: Model Resolution & Routing

### CHAT-006
**Description:** Parse model string into `provider/model` components. Supports formats: `provider/model`, `model` (alias), `combo-name` (no slash). Returns structured `ParsedModel` with `provider`, `model`, `providerAlias`, `isAlias` flag.

**Input/Output Contract:**
- Input: `string` (model string)
- Output: `*ParsedModel` struct
- Format detection:
  - Contains `/` → `provider=part0, model=part1, isAlias=false`
  - No `/` → `isAlias=true, provider=""`

**Test Strategy:**
- Unit: `gpt-4` → isAlias=true
- Unit: `openai/gpt-4` → provider=openai, model=gpt-4
- Unit: `kc/claude-sonnet` → uses local alias override
- Unit: Empty string → error
- Unit: Trailing slash `openai/` → handles gracefully
- Unit: Multiple slashes `a/b/c` → handles as provider=a, model=b/c

**Dependencies:** None

---

### CHAT-007
**Description:** Resolve model alias from local DB. Loads `modelAliases` from settings, finds entry where key matches. Returns provider + model if found, or original parsed structure if not.

**Input/Output Contract:**
- Input: `string` (alias), `func() (map[string]AliasEntry, error)` (aliases loader)
- Output: `*ModelInfo` with `provider`, `model`
- Cache: In-memory map (TTL: 5 min, refreshed on miss)

**Test Strategy:**
- Unit: Known alias resolves correctly
- Unit: Unknown alias returns nil
- Unit: Empty aliases map returns nil
- Unit: Alias with nested provider/model structure
- Integration: Real settings table

**Dependencies:** CHAT-006, Settings repository

---

### CHAT-008
**Description:** Match `providerAlias` against `providerNodes` table (custom OpenAI-compatible, Anthropic-compatible, custom-embedding nodes). If a node's `prefix` matches the alias prefix, route to that node's ID with the model portion.

**Input/Output Contract:**
- Input: `*ParsedModel`, `func() ([]ProviderNode, error)` (node loader)
- Output: `*ModelInfo` or `nil`
- Matching: Exact `prefix` match against `providerAlias`
- Types checked: `openai-compatible`, `anthropic-compatible`, `custom-embedding`

**Test Strategy:**
- Unit: `myprefix/gpt-4` matches node with prefix=myprefix → returns node.ID, model=gpt-4
- Unit: No matching node → returns nil
- Unit: Multiple types of nodes (filters by type)
- Integration: Real providerNodes table

**Dependencies:** CHAT-006, ProviderNode repository

---

### CHAT-009
**Description:** Detect if the model string is a combo name (not a `provider/model` format). Returns the array of models in the combo, or `nil` if not a combo. Critical to check BEFORE alias resolution to prevent combo names from being routed as aliases.

**Input/Output Contract:**
- Input: `string` (modelStr), `func(string) (*Combo, error)` (combo lookup)
- Output: `[]string` (models array) or `nil`
- Rules: Reject if `modelStr` contains `/`. Reject if combo has 0 models.

**Test Strategy:**
- Unit: `my-combo` with models [a, b, c] → returns [a, b, c]
- Unit: `provider/model` → returns nil
- Unit: Unknown combo name → returns nil
- Unit: Empty combo (no models) → returns nil
- Integration: Real combos table

**Dependencies:** Combo repository (Phase 1.3)

---

### CHAT-010
**Description:** Log model routing decision. Two log formats:
- When alias was used: `[ROUTING] alias-name → provider/model`
- When direct: `[ROUTING] Provider: openai, Model: gpt-4`

Uses colorized output (`\x1b[32m`) for terminal readability.

**Input/Output Contract:**
- Input: `string` (original modelStr), `*ModelInfo` (resolved)
- Output: Log line to logger

**Test Strategy:**
- Unit: Aliased model produces alias → actual format
- Unit: Direct model produces Provider/Model format
- Unit: Color codes present in terminal output mode

**Dependencies:** None (logger is provided by Phase 1.2)

---

## Sub-area C: Bypass & Naming Handler

### CHAT-011
**Description:** Detect Claude CLI bypass patterns. Five patterns trigger bypass:
1. **Title extraction:** last message is `assistant` with content `"{"`
2. **Warmup:** first message text equals `"Warmup"`
3. **Count:** single user message with text `"count"`
4. **Skip patterns:** user text contains any pattern from `SKIP_PATTERNS` config
5. **CC naming:** system message contains `"isNewTopic"` (only if `ccFilterNaming=true`)

Returns a fake response struct (no upstream call) to save cost and rotation slots.

**Input/Output Contract:**
- Input: `map[string]any` (body), `string` (userAgent), `bool` (ccFilterNaming)
- Output: `*BypassResult` with `bypass bool`, `namingBypass bool` OR `nil`
- Gate: Only triggers when `userAgent` contains `"claude-cli"`

**Test Strategy:**
- Unit: Each of 5 patterns triggers bypass correctly
- Unit: Non-claude-cli user-agent → no bypass
- Unit: Multiple patterns → first match wins
- Unit: Empty messages array → no bypass
- Unit: `ccFilterNaming=false` → naming pattern ignored
- Contract: Bypass response matches Node.js `handleBypassRequest` output

**Dependencies:** None

---

### CHAT-012
**Description:** Generate a title for CC naming request. Extracts first 3 words from user message, returns `{"isNewTopic": true, "title": "..."}`. This mimics Claude Code CLI's topic naming behavior to avoid burning tokens on a real LLM call.

**Input/Output Contract:**
- Input: `[]Message` (messages), `string` (user text)
- Output: `string` (JSON-encoded title payload)
- Title format: First 3 whitespace-separated words, trimmed

**Test Strategy:**
- Unit: `"What is Python?"` → title `"What is Python"`
- Unit: Multi-word `"   hello   world   foo   bar  "` → title `"hello world foo"`
- Unit: Empty user text → empty title
- Unit: Unicode handling (Chinese, emoji)
- Contract: JSON output matches Node.js `namingText` format

**Dependencies:** CHAT-011

---

### CHAT-013
**Description:** Build bypass response (both streaming and non-streaming). Uses the translator to convert OpenAI-format fake response to source format (Claude, Gemini, etc.). Sends through the same translation pipeline as real responses.

**Input/Output Contract:**
- Input: `string` (sourceFormat), `string` (model), `string` (text), `bool` (stream)
- Output: `*BypassResponse` with `success bool`, `response *ChatResponse`
- Streaming: SSE-formatted chunks
- Non-streaming: Single JSON response (merged chunks)

**Test Strategy:**
- Unit: OpenAI source format returns OpenAI response directly
- Unit: Claude source format returns translated Claude response
- Unit: Gemini source format returns translated Gemini response
- Unit: Streaming mode produces multiple SSE chunks + `[DONE]`
- Unit: Non-streaming mode produces single JSON object
- Contract: Stream format matches Node.js byte-for-byte

**Dependencies:** CHAT-011, Translator module (Phase 3.7)

---

## Sub-area D: Credential Rotation & Fallback

### CHAT-014
**Description:** Select provider credentials using `fill-first` strategy. Filters out unavailable accounts (rate-limited, error-state, model-locked), picks highest-priority connection. Returns `nil` if no accounts available, or `{allRateLimited: true, ...}` if all are rate-limited.

**Input/Output Contract:**
- Input: `string` (provider), `Set[string]` (excludeIds), `string|null` (model)
- Output: `*Credentials` or `nil` or `*RateLimitState`
- Order: `ORDER BY priority ASC`
- Filters: `isActive=true`, not in excludeIds, not model-locked

**Test Strategy:**
- Unit: Returns highest-priority account
- Unit: Excludes connectionIds in excludeIds set
- Unit: Filters model-locked accounts
- Unit: Returns nil if no accounts
- Unit: Returns allRateLimited with earliest expiry if all locked
- Unit: Multiple accounts with same priority → first by ID
- Integration: Real providerConnections table

**Dependencies:** ProviderConnection repository (Phase 1.3)

---

### CHAT-015
**Description:** Select provider credentials using `round-robin` strategy with sticky limit. Stays on current account for N consecutive requests before rotating. Sort by `lastUsedAt` ascending (least recent first). Resets counter on rotation.

**Input/Output Contract:**
- Input: `string` (provider), `Set[string]` (excludeIds), `string|null` (model), `int` (stickyLimit)
- Output: `*Credentials`
- Sort: `lastUsedAt ASC NULLS FIRST, priority ASC`
- Side-effect: Updates `lastUsedAt` and `consecutiveUseCount` in DB (awaited for persistence)

**Test Strategy:**
- Unit: No lastUsedAt → picks first by priority
- Unit: Stays on current until stickyLimit reached
- Unit: Rotates to least-recently-used after limit
- Unit: Excludes model-locked accounts
- Unit: Increments consecutiveUseCount on stay, resets on rotate
- Integration: Real DB with lastUsedAt column

**Dependencies:** CHAT-014, ProviderConnection repository

---

### CHAT-016
**Description:** Mutex-based guard to prevent race conditions during account selection. Uses a single global mutex (channel-based in Go) to serialize selection across concurrent requests. Critical for round-robin accuracy.

**Input/Output Contract:**
- Input: None (internal lock)
- Output: Lock acquired/released
- Implementation: `sync.Mutex` or channel-based semaphore

**Test Strategy:**
- Unit: Concurrent calls (goroutines) — only one runs at a time
- Unit: Lock released on panic (defer)
- Benchmark: 100 concurrent calls, no duplicate selection

**Dependencies:** None

---

### CHAT-017
**Description:** Check if access token is expiring soon and refresh proactively. Checks `expiresAt` field, refreshes if remaining < provider-specific lead time. Also handles GitHub Copilot token expiry. Persists new credentials to DB.

**Input/Output Contract:**
- Input: `string` (provider), `*Credentials`
- Output: `*Credentials` (updated with new token)
- Refresh trigger: `expiresAt - now < refreshLeadMs`
- Provider-specific lead: openai (5min), anthropic (10min), etc.

**Test Strategy:**
- Unit: Expired token triggers refresh
- Unit: Token valid for >lead time → no refresh
- Unit: No expiresAt field → no refresh
- Unit: Refresh failure → returns original credentials
- Unit: GitHub Copilot secondary token refresh
- Unit: Background projectId refresh for antigravity/gemini-cli
- Integration: Real DB persistence

**Dependencies:** Token refresh module (Phase 3.6), ProviderConnection repository

---

### CHAT-018
**Description:** Main fallback loop controller. Iterates: get credentials → call executor → on error mark unavailable and retry with next account → on success return response. Stops when all accounts exhausted or non-fallbackable error encountered.

**Input/Output Contract:**
- Input: `*ChatContext` (body, modelInfo, settings, etc.)
- Output: `*ChatResponse` (success or final error)
- Loop bound: Max iterations = number of accounts + 1

**Test Strategy:**
- Unit: First account succeeds → returns immediately
- Unit: First fails (fallbackable), second succeeds → returns second's response
- Unit: All fail → returns last error
- Unit: Non-fallbackable error (400) → returns immediately
- Unit: Aborted request (client disconnect) → returns 499
- Integration: Mock executor with configurable behavior

**Dependencies:** CHAT-014, CHAT-015, CHAT-019, CHAT-020

---

### CHAT-019
**Description:** Mark account+model as unavailable. Calculates cooldown using exponential backoff OR precise `resetsAtMs` (provider-specific). Writes `modelLock_<model>` field with expiry timestamp. Increments `backoffLevel` for repeated failures.

**Input/Output Contract:**
- Input: `string` (connectionId), `int` (status), `string` (error), `string` (provider), `string` (model), `int64` (resetsAtMs)
- Output: `{shouldFallback bool, cooldownMs int}`
- Backoff: 5s, 30s, 2min, 10min, 1hr (exponential)
- Precise override: If `resetsAtMs > now`, use `resetsAtMs - now` (capped at `MAX_RATE_LIMIT_COOLDOWN_MS`)

**Test Strategy:**
- Unit: 429 status → fallback=true, cooldownMs > 0
- Unit: 400 status → fallback=false
- Unit: 500 status → fallback=true
- Unit: resetsAtMs overrides backoff
- Unit: Backoff level increases on consecutive errors
- Unit: modelLock_<model> written to connection
- Unit: noAuth connection (id="noauth") → no-op

**Dependencies:** ProviderConnection repository, errorConfig (Phase 1.2)

---

### CHAT-020
**Description:** Clear account error state on successful request. Removes `modelLock_<model>`, lazy-cleans expired locks, resets `testStatus` to "active" only if no remaining active locks.

**Input/Output Contract:**
- Input: `string` (connectionId), `*Connection` (current), `string` (model)
- Output: void
- Clears: `modelLock_<model>`, `modelLock___all`, expired locks
- Resets: `testStatus`, `lastError`, `lastErrorAt`, `backoffLevel` (only if no remaining locks)

**Test Strategy:**
- Unit: Successful model unlocks that model
- Unit: Expired locks auto-cleaned
- Unit: Active locks remain (don't reset testStatus)
- Unit: No-op if connection was already clean
- Unit: noAuth connection → no-op

**Dependencies:** ProviderConnection repository

---

### CHAT-021
**Description:** Inject virtual credentials for no-auth free providers (e.g., public endpoints). Returns a synthetic `Credentials` object with `connectionId="noauth"`, `accessToken="public"`. Resolves proxy config from settings.

**Input/Output Contract:**
- Input: `string` (provider), `*Settings` (providerStrategies, proxyPoolId)
- Output: `*Credentials` (synthetic)
- Trigger: `providerId` in `FREE_PROVIDERS` map with `noAuth=true`

**Test Strategy:**
- Unit: Free provider returns virtual credentials
- Unit: Auth-required provider returns nil
- Unit: Proxy config resolved from settings
- Unit: vercelRelayUrl populated from settings

**Dependencies:** Settings repository, proxy pool config

---

## Sub-area E: Combo Orchestration

### CHAT-022
**Description:** Resolve combo strategy. Priority order:
1. Per-combo override: `settings.comboStrategies[comboName].fallbackStrategy`
2. Global: `settings.comboStrategy`
3. Default: `"fallback"`

Same lookup for `comboStickyRoundRobinLimit`.

**Input/Output Contract:**
- Input: `string` (comboName), `*Settings`
- Output: `{strategy string, stickyLimit int}`
- Strategy values: `"fallback"`, `"round-robin"`

**Test Strategy:**
- Unit: Per-combo override wins
- Unit: Falls back to global
- Unit: Falls back to "fallback" default
- Unit: Sticky limit defaults to 1 if not set

**Dependencies:** Settings repository

---

### CHAT-023
**Description:** Execute fallback strategy: try models in order, on fallbackable error continue to next, on non-fallbackable error or success return. Tracks earliest retry-after across all attempts.

**Input/Output Contract:**
- Input: `[]string` (models), `func(string) (*Response, error)` (handler), `*Logger`
- Output: `*Response` (first success or final error)
- Per-attempt error filter: `checkFallbackError(status, errorText)`
- Transient wait: 503/502/504 with cooldownMs ≤ 5000 → wait before continuing

**Test Strategy:**
- Unit: First model succeeds → returns first
- Unit: First fails fallbackable, second succeeds → returns second
- Unit: All fail → returns last error
- Unit: Non-fallbackable (400) → returns immediately
- Unit: Transient 503 with cooldown → waits then retries
- Unit: earliestRetryAfter tracked across attempts

**Dependencies:** CHAT-022, accountFallback module (Phase 3.x)

---

### CHAT-024
**Description:** Round-robin combo executor with sticky counter. Rotates starting index per combo, increments consecutiveUseCount, advances to next index when limit reached. In-memory state map per combo name.

**Input/Output Contract:**
- Input: `[]string` (models), `string` (comboName), `int` (stickyLimit)
- Output: `[]string` (rotated model list)
- State: `map[string]*RotationState{index int, consecutiveUseCount int}`
- Fallback: Returns original list if strategy != "round-robin" or length ≤ 1

**Test Strategy:**
- Unit: Sticky limit=1 → always rotates
- Unit: Sticky limit=3 → stays for 3 calls then rotates
- Unit: Multiple combos tracked independently
- Unit: Reset clears state for a specific combo or all

**Dependencies:** CHAT-026

---

### CHAT-025
**Description:** Aggregate errors across all combo model attempts. Returns the worst-case status (last seen), concatenates error messages, finds earliest retry-after. Used when all models in a combo fail.

**Input/Output Contract:**
- Input: `[]AttemptResult` (status, errorText, retryAfter per attempt)
- Output: `*ComboError{status int, message string, retryAfter string|null}`
- Status selection: 503 if "no credentials" in any error, else first seen status, else 503

**Test Strategy:**
- Unit: Multiple errors aggregated correctly
- Unit: Earliest retry-after selected
- Unit: Status 503 when "no credentials" present
- Unit: Empty input → 503 with "All combo models unavailable"

**Dependencies:** CHAT-023

---

### CHAT-026
**Description:** Persist combo rotation state in-memory. Use `sync.Map` or `sync.RWMutex` + `map`. Key = combo name. Value = `{index int, consecutiveUseCount int}`. Must be goroutine-safe (concurrent requests on same combo).

**Input/Output Contract:**
- Input: `string` (comboName), `int` (next index), `int` (next count)
- Output: void
- API: `Get(comboName) *RotationState`, `Set(comboName, state)`, `Reset(comboName?)`
- Cleanup: Optional TTL for unused entries (future)

**Test Strategy:**
- Unit: Concurrent reads/writes are safe
- Unit: Reset clears all
- Unit: Get returns nil for unknown combo
- Benchmark: 1000 concurrent reads, no data races

**Dependencies:** None

---

## Sub-area F: Streaming SSE Response Writer

### CHAT-027
**Description:** Low-level SSE chunk writer. Format: `data: <json>\n\n`. Provides helpers for:
- Writing a chunk: `WriteChunk(model, delta, finishReason)`
- Writing done marker: `WriteDone()`
- Writing error: `WriteError(statusCode, message)`
- Flushing buffer after each write

Must use buffered writer + explicit flush for proper streaming.

**Input/Output Contract:**
- Input: `io.Writer` (Fiber bufio.Writer)
- Output: void
- Format: `data: <json>\n\n`, `data: [DONE]\n\n`

**Test Strategy:**
- Unit: Write chunk produces correct format
- Unit: Write done produces `data: [DONE]\n\n`
- Unit: Write error produces error event
- Unit: Multiple writes produce concatenated stream
- Contract: Output matches Node.js `formatSSE()` byte-for-byte

**Dependencies:** None

---

### CHAT-028
**Description:** Fiber `SetBodyStreamWriter` integration. Sets up headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`), then writes chunks via callback. Returns nil from handler immediately (response is async).

**Input/Output Contract:**
- Input: `*fiber.Ctx`, `func(*bufio.Writer)` (writer callback)
- Output: `error` (nil)
- Headers set before callback: Content-Type, Cache-Control, Connection, X-Accel-Buffering

**Test Strategy:**
- Unit: SSE headers set correctly
- Unit: Writer callback invoked
- Unit: JSON headers set for non-streaming
- Integration: Real Fiber app, curl with `--no-buffer` reads chunks

**Dependencies:** CHAT-027

---

### CHAT-029
**Description:** Detect client disconnect mid-stream. Uses `c.Context().Done()` channel. When triggered, abort upstream executor via `signal.Cancel()`. Triggers `onDisconnect` callback for usage tracking cleanup.

**Input/Output Contract:**
- Input: `context.Context`, `*StreamController`
- Output: void
- Triggers: `onDisconnect(reason)` callback
- Aborts: Upstream request via cancel signal

**Test Strategy:**
- Unit: Client disconnect → callback fires
- Unit: Upstream request cancelled
- Unit: Server-side close → no callback
- Unit: Normal completion → no callback
- Integration: Test with `c.Context().Close()` mid-stream

**Dependencies:** None

---

### CHAT-030
**Description:** Translate OpenAI streaming response to target source format (Anthropic, Gemini, etc.). For each OpenAI chunk, produces 0+ translated chunks in the source format. Handles content deltas, tool calls, finish reasons, usage.

**Input/Output Contract:**
- Input: `string` (sourceFormat), `*TranslationState`, `*OpenAIChunk` (or nil for flush)
- Output: `[]TranslatedChunk`
- State: Tracks accumulated content, tool calls, message metadata across chunks

**Test Strategy:**
- Unit: OpenAI → Claude content delta translation
- Unit: OpenAI → Gemini function call translation
- Unit: OpenAI → Antigravity (Gemini) translation
- Unit: Final flush produces done markers
- Unit: Tool calls accumulated across chunks
- Contract: Anthropic format output matches Node.js byte-for-byte

**Dependencies:** Translator module (Phase 3.7), TranslationState

---

### CHAT-031
**Description:** Convert forced SSE response (from OpenAI/codex/commandcode) to JSON when client requested non-streaming. Reads all SSE chunks, merges into single response object, parses usage from final chunk.

**Input/Output Contract:**
- Input: `*Response` (SSE stream from provider), `string` (sourceFormat)
- Output: `*Response` (single JSON response)
- Logic: Accumulate deltas, extract usage from last chunk with `usage` field, return merged object

**Test Strategy:**
- Unit: Single-chunk SSE → JSON
- Unit: Multi-chunk SSE → merged content
- Unit: Usage extracted from final chunk
- Unit: Error event in SSE → JSON error response
- Unit: Empty stream → empty choices

**Dependencies:** CHAT-030, parseUpstreamError

---

## Sub-area G: Usage Tracking Per Request

### CHAT-032
**Description:** Track pending requests in-memory for live dashboard. Increments counter on request start, decrements on completion/error. Used by `/api/usage/stream` SSE endpoint for real-time updates.

**Input/Output Contract:**
- Input: `string` (model), `string` (provider), `string` (connectionId), `bool` (increment=true)
- Output: void
- Storage: `sync.Map[string]*ProviderStats{pending int, total int}`

**Test Strategy:**
- Unit: Increment/decrement balance correctly
- Unit: Multiple concurrent calls are race-free
- Unit: Get current pending count
- Benchmark: 10K calls/sec, no lock contention

**Dependencies:** None

---

### CHAT-033
**Description:** Record usage history row after request completion. Fields: timestamp, provider, model, connectionId, apiKey, promptTokens, completionTokens, cost, status. Uses GORM bulk-insert for performance (batched every 5s or 100 rows).

**Input/Output Contract:**
- Input: `*UsageRecord` struct
- Output: `error` (nil on success)
- Persistence: `UsageHistory` table (batch insert)
- Cost calculation: `promptTokens * pricing.input + completionTokens * pricing.output`

**Test Strategy:**
- Unit: Single record inserted
- Unit: Batch insert (100 records in 1 query)
- Unit: Cost calculation matches pricing config
- Unit: Null fields handled (pointer types)
- Integration: Real DB, count rows after batch

**Dependencies:** UsageHistory repository (Phase 1.3), Pricing config

---

### CHAT-034
**Description:** Log full request detail to `requestDetails` table. Captures: latency (TTFT, total), tokens (prompt, completion), request config (model, stream, tools count), provider request/response, status, error message, thinking content.

**Input/Output Contract:**
- Input: `*RequestDetail` struct
- Output: `error` (nil on success)
- Triggered: On every request (success or error)
- Async: Fire-and-forget (`.catch(()=>{})` equivalent)

**Test Strategy:**
- Unit: Successful request detail saved
- Unit: Error request detail saved
- Unit: TTFT and total latency captured
- Unit: Thinking content included if present
- Integration: Real DB, verify row count

**Dependencies:** RequestDetail repository (Phase 1.3)

---

## Sub-area H: Handler Entry Point & Plumbing

### CHAT-035
**Description:** Main POST `/v1/chat/completions` Fiber route. Wraps `handleChat()` with:
- CORS headers
- Recovery middleware (catch panics → 500)
- Request ID injection
- Logging

Entry point delegates to the chat handler with parsed body and request context.

**Input/Output Contract:**
- Input: `*fiber.Ctx`
- Output: `error` (nil if response written)
- Path: `POST /v1/chat/completions`
- Middleware: `Recovery`, `RequestID`, `Logger`, `CORS`

**Test Strategy:**
- Unit: Valid chat request returns response
- Unit: Panic in handler → 500 response (recovery)
- Unit: Request ID present in response headers
- Integration: Real Fiber app, full chat flow

**Dependencies:** CHAT-001 through CHAT-031, middleware (Phase 1.4)

---

### CHAT-036
**Description:** CORS preflight handler (OPTIONS). Returns 204 with allow-all headers for browser-based clients. Matches Node.js `OPTIONS` handler in route.js.

**Input/Output Contract:**
- Input: `*fiber.Ctx`
- Output: 204 No Content
- Headers: `Access-Control-Allow-Origin: *`, `Allow-Methods: GET, POST, OPTIONS`, `Allow-Headers: *`

**Test Strategy:**
- Unit: OPTIONS request → 204
- Unit: CORS headers present
- Unit: No body in response

**Dependencies:** None

---

### CHAT-037
**Description:** MITM integration compatibility. The Node.js MITM server forwards requests to Go via `fetchRouter()` on `/v1/chat/completions`. Go server must accept the same forwarded headers, accept `application/json` content type, and return the same response format. No changes needed to MITM code — Go server just needs to be drop-in compatible.

**Input/Output Contract:**
- Input: HTTP POST from MITM with stripped headers
- Output: Same response as direct client
- Key: ROUTER_BASE env var in MITM must point to Go server URL

**Test Strategy:**
- Integration: Real MITM server points to Go server, verify requests proxied correctly
- Contract: Headers forwarded correctly (no host, no authorization stripped)
- Contract: Streaming response piped through correctly

**Dependencies:** CHAT-035, MITM server (no rewrite needed)

---

### CHAT-038
**Description:** Detect source format from endpoint + body. Returns one of: `openai`, `claude`, `gemini`, `openai-responses`, `antigravity`, `gemini-cli`. Used to skip re-detection in chatCore.

**Input/Output Contract:**
- Input: `string` (endpoint path), `map[string]any` (body)
- Output: `string` (format constant)
- Rules:
  - `/v1/chat/completions` + no `input` field → `openai`
  - `/v1/messages` or has `system` field → `claude`
  - `/v1/responses` or has `input` (string/array) → `openai-responses`
  - `/v1beta/models/*` → `gemini` or `antigravity`
  - Body has `contents` → `gemini`

**Test Strategy:**
- Unit: Each endpoint maps to correct format
- Unit: Body shape overrides endpoint guess
- Unit: Unknown endpoint → defaults to `openai`

**Dependencies:** None

---

## Cross-Cutting Concerns

### Performance Notes
- **Account selection hot path:** CHAT-014, CHAT-015 called per request. Cache `providerConnections` list in memory with TTL (5 min), invalidate on write.
- **Token refresh:** CHAT-017 is the slowest path. Use background refresh (don't block request).
- **Combo rotation:** CHAT-024 state is in-memory, must be `sync.Map` or `sync.RWMutex` guarded.

### Error Handling Patterns
- All handlers return `*ChatResponse` with explicit status codes.
- 4xx errors are client-fault, no retry.
- 5xx errors may be retried with different account.
- 429 = rate-limited, respect `Retry-After` header from upstream.
- 499 = client disconnect (use 499, not 500).

### Concurrency
- **Mutex per combo** (CHAT-026) — multiple combos can rotate independently.
- **Mutex per provider** (CHAT-016) — serialize account selection within a provider.
- **No global locks** on request path.

### Testing Strategy Summary
- 38 tasks, ~95% pure functions = easy unit testing.
- ~5 integration tests needed: chat with mock provider, combo fallback, error retry, SSE streaming, MITM integration.
- Contract tests verify Go matches Node.js byte-for-byte for SSE chunks and error responses.

### Dependency Order (Implementation Sequence)

```
Phase A (Foundations):
  CHAT-001, CHAT-002, CHAT-004, CHAT-005, CHAT-006, CHAT-010
  CHAT-027, CHAT-028, CHAT-036

Phase B (Resolution):
  CHAT-003, CHAT-007, CHAT-008, CHAT-009, CHAT-038

Phase C (Auth & Bypass):
  CHAT-011, CHAT-012, CHAT-013, CHAT-021

Phase D (Credentials):
  CHAT-014, CHAT-015, CHAT-016, CHAT-017, CHAT-019, CHAT-020

Phase E (Combo):
  CHAT-022, CHAT-023, CHAT-024, CHAT-025, CHAT-026

Phase F (Streaming):
  CHAT-029, CHAT-030, CHAT-031

Phase G (Usage):
  CHAT-032, CHAT-033, CHAT-034

Phase H (Integration):
  CHAT-018, CHAT-035, CHAT-037
```

### Estimated Effort

| Sub-area | Tasks | Total Days |
|----------|-------|-----------|
| A: Parsing | 5 | 2-3 |
| B: Resolution | 5 | 2-3 |
| C: Bypass | 3 | 1-2 |
| D: Credentials | 8 | 4-5 |
| E: Combo | 5 | 3-4 |
| F: Streaming | 5 | 4-5 |
| G: Usage | 3 | 2-3 |
| H: Integration | 3 | 2-3 |
| **Total** | **38** | **20-28 days** |

---

## Open Questions for Reviewer

1. **Provider node prefix matching:** Should CHAT-008 also support `anthropic-compatible` and `custom-embedding` types in v1, or defer to v2?
2. **Combo state persistence:** Should CHAT-026 state survive server restarts (persist to DB), or keep in-memory only? Current Node.js code is in-memory.
3. **Bypass patterns:** Are there any additional Claude CLI patterns beyond the 5 documented? Check upstream Claude Code CLI changelog.
4. **Cost calculation:** CHAT-033 cost calculation — should it use the `pricing` table from DB, or hardcoded rates per model? Recommend DB-driven.
5. **Streaming chunk size:** Should we batch multiple OpenAI deltas into one SSE write for efficiency, or flush per chunk? Node.js flushes per chunk.
