---
id: EXEC-009
domain: executors
status: DONE
estimate: 3d
title: Grok Web Executor
---

## Description

Port the `GrokWebExecutor`. This is a completely custom executor that bypasses xAI's API and directly scrapes Grok's web interface. Custom SSO cookie-based auth, NDJSON event stream parsing, browser-like headers.

## Input

- HTTP POST to `https://grok.com/rest/app-chat/conversations` (or similar endpoint)
- Auth: `Cookie: sso=<token>` from credentials.apiKey
- Body: Grok-specific payload with modelMode, deviceEnvInfo, etc.
- Response: NDJSON (newline-delimited JSON, NOT SSE)

## Output

- `internal/executor/grokweb.go` — GrokWebExecutor struct
- Full override of `Execute()` — reads raw socket, parses NDJSON, emits OpenAI SSE chunks
- Helper functions: `parseOpenAIMessages`, `modelMap`, `generateStatsigId`, `randomHex`, `randomString`

## Logic

1. Define `GrokWebExecutor` struct
2. Implement `modelMap` — map Grok model names to internal model modes
3. Implement `generateStatsigId`, `randomHex`, `randomString` utilities
4. Override `Execute` — full custom execution
5. Implement NDJSON parsing via `bufio.Scanner` with custom split function
6. Implement `readGrokNdjsonEvents` → Go channel pattern
7. Implement NDJSON → OpenAI SSE transformation
8. Build browser-like headers (User-Agent, Sec-CH-UA, traceparent, x-request-id)

## Acceptance Criteria
- [ ] SSO cookie correctly set in Cookie header
- [ ] NDJSON stream correctly parsed (not SSE)
- [ ] OpenAI SSE chunks emitted from NDJSON events
- [ ] Model mapping resolves all known Grok model names
- [ ] Browser headers faithfully reproduced
- [ ] Statsig ID and request ID generation works

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Cookie auth | BuildHeaders with sso token | Cookie: sso={token} header |
| NDJSON parsing | NDJSON stream with events | Parsed event objects |
| Model mapping | "grok-3" | Correct modelMode |
| SSE transformation | NDJSON event | OpenAI SSE chunk |
| Browser headers | BuildHeaders | Sec-CH-UA, traceparent headers |
| Error handling | NDJSON error event | ExecutorError returned |
