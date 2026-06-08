---
id: AUTH-013
domain: auth
status: DONE
estimate: 2h
title: JWT Middleware Tier 1 (/v1/*)
---

## Description
Fiber middleware for `/v1/*` and `/v1beta/*` API tier. Enforces API key authentication with three bypass paths: valid CLI token header, loopback source IP, and CLI token in the `x-9r-cli-token` header. Returns `401` with a structured error for unauthorized remote requests.

## Input
- Request headers: `Authorization: Bearer <key>`, `x-api-key`, `x-9r-cli-token`
- `c.IP()` — client IP for loopback detection
- `ApiKeyRepo` — for bcrypt comparison against stored key hashes
- `ValidateCLIToken(headerValue)` — from AUTH-008

## Output
- `c.Next()` — if authorized (valid key, CLI token, or loopback)
- `401 {"error":"API key required for remote API access"}` — if not authorized

## Logic
1. Check if request path matches `publicPaths`, `alwaysProtected`, or `localOnly` — skip this middleware if matched (handled by other tiers).
2. Extract `x-9r-cli-token` header; if present and `ValidateCLIToken` returns true, call `c.Next()`.
3. Check if `c.IP()` is loopback (127.0.0.1, ::1, 10.x, 172.16-31.x, 192.168.x, fc00::/7) — if loopback, call `c.Next()`.
4. Extract API key: check `Authorization` header for `Bearer <key>` format; fall back to `x-api-key` header.
5. If no API key found, return `401 {"error":"API key required for remote API access"}`.
6. Call `ApiKeyRepo.FindValidApiKey(ctx, rawKey)` — bcrypt-compares the raw key against all stored hashes.
7. If found, call `ApiKeyRepo.UpdateLastUsed(ctx, key.ID)` in a goroutine (non-blocking).
8. Call `c.Next()` with key info in `c.Locals("apiKey", key.ID)`.
9. If not found, return `401 {"error":"invalid API key"}`.

## Acceptance Criteria
- [ ] Valid API key in `Authorization: Bearer` header passes
- [ ] Valid API key in `x-api-key` header passes
- [ ] CLI token bypasses check entirely
- [ ] Loopback IP (127.0.0.1) bypasses check entirely
- [ ] Remote request without key returns 401 with error message
- [ ] Invalid API key returns 401
- [ ] `ApiKey.ID` stored in `c.Locals("apiKey")` on success

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid Bearer key | Authorization: Bearer validKey | 200 |
| Valid x-api-key | x-api-key: validKey | 200 |
| CLI token | x-9r-cli-token: validMachineToken | 200 |
| Loopback | c.IP() = 127.0.0.1, no key | 200 |
| No key (remote) | No headers, remote IP | 401 |
| Invalid key | Bearer wrong-key | 401 |
