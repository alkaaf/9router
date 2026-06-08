---
id: AUTH-014
domain: auth
status: DONE
estimate: 2h
title: JWT Middleware Tier 2 (/api/*)
---

## Description
Fiber middleware for `/api/*` routes (non-v1). Enforces JWT authentication extracted from `Authorization: Bearer <token>` or `x-api-token` header. CLI token in `x-9r-cli-token` header also grants access. Public paths and always-protected paths are exempt.

## Input
- Request headers: `Authorization: Bearer <jwt>`, `x-api-token`, `x-9r-cli-token`
- `c.IP()` — for local-only path enforcement
- `VerifyJWT(token)` from AUTH-003
- `ValidateCLIToken(headerValue)` from AUTH-008

## Output
- `c.Next()` with `c.Locals("claims", *jwt.RegisteredClaims)` — if authorized
- `401 {"error":"invalid token"}` — if JWT invalid/expired
- `401 {"error":"CLI token required for local-only endpoints"}` — if local-only without CLI token

## Logic
1. Define `publicPaths`: `/api/health`, `/api/init`, `/api/locale`, `/api/auth/*`, `/api/version`, `/api/settings/require-login`.
2. Define `alwaysProtected`: `/api/shutdown`, `/api/settings/database`, `/api/settings/oauth`.
3. Define `localOnly`: `/api/cli-tools/*`, `/api/tunnel/*`.
4. If request path matches `publicPaths`, call `c.Next()`.
5. If path matches `localOnly`:
   - Check `x-9r-cli-token`; if valid, `c.Next()`.
   - Check if `c.IP()` is loopback; if loopback, `c.Next()`.
   - Otherwise return `401 {"error":"CLI token required for local-only endpoints"}`.
6. Extract JWT: check `Authorization` header for `Bearer <token>`; fall back to `x-api-token` header.
7. If no token found, return `401 {"error":"missing token"}`.
8. Call `VerifyJWT(token)`; if not ok, return `401 {"error":"invalid token"}`.
9. On success, set `c.Locals("claims", claims)` and call `c.Next()`.

## Acceptance Criteria
- [ ] Valid JWT in `Authorization: Bearer` passes
- [ ] Valid JWT in `x-api-token` passes
- [ ] CLI token passes without JWT
- [ ] Public paths (`/api/health`, `/api/auth/*`) skip auth
- [ ] Local-only path (`/api/cli-tools/*`) returns 401 from remote without CLI token
- [ ] Local-only path allows loopback without CLI token
- [ ] Expired JWT returns 401

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid Bearer JWT | Authorization: Bearer validJWT | 200 |
| Valid x-api-token | x-api-token: validJWT | 200 |
| CLI token | x-9r-cli-token: validMachineToken | 200 |
| Public path | GET /api/health | 200 |
| No token | No headers on /api/chat | 401 |
| Expired JWT | Authorization: Bearer expiredJWT | 401 |
| Local-only remote | GET /api/cli-tools/exec from remote | 401 |
| Local-only loopback | GET /api/cli-tools/exec from 127.0.0.1 | 200 |
