# Executors Domain — Detailed Task Breakdown

**Domain:** Executors (19 providers + base + registry + translators + RTK)
**Source Base:** `/Users/alkaaf/project/9router/open-sse/executors/`
**Target:** `internal/executor/` + `internal/translator/` + `internal/rtk/`
**Phase:** Phase 3 (AI Proxy Engine), weeks 9-12
**Dependency On:** Domain: Infrastructure (config, DB)

---

## Architecture Overview

```
internal/executor/
├── registry.go              # Executor factory + provider map
├── base.go                  # Executor interface + BaseExecutor
├── base_test.go             # Tests for base implementation
│
├── openai.go                # OpenAI-compatible (reference)
├── github.go                # GitHub Copilot
├── grokweb.go               # Grok Web (SSO cookie)
├── qwen.go                  # Alibaba Qwen
├── perplexity.go            # Perplexity Web
├── commandcode.go           # CommandCode AI SDK NDJSON
├── antigravity.go           # Antigravity (Gemini format)
├── geminicli.go             # Gemini CLI
├── kiro.go                  # Kiro CodeWhisperer (AWS EventStream)
├── iflow.go                 # iFlow (HMAC-SHA256)
├── qoder.go                 # Qoder (COSY + WAF bypass)
├── azure.go                 # Azure OpenAI
├── vertex.go                # Google Vertex AI (SA JSON)
├── vertex_partner.go        # Vertex partner (OpenAI-compatible)
├── ollama.go                # Local Ollama
├── opencode.go              # OpenCode
├── opencode_go.go           # OpenCode Go
├── cursor.go                # Cursor (HTTP/2 Protobuf)
└── codex.go                 # OpenAI Codex (Responses API)

internal/translator/
├── translator.go            # Translator interface + pipeline orchestrator
├── anthropic.go             # Anthropic ↔ OpenAI
├── gemini.go                # Gemini ↔ OpenAI
├── responses.go             # Responses API ↔ Chat Completions
└── kiro.go                  # AWS EventStream handling

internal/rtk/
├── rtk.go                   # RTK orchestrator (compressMessages entry point)
├── caveman.go               # Caveman compression strategy
├── autodetect.go            # Auto-detect filter
├── apply_filter.go          # Safe filter application (panic-safe)
└── filters.go              # Filter implementations
```

## Dependency Graph (executor groups)

```
Base Interface
  ├── OpenAI-Compatible Group ──────────────────┐
  │   ├── openai (reference)                    │
  │   ├── github (overrides)                    │
  │   ├── grok-web (completely custom execute)  │
  │   ├── qwen (extends default)                │
  │   ├── perplexity-web (completely custom)    │
  │   └── commandcode (NDJSON transform)        │
  ├── OAuth-Based Group ───────────────────────┤
  │   ├── antigravity (fully custom)            │
  │   ├── gemini-cli (Gemini format)            │
  │   ├── kiro (AWS EventStream binary)         │
  │   ├── iflow (HMAC-SHA256 signature)         │
  │   └── qoder (COSY + WAF encoding)           │
  ├── Custom Format Group ─────────────────────┤
  │   ├── azure (extends default)               │
  │   ├── vertex (SA JSON JWT)                  │
  │   ├── ollama-local (extends default)        │
  │   ├── opencode / opencode-go (dual format)  │
  │   ├── cursor (HTTP/2 + Protobuf)            │
  │   └── codex (Responses API)                 │
  └── default (catch-all for unknown providers) │

Translators            RTK
  ├── anthropic          ├── caveman
  ├── gemini             ├── autodetect
  ├── responses          ├── apply filter
  └── kiro eventstream   └── filters (11)

Chat Core (connects everything)
  ├── translateRequest → executor.execute → translateResponse
  └── Uses: translators + executors + RTK
```

---

## Phase 1: Base Infrastructure Tasks

### EXEC-001: Executor Interface + Base Implementation

**Description:**
Define the Go `Executor` interface and `BaseExecutor` struct that all provider executors will implement. Port the Node.js `BaseExecutor` class, which provides URL building, header construction, request transformation, retry logic with fallback URLs, credential refresh, and error mapping.

**Input Contract:**
- `executor` package with `Executor` interface and `BaseExecutor` struct
- `Executor` interface:
  ```go
  type Executor interface {
      Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error)
      GetProvider() string
      RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error)
      NeedsRefresh(creds *Credentials) bool
  }
  ```
- `BaseExecutor` struct with:
  - `provider string`, `config *ProviderConfig`
  - `BaseUrls() []string`, `FallbackCount() int`
  - `BuildUrl(model string, stream bool, urlIndex int, creds *Credentials) string`
  - `BuildHeaders(creds *Credentials, stream bool) map[string]string`
  - `TransformRequest(model string, body *Request, stream bool, creds *Credentials) *Request`
  - `ShouldRetry(status int, urlIndex int) bool` (429 + has fallback)
  - `Execute(ctx, req)` — main loop: iterate URLs, handle retries, return response
  - `ParseError(response *http.Response, body []byte) *ExecutorError`
  - `RefreshCredentials(ctx, creds) (*Credentials, error)` — no-op base
  - `NeedsRefresh(creds) bool` — check expiresAt

**Output Contract:**
- `internal/executor/base.go` — interface + base implementation
- `internal/executor/base_test.go` — unit tests

**Key Porting Decisions:**
- Replace `proxyAwareFetch` with standard `http.Client` (Go's transport handles proxy natively)
- Replace `AbortSignal.any` with `context.Context` cancellation + timeout
- Replace `let urlIndex = 0; let retryAttemptsByUrl = {}` with cleaner Go loop + counters
- Replace `DEFAULT_RETRY_CONFIG` with struct-based retry config
- Replace `FETCH_CONNECT_TIMEOUT_MS` with `context.WithTimeout`
- Replace `response.body.pipeThrough(TransformStream)` with `io.Pipe` or direct `io.Copy` (Go SSE model)

**Test Strategy:**
- Unit test `BuildUrl`, `BuildHeaders`, `TransformRequest`, `ShouldRetry`, `NeedsRefresh` as pure functions
- Table-driven test for `Execute`: mock HTTP server returns various status codes, verify retry behavior
- Test fallback URL iteration: 2+ URLs, first fails, second succeeds
- Test retry config: different status codes → different retry counts
- Test context cancellation: verify upstream request is aborted
- Error mapping table: 400→BadRequest, 401→AuthFailed, 403→Forbidden, 429→RateLimited, 500→Internal, 503→Unavailable

**Dependencies:**
- None (foundational)

**Effort Estimate:** 2 days

---

### EXEC-002: Executor Registry

**Description:**
Port the `executors/index.js` registry to Go. The registry maintains a `map[string]Executor` mapping provider names to executor instances, plus a `getExecutor()` function that returns the specialized executor or creates a `DefaultExecutor` for unknown providers.

**Input Contract:**
- Map of 19 known provider names → executor instances
- `getExecutor(provider string) Executor` function
- `hasSpecializedExecutor(provider string) bool`

**Output Contract:**
- `internal/executor/registry.go`
- Factory pattern: each executor is lazy-initialized on first access

**Key Porting Decisions:**
- Use `sync.Once` or `sync.Map` for thread-safe lazy initialization
- `DefaultExecutor` cache: `map[string]Executor` with `sync.RWMutex` is simpler than JavaScript's `Map` with closure

**Test Strategy:**
- `getExecutor("openai")` returns `*OpenAIExecutor`
- `getExecutor("unknown")` returns `*DefaultExecutor`
- `hasSpecializedExecutor("kiro")` returns `true`
- `hasSpecializedExecutor("unknown")` returns `false`
- Register + deregister test

**Dependencies:**
- EXEC-001 (needs Executor interface + BaseExecutor)

**Effort Estimate:** 0.5 day

---

### EXEC-003: Request/Response Model Types

**Description:**
Define Go structs for request/response payloads that flow through the executor pipeline. These mirror the OpenAI Chat Completions JSON structure, which is the canonical internal format.

**Input Contract:**
- Node.js `req.body` shape (Chat Completions JSON):
  - `model`, `messages[{role, content}], stream, max_tokens, temperature, tools, tool_choice, ...`
- Go structs must support JSON round-trip that matches upstream expectations
- `ExecuteRequest` struct: `{Model, Body, Stream, Credentials, Ctx, Log}`
- `ExecuteResponse` struct: `{StatusCode, Headers, Body io.ReadCloser, Usage}`

**Output Contract:**
- `internal/executor/types.go` containing:
  ```go
  type ExecuteRequest struct {
      Model       string
      Body        json.RawMessage   // Raw JSON, deserialized per executor
      Stream      bool
      Credentials *Credentials
      Ctx         context.Context
      Logger      *zap.Logger
  }

  type ExecuteResponse struct {
      Response   *http.Response
      URL        string
      Headers    map[string]string
      Body       io.ReadCloser
  }

  type ExecutorError struct {
      Status     int
      Message    string
      RetryAfter *time.Duration
      Code       string
  }

  type Credentials struct {
      AccessToken        string
      RefreshToken       string
      APIKey             string
      ExpiresAt          time.Time
      ProviderSpecific   map[string]interface{}
  }

  type RetryConfig struct {
      MaxRetries int
      DelayMs    int
  }
  ```

**Test Strategy:**
- JSON marshal/unmarshal round-trip tests for each struct
- Edge case: empty fields, null values, nested objects

**Dependencies:**
- EXEC-001 (types used by interface)

**Effort Estimate:** 0.5 day

---

## Phase 2: OpenAI-Compatible Group

### EXEC-004: OpenAI Executor (Reference Implementation)

**Description:**
Port the OpenAI executor, which is the simplest and most standardized executor. The Node.js codebase uses `DefaultExecutor` configured with openai provider config. This Go implementation will serve as the reference pattern for all other executors.

**Input Contract:**
- HTTP POST to `https://api.openai.com/v1/chat/completions`
- Auth: Bearer token (apiKey)
- Standard SSE response stream
- Body passthrough (no transformation needed)
- Standard error mapping

**Output Contract:**
- `internal/executor/openai.go`
- `OpenAIExecutor` struct extending `BaseExecutor`
- Override: `BuildUrl`, `BuildHeaders` (may be minimal since base handles standard Bearer)
- Passthrough `TransformRequest`

**Key Porting Decisions:**
- OpenAI API is the baseline — if base handles everything, this file may be very small
- SSE parsing: use Go's `bufio.Scanner` or dedicated SSE library
- Usage tracking: extract `usage` from final SSE chunk, pass to tracking service
- Tool calls streaming: accumulate delta chunks, emit complete tool_call on finish_reason

**Test Strategy:**
- Integration test with mock HTTP server returning SSE chunks
- Verify correct SSE event format
- Verify tool_calls accumulation in streaming mode
- Verify usage token extraction
- Error mapping: 400, 401, 429, 500 → typed errors
- Table-driven test for body passthrough

**Dependencies:**
- EXEC-001, EXEC-002, EXEC-003

**Effort Estimate:** 2 days

---

### EXEC-005: GitHub Copilot Executor

**Description:**
Port the `GithubExecutor`. This is significantly more complex than a simple OpenAI wrapper. It handles:
- Copilot-specific headers (copilot-integration-id, editor-version, etc.)
- Model-specific transforms (max_completion_tokens for gpt-5+, temperature for gpt-5.4, thinking for Claude)
- Dual routing: /chat/completions first, fallback to /responses for codex models
- Sanitizes messages (strips tool-related content types for Claude models on Copilot)
- Copilot token refresh (GitHub OAuth → Copilot Token)
- Response translation (Responses API SSE → OpenAI Chat SSE)

**Input Contract:**
- HTTP POST to `https://api.githubcopilot.com/chat/completions` or `/responses`
- Auth: `copilotToken` (refreshed from GitHub OAuth token)
- Model detection: which models need /responses endpoint
- Message sanitization: strip non-text/image_url content types

**Output Contract:**
- `internal/executor/github.go`
- `GithubExecutor` struct
- Overrides: `BuildHeaders`, `TransformRequest`, `Execute`, `RefreshCredentials`, `NeedsRefresh`
- Internal transform stream for Responses API → Chat Completions SSE

**Key Porting Decisions:**
- The `/responses` fallback logic (try /chat/completions → 400 → switch to /responses) needs careful state handling
- `knownCodexModels` set can be `map[string]bool` or `sync.Map` for thread safety
- The SSE-to-SSE transform (Responses API → Chat) should be a `io.Reader` wrapper
- `sanitizeMessagesForChatCompletions` — pure function, easy to port
- `initState` equivalent for Responses API (stateful streaming parser)

**Test Strategy:**
- Unit test each model detection: `requiresMaxCompletionTokens`, `supportsTemperature`, `supportsThinking`, `supportsReasoningEffort`, `supportsResponsesEndpoint`
- Unit test message sanitization with tool messages, Claude thinking content
- Integration test with mock Copilot server returning /chat/completions SSE
- Integration test with mock /responses endpoint → Chat SSE conversion
- Token refresh with mock OAuth server
- Edge: model name patterns (case-insensitive matching)

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 3 days

---

### EXEC-006: Grok Web Executor

**Description:**
Port the `GrokWebExecutor`. This is a completely custom executor that bypasses xAI's API and directly scrapes Grok's web interface:
- Custom Grok web chat API at `https://grok.com/rest/app-chat/conversations`
- SSO cookie-based auth (not Bearer token)
- Model map: `grok-3`, `grok-4`, `grok-4.1-mini` etc. mapped to internal model modes
- NDJSON event stream (not SSE) parsing
- Browser-like headers (User-Agent, Sec-CH-UA, traceparent)
- Statsig ID and x-request-id generation
- Completely custom `Execute()` — overrides everything, does not call super

**Input Contract:**
- HTTP POST to `https://grok.com/rest/app-chat/conversations` (or similar endpoint)
- Auth: `Cookie: sso=<token>` from credentials.apiKey
- Body: Grok-specific payload with modelMode, deviceEnvInfo, etc.
- Response: NDJSON (newline-delimited JSON, NOT SSE)

**Output Contract:**
- `internal/executor/grokweb.go`
- `GrokWebExecutor` struct
- Full override of `Execute()` — reads raw socket, parses NDJSON, emits OpenAI SSE chunks
- Helper functions: `parseOpenAIMessages`, `modelMap`, `generateStatsigId`, `randomHex`, `randomString`

**Key Porting Decisions:**
- The complex `buildStreamingResponse` / `buildNonStreamingResponse` can be merged into a single `ExecuteStream` / `Execute` pattern
- `readGrokNdjsonEvents` (async generator) → Go channel pattern: `generator(ctx) <-chan *GrokEvent`
- `extractContent` (async generator) → Go channel pipeline
- NDJSON parsing: `bufio.Scanner` with custom split func for newline-delimited JSON
- All the browser headers need to be faithfully reproduced (Sec-CH-UA, traceparent)
- Random string/hex functions need Go equivalents

**Test Strategy:**
- Unit test `parseOpenAIMessages`: system role, developer → system, text array, tool messages
- Unit test model map resolution (every known model key)
- Integration test with mock NDJSON server
- Verify OpenAI SSE output format matches spec
- Benchmark string parsing (NDJSON stream handling should not allocate excessively)

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 3 days

---

### EXEC-007: Qwen Executor

**Description:**
Port the `QwenExecutor` (extends `DefaultExecutor`). Qwen uses Alibaba's DashScope API with:
- Custom headers: `X-DashScope-AuthType`, `X-DashScope-CacheControl`, `X-Stainless-*` fingerprint headers
- Dynamic `buildUrl` using `resourceUrl` from OAuth response (not a static base URL)
- Injects default system message
- Thinking mode awareness (sanitize tool_choice when thinking enabled)
- OAuth refresh that captures `resource_url` from response

**Input Contract:**
- HTTP POST to `https://<resource_url>/v1/chat/completions`
- Auth: Bearer token (OAuth access token)
- Response: Standard SSE

**Output Contract:**
- `internal/executor/qwen.go`
- `QwenExecutor` struct extending base
- Overrides: `BuildUrl`, `BuildHeaders`, `TransformRequest`, `RefreshCredentials`

**Key Porting Decisions:**
- The `resourceUrl` from OAuth is critical — must be stored in credentials and used in BuildUrl
- `ensureQwenSystemMessage` injects system message — pure function
- `sanitizeQwenThinkingToolChoice` — pure function
- `isQwenThinkingActive` — boolean check

**Test Strategy:**
- Unit test `BuildUrl` with/without resourceUrl
- Unit test `BuildHeaders` — check all 12 custom headers
- Unit test `TransformRequest` — system message injection, thinking sanitization
- Integration test with mock Qwen SSE server
- Token refresh mock (verify resource_url capture)

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 1.5 day

---

### EXEC-008: Perplexity Web Executor

**Description:**
Port the `PerplexityWebExecutor`. This is a completely custom executor that interfaces with Perplexity's web SSE API:
- Custom Perplexity web API at `https://www.perplexity.ai/rest/sse/chat` or similar
- Cookie-based auth (`__Secure-next-auth.session-token`) or Bearer token
- Model map: pplx-auto, pplx-sonar, pplx-gpt, pplx-gemini, etc.
- Custom non-standard SSE format with `blocks` array structure
- Session cache (in-memory, TTL 1 hour, max 200 entries)
- Thinking mode support (search queries streamed as reasoning_content)
- Completely custom `Execute()` — overrides everything

**Input Contract:**
- HTTP POST to Perplexity web SSE endpoint
- Auth: Cookie or Bearer token
- Body: Perplexity-specific JSON with `query_str`, `params`, `search_focus`, `model_preference`
- Response: Perplexity's block-based SSE (not standard OpenAI SSE)

**Output Contract:**
- `internal/executor/perplexity.go`
- `PerplexityWebExecutor` struct
- Full override of `Execute()` — reads non-standard SSE, extracts blocks, emits OpenAI SSE

**Key Porting Decisions:**
- `readPplxSseEvents` → Go channel generator
- `extractContent` → Go channel pipeline with block parsing
- Session cache: `sync.RWMutex` + `map[string]*sessionEntry` with periodic cleanup via `time.Ticker`
- `parseOpenAIMessages`, `buildQuery`, `cleanResponse` — pure functions
- `buildPplxRequestBody` — construct Perplexity-specific payload

**Test Strategy:**
- Unit test `parseOpenAIMessages`: system message extraction, history building
- Unit test model map (MODEL_MAP + THINKING_MAP)
- Unit test `cleanResponse`: XML decl, citation marks, grok tags
- Unit test session cache: TTL expiry, max entries eviction
- Integration test with mock Perplexity SSE server
- Verify thinking streaming (search queries as reasoning_content)

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 3 days

---

### EXEC-009: CommandCode Executor

**Description:**
Port the `CommandCodeExecutor`. This executor talks to CommandCode's AI SDK v5 API which returns NDJSON (newline-delimited JSON, not SSE). It wraps the NDJSON stream into standard OpenAI SSE chunks.

**Input Contract:**
- HTTP POST to `https://api.commandcode.ai/alpha/generate`
- Auth: Bearer token (apiKey)
- Headers: `x-session-id` (UUID per request)
- Response: NDJSON lines (each line = AI SDK v5 event)

**Output Contract:**
- `internal/executor/commandcode.go`
- `CommandCodeExecutor` struct
- Override: `BuildHeaders` (adds x-session-id), `Execute` (wraps response)
- NDJSON → SSE transform via `io.Pipe` or custom reader

**Key Porting Decisions:**
- The `wrapNdjsonAsOpenAISse` transform is a `TransformStream` in JS → `io.Reader` wrapper in Go
- `convertCommandCodeToOpenAI` — pure function, translates one NDJSON event → one or more OpenAI SSE chunks
- Need `ndjson.Decoder` or custom `bufio.Scanner` split func

**Test Strategy:**
- Unit test NDJSON → SSE conversion with sample CommandCode events
- Integration test with mock NDJSON server
- Verify finish_reason and [DONE] marker emission

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 1 day

---

## Phase 3: OAuth-Based Executors

### EXEC-010: Antigravity Executor

**Description:**
Port the `AntigravityExecutor`. This is one of the most complex executors:
- Translates to Gemini format (not OpenAI)
- Google OAuth token refresh
- Heavy request transformation: sessionId, projectId, tool sanitization, function name cleaning
- Tool cloaking (anti-ban): renames client tools with `_ide` suffix, injects decoy tools
- URL fallback with retry: multiple base URLs, 429 retry with Retry-After header + error message parsing
- Custom URL path: `streamGenerateContent?alt=sse` vs `generateContent`

**Input Contract:**
- HTTP POST to Antigravity Gemini endpoint
- Auth: Bearer token (Google OAuth access token)
- Body: Gemini-format request wrapped in `{ project, model, request: <gemini-body>, ...}`
- Response: SSE (Gemini format: `data: {"candidates":[...]}`)
- Multiple fallback URLs

**Output Contract:**
- `internal/executor/antigravity.go`
- `AntigravityExecutor` struct
- Full override of `Execute()` — custom retry logic, fallback URLs, error message parsing
- Static methods: `CloakTools` (tool renaming + decoy injection)
- Google OAuth `RefreshCredentials`

**Key Porting Decisions:**
- `cloakTools` is a complex static method — port as standalone function or method
- Error message parsing (`parseRetryFromErrorMessage`) regex in Go
- `cleanJSONSchemaForAntigravity` — depends on geminiHelper, needs porting
- `deriveSessionId` — utility function dependency
- The complex URL retry loop with three disjoint retry counters (fallback URLs, Retry-After, auto retry)
- `AG_DECOY_TOOLS` (21 tools) — constants

**Test Strategy:**
- Unit test `BuildUrl`, `BuildHeaders`, `TransformRequest`
- Unit test `sanitizeFunctionName` (regex edge cases)
- Unit test `cloakTools` with various tool configurations
- Unit test `parseRetryHeaders`, `parseRetryFromErrorMessage`
- Unit test generated `sessionId`, `projectId`
- Integration test with mock Gemini SSE server
- Mock Google OAuth for refresh

**Dependencies:**
- EXEC-001, EXEC-003
- Translator: Gemini ↔ OpenAI (for format translation)

**Effort Estimate:** 3 days

---

### EXEC-011: Gemini CLI Executor

**Description:**
Port the `GeminiCLIExecutor`. Similar to Antigravity but specifically for Gemini CLI (Codey Assist):
- Gemini format (streamGenerateContent)
- Google OAuth refresh (same pattern as Antigravity)
- Cloud Code Assist wrapping: `{ project, model, request: <body> }`
- Custom `parseError` for Google API RetryInfo (429 with `type.googleapis.com/google.rpc.RetryInfo`)
- Model tracking for User-Agent header (`geminiCLIUserAgent(model)`)

**Input Contract:**
- HTTP POST to Gemini API endpoint
- Auth: Bearer token (Google OAuth)
- Body: wrapped in `{ project, model, request }` envelope
- Response: Gemini SSE format

**Output Contract:**
- `internal/executor/geminicli.go`
- `GeminiCLIExecutor` struct
- Overrides: `BuildUrl`, `BuildHeaders`, `TransformRequest`, `ParseError`, `RefreshCredentials`

**Key Porting Decisions:**
- `parseError` for RetryInfo: parse `@type` + `retryDelay` from Google error details
- `_currentModel` (instance variable) stored during `TransformRequest` for use in `BuildHeaders`

**Test Strategy:**
- Unit test `TransformRequest` wrapping
- Unit test `ParseError` with RetryInfo payload
- Unit test URL construction (stream vs non-stream)
- Integration test with mock Gemini server

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 1.5 day

---

### EXEC-012: Kiro Executor (AWS EventStream)

**Description:**
Port the `KiroExecutor`. This executor handles Amazon CodeWhisperer's binary AWS EventStream protocol:
- Binary EventStream frame parsing (not JSON, not SSE)
- Uses `proxyAwareFetch` for requests
- Headers: `Amz-Sdk-Request`, `Amz-Sdk-Invocation-Id` (UUID per request)
- Multiple event types: `assistantResponseEvent`, `reasoningContentEvent`, `codeEvent`, `toolUseEvent`, `messageStopEvent`, `contextUsageEvent`, `meteringEvent`, `metricsEvent`
- Token usage estimation from event metadata
- OAuth token refresh via `refreshKiroToken`

**Input Contract:**
- HTTP POST to Kiro's AWS EventStream endpoint
- Auth: Bearer token (Kiro OAuth access token)
- Headers: AWS SDK headers
- Response: Binary AWS EventStream frames (not SSE, not JSON)
- Custom retry configuration per status code

**Output Contract:**
- `internal/executor/kiro.go`
- `KiroExecutor` struct
- Full override of `Execute()` — custom retry logic + binary stream parsing
- AWS EventStream frame parser (`parseEventFrame`)
- Binary → SSE transform

**Key Porting Decisions:**
- AWS EventStream binary format: 4-byte big-endian total length, 4-byte headers length, headers, payload, 4-byte CRC
- No standard Go library for AWS EventStream — may need custom binary parser
- The `TransformStream` pipe-through pattern → `io.Pipe` + goroutine
- `refreshKiroToken` — depends on external service function, needs interface abstraction
- Token estimation: content length / 4 for output tokens, contextUsagePercentage × 200,000 for input tokens

**Test Strategy:**
- Unit test AWS EventStream frame parsing with crafted binary data
- Unit test each event type → SSE output format
- Unit test token estimation logic
- Integration test with mock Kiro server
- Edge: partial frames, split across TCP segments

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 4 days

---

### EXEC-013: iFlow Executor

**Description:**
Port the `IFlowExecutor`. iFlow uses HMAC-SHA256 request signing:
- Signature: `HMAC-SHA256(userAgent + ":" + sessionID + ":" + timestamp, apiKey)`
- Headers: `session-id`, `x-iflow-timestamp`, `x-iflow-signature`
- Standard OpenAI-compatible request/response body
- OAuth token refresh (in DefaultExecutor, but could be extracted)

**Input Contract:**
- HTTP POST to iFlow API endpoint
- Auth: Bearer apiKey + HMAC signature
- Body: Standard OpenAI Chat Completions
- Response: Standard SSE

**Output Contract:**
- `internal/executor/iflow.go`
- `IFlowExecutor` struct
- Overrides: `BuildHeaders` (adds signature), `BuildUrl`, `TransformRequest`
- HMAC-SHA256 signing function

**Key Porting Decisions:**
- `crypto.hmac` → Go's `crypto/hmac`
- UUID generation → `github.com/google/uuid`
- `stream_options: { include_usage: true }` injection in `TransformRequest`

**Test Strategy:**
- Unit test HMAC signature generation
- Unit test header construction (session-id, timestamp, signature)
- Unit test `TransformRequest` stream_options injection
- Integration test with mock iFlow server

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 1 day

---

### EXEC-014: Qoder Executor

**Description:**
Port the `QoderExecutor`. This is a highly custom executor:
- COSY signing: RSA + AES + MD5 + ~17 headers (anti-WAF, anti-reverse-engineering)
- WAF bypass encoding: `qoderEncodeBody` transforms plain JSON into encoded format
- Custom request body format: `chat_context`, `model_config`, `business` blocks
- Non-standard SSE response: `{statusCodeValue, body}` wrapper envelope
- Response unwrapping: strip envelope → plain OpenAI SSE
- Live model config fetching from `/algo/api/v2/model/list`
- In-memory model config cache
- Heads: `X-Model-Key`, `X-Model-Source`

**Input Contract:**
- HTTP POST to `api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?Encode=1`
- Auth: COSY-signed headers (not Bearer token, not API key)
- Body: WAF-encoded (not plain JSON)
- Response: `{statusCodeValue, body}` SSE envelope

**Output Contract:**
- `internal/executor/qoder.go`
- `QoderExecutor` struct
- Full override of `Execute()` — request building, encoding, signing, response unwrapping
- Request body builder: `normalizeMessages`, `buildQoderRequestBody`, `stableHash`, `stableChatRecordId`
- SSE unwrapper: `wrapQoderSSE` (envelope → plain SSE)
- Model config cache with live fetch

**Key Porting Decisions:**
- Qoder encoding + COSY signing are in separate modules (`@/lib/qoder/encoding.js`, `@/lib/qoder/cosy.js`) — must be ported or reimplemented
- The encoding/signing logic is critical — errors here result in WAF blocks
- `getQoderModelConfig` + `resolveQoderModels` — in-memory cache with force-refresh fallback
- `model_config` structure is complex — must match upstream expectations exactly
- The `providerSpecificData` dependency (userId, machineId) is strict

**Test Strategy:**
- Unit test `normalizeMessages`: system message hoisting, text extraction
- Unit test `stableHash`, `stableChatRecordId` (deterministic)
- Unit test `wrapQoderSSE` envelope → SSE transformation
- Unit test `buildQoderRequestBody` with various inputs
- Integration test with model list fetch and chat SSE
- Edge: missing userId/machineId → graceful 401

**Dependencies:**
- EXEC-001, EXEC-003
- Qoder encoding library (separate port effort)

**Effort Estimate:** 4 days

---

## Phase 4: Custom Format Executors

### EXEC-015: Azure Executor

**Description:**
Port the `AzureExecutor` (extends `DefaultExecutor`). Azure OpenAI uses:
- Custom URL: `{endpoint}/openai/deployments/{deployment}/chat/completions?api-version={version}`
- Auth: `api-key` header (not Bearer token)
- Configuration from `providerSpecificData`: azureEndpoint, apiVersion, deployment, organization
- OpenAI-Organization header support
- Body passthrough (same as OpenAI)

**Input Contract:**
- HTTP POST to Azure OpenAI endpoint
- Auth: `api-key` header
- Response: Standard SSE (OpenAI-compatible)

**Output Contract:**
- `internal/executor/azure.go`
- `AzureExecutor` struct extending base
- Overrides: `BuildUrl`, `BuildHeaders`
- Passthrough `TransformRequest`

**Key Porting Decisions:**
- `azureEndpoint`, `apiVersion`, `deployment`, `organization` all from `providerSpecificData` or env vars
- Different auth header (`api-key` vs `Authorization: Bearer`)

**Test Strategy:**
- Unit test `BuildUrl` with various endpoint/version/deployment combos
- Unit test `BuildHeaders` with/without organization
- Integration test with mock Azure server

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 1 day

---

### EXEC-016: Vertex Executor

**Description:**
Port the `VertexExecutor`. This handles Google Cloud Vertex AI with two sub-modes:
1. **Vertex (Gemini):** SA JSON (Service Account) → JWT assertion → Bearer token; or raw API key
   - URL: `https://aiplatform.googleapis.com/v1/projects/{project}/locations/{location}/publishers/google/models/{model}:streamGenerateContent?alt=sse`
2. **Vertex Partner:** OpenAI-compatible endpoint for Llama, Mistral, DeepSeek, Qwen etc.
   - URL: `https://aiplatform.googleapis.com/v1/projects/{project}/locations/global/endpoints/openapi/chat/completions`
- Project ID resolution: sends probe request, parses `projects/{id}` from error message
- SA JSON parsing: extracts project_id, client_email, private_key
- Token refresh: mint JWT from SA → exchange for Bearer token

**Input Contract:**
- HTTP POST to Google Cloud Vertex AI endpoint
- Auth: Bearer token (from SA JWT) or API key in URL query
- Body: Gemini format (vertex) or OpenAI format (vertex-partner)
- Response: Gemini SSE or OpenAI SSE

**Output Contract:**
- `internal/executor/vertex.go`
- `VertexExecutor` struct
- Override: `BuildUrl` (complex per-mode), `BuildHeaders`, `Execute` (full override for SA JSON token minting), `RefreshCredentials`
- Project ID resolution function
- SA JSON → JWT → Bearer token flow

**Key Porting Decisions:**
- `parseVertexSaJson` → parse SA JSON, extract project_id
- `refreshVertexToken` → JWT creation + Bearer token exchange (depends on `golang-jwt` or cryptography libraries)
- The `vertex-partner` with raw API key uses `?key=` in URL, while SA JSON uses Bearer header
- Project ID cache: `map[string]string` with TTL
- The probe request (404 intentionally) for project resolution is a clever hack that must be preserved

**Test Strategy:**
- Unit test `BuildUrl` for both vertex and vertex-partner modes, with/without SA JSON, with/without API key
- Unit test `BuildHeaders` for both auth modes
- Unit test project ID resolution with mock 404 error response
- Unit test SA JSON parsing (valid, invalid, missing fields)
- Integration test with mock Vertex server
- Token refresh mock (SA JWT → Bearer)

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 3 days

---

### EXEC-017: Ollama Local Executor

**Description:**
Port the `OllamaLocalExecutor` (extends `DefaultExecutor`). Local-only:
- Custom URL: `{ollamaHost}/api/chat` (not `/v1/chat/completions`)
- No auth required
- Host resolved from credentials or environment
- Ollama-specific API format (not fully OpenAI-compatible)

**Input Contract:**
- HTTP POST to `http://localhost:11434/api/chat`
- No auth
- Body: Ollama-specific format (different from OpenAI)
- Response: Ollama-specific NDJSON

**Output Contract:**
- `internal/executor/ollama.go`
- `OllamaLocalExecutor` struct extending base
- Override: `BuildUrl`

**Key Porting Decisions:**
- The current Node.js shows it as a simple URL override on `DefaultExecutor`, but Ollama actually has a different API format (not full OpenAI compatible)
- Need to check if actual Ollama format translation is needed (response translator `ollama-to-openai.js` exists)
- The Go implementation should handle both URL override and format translation via translators

**Test Strategy:**
- Unit test `BuildUrl` with localhost default and custom host
- Integration test with mock Ollama server
- Verify format translation if translator exists

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 0.5 day (if format translation already handled by translators)

---

### EXEC-018: OpenCode / OpenCode Go Executor

**Description:**
Port both `OpenCodeExecutor` and `OpenCodeGoExecutor`. They connect to `opencode.ai/zen/` with:
- Dual endpoints: `/v1/chat/completions` (OpenAI format) or `/v1/messages` (Claude format)
- Model-based routing: some models use Claude format, others use OpenAI format
- Auth: `Bearer public` (OpenCode) or `x-api-key` (OpenCode Go)
- Custom header: `x-opencode-client: desktop`
- Reasoning content injection (via shared utility)

**Input Contract:**
- HTTP POST to OpenCode API
- Auth: Public (hardcoded `Bearer public`) or API key
- Headers: `x-opencode-client`
- Body: OpenAI or Claude format depending on model

**Output Contract:**
- `internal/executor/opencode.go`
- `OpenCodeExecutor` struct
- `internal/executor/opencode_go.go`
- `OpenCodeGoExecutor` struct
- Overrides: `BuildUrl`, `BuildHeaders`, `TransformRequest`

**Key Porting Decisions:**
- `injectReasoningContent` — utility in Node.js, needs porting as shared function
- Model routing: `CLAUDE_FORMAT_MODELS` set determines which models use /messages vs /chat/completions
- `OpenCodeExecutor` has hardcoded `Bearer public` — may need update or config
- OpenCode Go has different base path: `/zen/go/v1/`

**Test Strategy:**
- Unit test `BuildUrl` model routing
- Unit test `BuildHeaders` for both OpenAI and Claude format auth
- Integration test with mock OpenCode server
- Verify reasoning content injection

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 1 day (combined)

---

### EXEC-019: Cursor Executor

**Description:**
Port the `CursorExecutor`. This is one of the most complex executors:
- **HTTP/2** direct connection (not standard HTTP/1.1)
- **Protobuf binary format** (Connect RPC protocol) — not JSON, not SSE
- **Gzip decompression** of protobuf frames with flag detection (NONE=0x00, GZIP=0x01, TRAILER=0x02, GZIP_TRAILER=0x03)
- Machine ID-based auth with custom checksum headers (`buildCursorHeaders`)
- Custom cursor-specific request body (`generateCursorBody`)
- Tool call accumulation and streaming
- Composer model thinking mode
- Proxy support for fetch fallback (when proxy enabled)

**Input Contract:**
- HTTP POST to Cursor API (HTTP/2 only, with HTTP/1.1 fallback)
- Auth: accessToken + machineId + ghostMode → computed checksum headers
- Body: Custom Cursor-specific protobuf-encoded message
- Response: Binary protobuf frames with gzip-compressed payloads

**Output Contract:**
- `internal/executor/cursor.go`
- `CursorExecutor` struct
- Full override of `Execute()` — HTTP/2 client, protobuf parsing, gzip decompress
- `transformProtobufToSSE` / `transformProtobufToJSON` — binary → SSE/JSON
- `makeHttp2Request` / `makeFetchRequest` — dual transport
- Protobuf frame parser: 1-byte flags + 4-byte length + payload

**Key Porting Decisions:**
- Go's `net/http` does not support HTTP/2 client by default at the low level — use `golang.org/x/net/http2` or Go's built-in `http.Transport` with `h2c`
- Protobuf parsing: Cursor's custom protobuf format needs field extraction — may need proto definitions or use `protobuf/proto` with generated types
- Actually looking at the code more carefully: Cursor uses a custom binary frame format (1 byte flags, 4 bytes length, then payload), NOT standard protobuf encoding. The payload is gzip-compressed.
- The actual content extraction is done via `extractTextFromResponse` which is in `cursorProtobuf.js` — needs porting
- `buildCursorHeaders` depends on `cursorChecksum.js` — complex checksum computation
- HTTP/2 in Go: multiple goroutines per connection, careful cleanup needed
- The `decompressPayload` function with three fallback strategies (gzip, zlib, raw deflate)

**Test Strategy:**
- Unit test protobuf frame parsing with crafted binary data
- Unit test decompression with each flag type (GZIP, TRAILER, GZIP_TRAILER)
- Unit test `transformProtobufToSSE` with known frame sequences
- Unit test error detection (JSON error frames inside binary stream)
- Integration test with HTTP/2 mock server
- Integration test with HTTP/1.1 fetch fallback

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 4 days

---

### EXEC-020: Codex Executor (Responses API)

**Description:**
Port the `CodexExecutor`. This handles OpenAI Codex's Responses API:
- **Responses API** format (not Chat Completions) — Events-based SSE: `response.created`, `response.output_item.added`, `response.output_text.delta`, `response.completed`
- Non-standard SSE events: event types on separate lines (`event: response.output_text.delta`, `data: {...}`)
- TransformResponse: translate Responses API SSE → Chat Completions SSE
- Image prefetching: fetch remote URLs → inline as base64 data URIs
- Session ID management with 4-tier priority: body → assistant-text hash → workspaceId → machineId
- SSE overload detection: peeks first 4096 bytes for "server_is_overloaded" patterns
- Instructions injection: default Codex instructions if missing
- Model name suffix parsing: `gpt-5.3-codex-high` → effort level + model name
- `store: false` requirement, `prompt_cache_key`, `include: [reasoning.encrypted_content]`
- Response API field allowlist — strips unsupported fields

**Input Contract:**
- HTTP POST to `https://api.openai.com/v1/responses` (or `/responses/compact`)
- Auth: Bearer token
- Body: OpenAI Responses API format (not Chat Completions)
- Response: Events-based SSE (`event:` + `data:` lines)
- Allowlist: only `model, input, instructions, tools, tool_choice, stream, store, reasoning, service_tier, include, prompt_cache_key, client_metadata`

**Output Contract:**
- `internal/executor/codex.go`
- `CodexExecutor` struct
- Full override of `Execute()` — image prefetch, transform, SSE overload retry
- `TransformRequest` — full request builder with Responses API logic
- SSE overload peek + retry logic
- Response API SSE → Chat SSE conversion

**Key Porting Decisions:**
- The Responses API SSE format is radically different from Chat Completions — need a full SSE event parser
- The stateful parsing in `transformRequest` (sessionId, thinking config, etc.) must be done before the request
- `normalizeResponsesInput`, `convertSystemToDeveloperRole`, `stripStoredItemReferences`, `normalizeCodexTools` — all pure transformations
- Image prefetch: `fetchImageAsBase64` → network call per image, 15s timeout
- `resolveCacheSessionId` — in-memory session cache with TTL + periodic cleanup
- `CODEX_DEFAULT_INSTRUCTIONS` — constant string
- `CODEX_SSE_OVERLOADED_PATTERNS` — prefix scanning

**Test Strategy:**
- Unit test `TransformRequest` allowlist enforcement (all 27+ stripped fields)
- Unit test session ID resolution (all 4 tiers)
- Unit test model name suffix parsing (effort level extraction)
- Unit test `normalizeCodexTools` (function flattening, tool_choice cleanup)
- Unit test `convertSystemToDeveloperRole`, `stripStoredItemReferences`
- Integration test with mock Responses API server
- SSE overload detection test (peek first N bytes)
- Image prefetching test with mock image server

**Dependencies:**
- EXEC-001, EXEC-003

**Effort Estimate:** 4 days

---

## Phase 5: Translator Tasks

### EXEC-021: Anthropic ↔ OpenAI Translator

**Description:**
Port the bidirectional format translation between Anthropic (Claude) format and OpenAI (Chat Completions) format. This includes both request and response translation pipelines.

**Input Contract:**
- Request translators:
  - `claude-to-openai.js` — Claude messages → OpenAI messages
  - `openai-to-claude.js` — OpenAI messages → Claude messages
- Response translators:
  - `claude-to-openai.js` — Claude SSE → OpenAI SSE
  - `openai-to-claude.js` — OpenAI SSE → Claude SSE

**Translation Mapping:**
```
Claude → OpenAI:
  content blocks: text, tool_use, tool_result, thinking → message roles
  system prompt → system message  
  thinking blocks → reasoning_content delta
  tool_use → tool_calls

OpenAI → Claude:
  messages[] with roles → content blocks
  system message → system[]
  tool_calls → tool_use blocks
  tool results → tool_result blocks
  reasoning_content/thinking → thinking blocks
```

**Output Contract:**
- `internal/translator/translator.go` — Interface + pipeline orchestrator
- `internal/translator/anthropic.go` — Both directions
- `internal/translator/anthropic_test.go`

**Key Porting Decisions:**
- The `translateRequest` / `translateResponse` pipeline: `source → openai → target` (two-step via OpenAI intermediate)
- Need `Translator` interface: `TranslateRequest(req)`, `TranslateResponse(chunk, state)`
- Streaming state management for response translation (accumulate partial deltas)
- Tools block translation: function → tool_use, function_call → tool_use.name+arguments

**Test Strategy:**
- Unit test request translation: system prompts, multi-turn messages, tool definitions
- Unit test response translation: text deltas, thinking blocks, tool_use blocks
- Round-trip tests: Claude → OpenAI → Claude should preserve semantic meaning
- Streaming test: partial chunks, finish_reason, tool_calls accumulation
- Edge cases: empty content, null role, mixed block types

**Dependencies:**
- None (standalone package)

**Effort Estimate:** 3 days

---

### EXEC-022: Gemini ↔ OpenAI Translator

**Description:**
Port the bidirectional format translation between Google Gemini format and OpenAI format. Both request and response directions.

**Input Contract:**
- Request translators:
  - `gemini-to-openai.js` — Gemini contents → OpenAI messages
  - `openai-to-gemini.js` — OpenAI messages → Gemini contents
- Response translators:
  - `gemini-to-openai.js` — Gemini SSE → OpenAI SSE
  - `openai-to-antigravity.js` — OpenAI SSE → Antigravity SSE

**Translation Mapping:**
```
Gemini → OpenAI:
  contents[{role, parts}] → messages[{role, content}]
  parts: text, inline_data, function_call, function_response
  candidates → choices
  safety ratings → finish_reason

OpenAI → Gemini:
  messages + system → contents[{role, parts}]
  content → text part
  image_url → inline_data
  tool_calls → function_call
  tool results → function_response
```

**Output Contract:**
- `internal/translator/gemini.go` — Both directions
- `internal/translator/gemini_test.go`

**Key Porting Decisions:**
- Gemini's `parts` array is not equivalent to OpenAI's `content` array — careful mapping needed
- Gemini role mapping: `user`, `model` → `user`, `assistant`
- `function_response` in Gemini must be mapped to `tool` role in OpenAI
- Streaming: Gemini uses `data: {"candidates":[...]}` format with `finishReason` in base response

**Test Strategy:**
- Unit test request translation for both directions
- Unit test response translation for streaming chunks
- Round-trip tests
- Edge cases: empty contents, system instruction, safety ratings

**Dependencies:**
- EXEC-021 (same translator interface)

**Effort Estimate:** 2 days

---

### EXEC-023: Responses API ↔ Chat Completions Translator

**Description:**
Port the `responsesTransformer.js` which converts between OpenAI's Responses API SSE format and Chat Completions SSE format. This is used by the GitHub executor (for codex model fallback) and by the /v1/responses endpoint.

**Input Contract:**
- Source: Responses API SSE events (`event: response.created`, `data: {...}`)
- Target: Chat Completions SSE (`data: {"choices":[{"delta":...}]}`)
- Event types to handle:
  - `response.created` → start streaming
  - `response.output_item.added` → item start (message, function_call, reasoning)
  - `response.output_text.delta` → content delta
  - `response.reasoning_summary_text.delta` → reasoning_content delta
  - `response.function_call_arguments.delta` → tool_calls delta
  - `response.output_item.done` → item completion
  - `response.completed` → stream end
  - `response.in_progress` → status update

**Output Contract:**
- `internal/translator/responses.go`
- Stateful SSE event parser with accumulation of message/text/tool data
- `ResponsesToChatTransform` — stream transformation

**Key Porting Decisions:**
- The state object in `responsesTransformer.js` tracks 25+ fields — needs careful port
- The reasoning content handling (both native `reasoning_content` delta and `<think>` tag parsing)
- Sequence number tracking for event ordering
- `createResponsesLogger` is for debugging — may skip or implement as debug logger

**Test Strategy:**
- Unit test each event type → expected SSE output
- Integration test with sample Responses API trace
- Edge: interleaved events (thinking + text + tool_calls), partial deltas, finish_reason

**Dependencies:**
- None

**Effort Estimate:** 2 days

---

### EXEC-024: Kiro AWS EventStream Translator

**Description:**
Port the AWS EventStream binary format translator. This is already embedded inside `KiroExecutor.transformEventStreamToSSE`, but the parsing logic (`parseEventFrame`) should be extracted into a reusable translator for testability and reuse.

**Input Contract:**
- Binary AWS EventStream frames:
  - 4 bytes: total length (big-endian)
  - 4 bytes: headers length (big-endian)
  - Headers: name-length + name + type + value-length + value
  - Payload: JSON string
  - 4 bytes: CRC
- Event types: `assistantResponseEvent`, `reasoningContentEvent`, `codeEvent`, `toolUseEvent`, `messageStopEvent`, `contextUsageEvent`, `meteringEvent`, `metricsEvent`

**Output Contract:**
- `internal/translator/kiro.go`
- `ParseEventFrame(b []byte) (*EventFrame, error)` — raw binary parser
- `EventFrameToChunk(frame, state) []*OpenAIChunk` — event → SSE chunks
- `KiroEventStreamReader` — `io.Reader` that reads binary stream and emits OpenAI SSE

**Key Porting Decisions:**
- The binary parsing is delicate: incorrect byte offsets → corrupt output
- Payload CRC validation — skip for now, focus on correct parsing
- Token estimation from `contextUsageEvent` + `metricsEvent` — per-event logic
- The state tracking (endDetected, finishEmitted, hasToolCalls, seenToolIds) must be maintained

**Test Strategy:**
- Unit test `ParseEventFrame` with crafted binary packets for each event type
- Fuzz test: random binary data should not panic
- Unit test event → chunk conversion for each event type
- Integration test with full binary stream → SSE output

**Dependencies:**
- None

**Effort Estimate:** 2 days

---

## Phase 6: RTK (Response Transform Kit)

### EXEC-025: Caveman Context Compression

**Description:**
Port the RTK caveman compression logic. This is a context compression system that reduces LLM agent call costs by compressing long `tool_result` content in request bodies before they are sent to the provider.

**Input Contract:**
- `compressMessages(body, enabled)` function
- Handles multiple message shapes:
  1. OpenAI tool response: `{role: "tool", content: "string"}`
  2. Claude tool_result string: `{content: [{type: "tool_result", content: "string"}]}`
  3. Claude tool_result array: `{content: [{type: "tool_result", content: [{type: "text", text: "..."}]}]}`
  4. OpenAI Responses: `{type: "function_call_output", output: string | array}`
  5. Kiro format: `conversationState.history[].userInputMessage...toolResults[].content[].text`
- Compression threshold: `MIN_COMPRESS_SIZE` (minimum length to compress)
- Safety cap: `RAW_CAP` (maximum length that can be compressed)
- Auto-detect filter selection per block
- Safe application via `safeApply` (panic-safe, never returns empty)

**Output Contract:**
- `internal/rtk/rtk.go` — Main `CompressMessages` function
- `internal/rtk/caveman.go` — Compression strategy (placeholder for algorithm)
- Returns statistics: `{BytesBefore, BytesAfter, Hits[{Shape, Filter, Saved}]}`

**Key Porting Decisions:**
- The actual compression algorithm is in `caveman.js` (not fully read) — strategy pattern
- `autoDetectFilter` selects among 11 filters based on text content — pure function
- `safeApply` — panic recovery with `defer/recover`
- Statistics tracking for observability
- Kiro format compression has separate function path

**Test Strategy:**
- Unit test `CompressMessages` for all 5 message shapes
- Unit test safety: doesn't compress below MIN_COMPRESS_SIZE, doesn't touch RAW_CAP
- Unit test safety: never returns empty string, never grows input
- Unit test Kiro-specific compression
- Unit test stats output format
- Verify panic safety with intentionally broken filter

**Dependencies:**
- None (standalone)

**Effort Estimate:** 1.5 day

---

### EXEC-026: AutoDetect + Filters

**Description:**
Port the content-type auto-detection pipeline and all 11 filters. The autodetect system scans text and selects the best compression filter based on content patterns.

**Input Contract:**
- `autoDetectFilter(text)` — examines first N chars (`DETECT_WINDOW`), returns a filter function or nil
- 11 filters: `gitDiff`, `gitStatus`, `buildOutput`, `grep`, `find`, `dedupLog`, `ls`, `tree`, `smartTruncate`, `readNumbered`, `searchList`
- Regex-based detection via `RE_GIT_DIFF`, `RE_GIT_STATUS`, etc.
- Detection priority: git-diff → git-status → build-output → git-status(porcelain) → grep → find → tree → ls → search-list → read-numbered → dedup-log → smart-truncate → null
- `safeApply(fn, text)` — execute filter with panic recovery

**Output Contract:**
- `internal/rtk/autodetect.go` — Auto-detection logic
- `internal/rtk/apply_filter.go` — Safe filter application
- `internal/rtk/filters.go` — All 11 filter implementations
- Each filter: `func(text string) string` that returns compressed output

**Key Porting Decisions:**
- Each filter is a JavaScript function → Go function with filter metadata
- Regex patterns: fine-tune for Go's `regexp` syntax (differences in lookahead, etc.)
- Detection priority is strict — order matters (e.g., build-output before git-status to prevent cargo misdetection)
- `isPathLike`, `isGrepLine`, `isMostlyPorcelain` — helper functions
- `READ_NUMBERED_LINE_RE`, `SEARCH_LIST_HEADER_RE` — exported for tests in Node.js, keep as exported in Go

**Test Strategy:**
- Unit test each filter with typical input → expected output
- Unit test detection priority (ambiguous input goes to correct filter)
- Unit test `safeApply` panic recovery
- Unit test edge cases: empty input, unicode, very long lines, byte boundary
- Filter-specific tests:
  - gitDiff: valid diff vs unrelated text
  - gitStatus: porcelain format detection
  - grep: filename:line:content format
  - find: path-like lines without colons
  - tree: box-drawing characters
  - ls: permission string detection
  - readNumbered: `  N|content` pattern match ratio
  - smartTruncate: long text fallback

**Dependencies:**
- EXEC-025 (used by caveman compression)

**Effort Estimate:** 2 days

---

## Phase 7: Integration Tasks

### EXEC-027: Chat Core Integration

**Description:**
Wire executors, translators, and RTK into the chat completion handler. This creates the pipeline:
1. Translate request: client format (Anthropic/Gemini) → OpenAI format
2. Apply RTK caveman compression
3. Select executor via registry
4. Execute: HTTP request to provider
5. Translate response: provider format → client format
6. Stream SSE to client
7. Track usage

**Input Contract:**
- Chat handler receives `/v1/chat/completions` request
- Pipeline orchestration function in `handler/v1/chat.go`

**Output Contract:**
- Pipeline integration code (not in executor package, but in handler)
- `chatHandler.go` — orchestrates translation → RTK → executor → response translation

**Key Porting Decisions:**
- The Node.js `chatCore` module handles routing, credential management, combo strategies — not in scope for this domain
- The handler side is documented in Phase 4 (Streaming SSE) of the manifest
- Error propagation: executor errors → typed errors → HTTP responses

**Test Strategy:**
- Integration test: full pipeline with mocked executor
- E2E test: real HTTP request to `/v1/chat/completions` with mock provider

**Dependencies:**
- EXEC-001 through EXEC-026
- SSE streaming handler (Phase 4, separate domain)

**Effort Estimate:** 1 day

---

### EXEC-028: Error Mapping + Observability

**Description:**
Implement standardized error mapping from provider errors to HTTP status codes, and add structured logging/metrics throughout the executor pipeline.

**Input Contract:**
- Error mapping table (from manifest):
  - 400 → Bad request (invalid model, missing params)
  - 401 → Auth failed (invalid token, expired)
  - 403 → Forbidden (quota exceeded, region blocked)
  - 429 → Rate limited (retry after from header)
  - 500 → Internal error (provider down)
  - 503 → Service unavailable (all accounts failed)
- Provider-specific error parsing (GitHub, Antigravity, Gemini CLI, Codex, Grok)

**Output Contract:**
- `internal/executor/errors.go` — Error types + mapping
- Logging integration throughout the executor layer
- Usage tracking hooks

**Key Porting Decisions:**
- Error type hierarchy: `ExecutorError{Status, Message, RetryAfter, Code}`
- Provider error parsers as methods on each executor
- Logging via structured logger (zerolog/slog)

**Test Strategy:**
- Unit test error mapping for all 6 categories
- Unit test each provider's parseError method
- Table-driven test for status code → error type conversion

**Dependencies:**
- EXEC-001 (uses Executor interface)
- EXEC-004 through EXEC-020 (each has custom error parsing)
- EXEC-027 (integrated into pipeline)

**Effort Estimate:** 1 day

---

## Summary

| Task ID | Name | Effort | Dependencies |
|---------|------|--------|-------------|
| **EXEC-001** | Executor Interface + Base Implementation | 2d | — |
| **EXEC-002** | Executor Registry | 0.5d | EXEC-001 |
| **EXEC-003** | Request/Response Model Types | 0.5d | EXEC-001 |
| **EXEC-004** | OpenAI Executor (Reference) | 2d | EXEC-001, 002, 003 |
| **EXEC-005** | GitHub Copilot Executor | 3d | EXEC-001, 003 |
| **EXEC-006** | Grok Web Executor | 3d | EXEC-001, 003 |
| **EXEC-007** | Qwen Executor | 1.5d | EXEC-001, 003 |
| **EXEC-008** | Perplexity Web Executor | 3d | EXEC-001, 003 |
| **EXEC-009** | CommandCode Executor | 1d | EXEC-001, 003 |
| **EXEC-010** | Antigravity Executor | 3d | EXEC-001, 003 |
| **EXEC-011** | Gemini CLI Executor | 1.5d | EXEC-001, 003 |
| **EXEC-012** | Kiro Executor (AWS EventStream) | 4d | EXEC-001, 003 |
| **EXEC-013** | iFlow Executor | 1d | EXEC-001, 003 |
| **EXEC-014** | Qoder Executor | 4d | EXEC-001, 003 |
| **EXEC-015** | Azure Executor | 1d | EXEC-001, 003 |
| **EXEC-016** | Vertex Executor | 3d | EXEC-001, 003 |
| **EXEC-017** | Ollama Local Executor | 0.5d | EXEC-001, 003 |
| **EXEC-018** | OpenCode + OpenCode Go | 1d | EXEC-001, 003 |
| **EXEC-019** | Cursor Executor (HTTP/2 + Protobuf) | 4d | EXEC-001, 003 |
| **EXEC-020** | Codex Executor (Responses API) | 4d | EXEC-001, 003 |
| **EXEC-021** | Anthropic ↔ OpenAI Translator | 3d | — |
| **EXEC-022** | Gemini ↔ OpenAI Translator | 2d | — |
| **EXEC-023** | Responses API ↔ Chat Translator | 2d | — |
| **EXEC-024** | Kiro EventStream Translator | 2d | — |
| **EXEC-025** | RTK Caveman Compression | 1.5d | — |
| **EXEC-026** | RTK AutoDetect + Filters | 2d | EXEC-025 |
| **EXEC-027** | Chat Core Integration | 1d | EXEC-001..026 |
| **EXEC-028** | Error Mapping + Observability | 1d | EXEC-001, 004..020 |
| | **Total** | **54 days** | |

## Complexity Tiers

**Tier 1: Trivial (0.5-1d)**
- EXEC-002 Registry
- EXEC-003 Types
- EXEC-009 CommandCode
- EXEC-013 iFlow
- EXEC-015 Azure
- EXEC-017 Ollama
- EXEC-018 OpenCode pair

**Tier 2: Moderate (1.5-2d)**
- EXEC-001 Base
- EXEC-004 OpenAI (reference)
- EXEC-007 Qwen
- EXEC-011 Gemini CLI
- EXEC-021 Anthropic Translator
- EXEC-022 Gemini Translator
- EXEC-023 Responses Translator
- EXEC-024 Kiro Translator
- EXEC-025 RTK Caveman
- EXEC-026 RTK Filters
- EXEC-027 Integration
- EXEC-028 Errors

**Tier 3: Complex (3d)**
- EXEC-005 GitHub
- EXEC-006 Grok Web
- EXEC-008 Perplexity Web
- EXEC-010 Antigravity
- EXEC-016 Vertex

**Tier 4: Very Complex (4d)**
- EXEC-012 Kiro (binary eventstream)
- EXEC-014 Qoder (COSY + WAF)
- EXEC-019 Cursor (HTTP/2 + protobuf)
- EXEC-020 Codex (Responses API)

## Risk Mitigation

1. **Kiro binary format** (EXEC-012): No standard Go library for AWS EventStream. Mitigation: custom binary parser with thorough unit tests.
2. **Qoder COSY signing** (EXEC-014): RSA+AES+MD5 multi-step signing + proprietary encoding. Mitigation: extract encoding/signing as testable pure functions first.
3. **Cursor HTTP/2 protobuf** (EXEC-019): Custom binary frames + gzip with multiple decompression fallbacks. Mitigation: baseline test with synthetic frames before real API integration.
4. **Responses API state machine** (EXEC-020, EXEC-023): Complex 25+ field state tracking for event accumulation. Mitigation: pure-function state reducers for testability.
5. **OAuth token refresh** (multiple): Each provider has different refresh endpoints and response shapes. Mitigation: `RefreshCredentials` is method on each executor, testable with mock OAuth servers.
6. **Web scraping fragility** (EXEC-006 Grok, EXEC-008 Perplexity): Browser-like headers and session management. Mitigation: integration tests with recorded responses; document pattern for cookie/SSO auth renewal.
