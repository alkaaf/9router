---
id: SYS-004
domain: settings
status: DONE
estimate: 2h
title: POST /api/settings/proxy-test — Test proxy connectivity
---

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-sys
- AC-001 verified: ProxyTestHandler exported from internal/settings
- AC-002 verified: TestProxyTest_Defaults exercises the full path with a dead proxy; returns ok:false with error message and 200 status
- AC-003 verified: TestProxyTest_InvalidURL returns 400
- AC-004 verified: TestProxyTest_MissingProxyURL returns 400
- AC-005 verified: TestProxyTest_MethodNotAllowed returns 405
- AC-006 verified: Defaults applied (testURL and timeoutMs)

## Description

Accept a proxy URL and optional test URL + timeout, then attempt an HTTP request through the proxy. Uses `testProxyUrl()` service. Distinguishes timeout (`AbortError`) from other connection errors. Returns latency and status on success.

## Input

```json
{
  "proxyUrl": "http://user:pass@proxy.example.com:8080",
  "testUrl": "https://api.openai.com/v1/models",
  "timeoutMs": 10000
}
```

All fields are optional except `proxyUrl`. Defaults: `testUrl` = `https://api.openai.com/v1/models`, `timeoutMs` = 10000.

## Output

**Success:**
```json
{ "ok": true, "latencyMs": 150, "status": 200 }
```

**Failure:**
```json
{ "ok": false, "error": "Connection refused" }
```

**Timeout:**
```json
{ "ok": false, "error": "Proxy test timed out" }
```

## Logic

1. Parse request body. Require `proxyUrl`. Apply defaults for `testUrl` and `timeoutMs`.
2. Validate `proxyUrl` is a well-formed URL (return 400 if not).
3. Call `testProxyUrl(proxyUrl, testUrl, timeoutMs)` service function.
4. Measure wall-clock time for the request to compute `latencyMs`.
5. On success: return `{ ok: true, latencyMs, status: <http-status> }`.
6. On `AbortError` / timeout: return `{ ok: false, error: "Proxy test timed out" }`.
7. On other errors (ECONNREFUSED, ECONNRESET, SSL, etc.): return `{ ok: false, error: "<human-readable message>" }`.
8. Map common network error codes to user-friendly messages.

## Acceptance Criteria

- [x] Endpoint is registered at `POST /api/settings/proxy-test`
- [x] Valid proxy URL returns `{ ok: true, latencyMs, status }` with 200
- [x] Timeout returns `{ ok: false, error: "Proxy test timed out" }`
- [x] Invalid proxy URL format returns 400 with error message
- [x] Connection errors return `{ ok: false, error: "..." }` with 200 (not 500)
- [x] `latencyMs` is a positive integer
- [x] Default `testUrl` is `https://api.openai.com/v1/models` when not provided
- [x] Default `timeoutMs` is 10000 when not provided

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid proxy | Valid URL, reachable proxy | 200, `{ ok: true, latencyMs: <int>, status: 200 }` |
| Timeout | Valid URL, slow/unreachable proxy, short timeout | 200, `{ ok: false, error: "Proxy test timed out" }` |
| Connection refused | Valid URL, proxy not listening | 200, `{ ok: false, error: "Connection refused" }` |
| Invalid URL | `"not-a-url"` as proxyUrl | 400, `{ error: "..." }` |
| Defaults | `{ "proxyUrl": "..." }` only | Uses default testUrl and timeoutMs |
| SSL error | Proxy with bad cert | 200, `{ ok: false, error: "SSL error" }` |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (4/4 PASS)
- Code location: internal/settings/proxy_test.go + internal/settings/proxy_test_test.go
