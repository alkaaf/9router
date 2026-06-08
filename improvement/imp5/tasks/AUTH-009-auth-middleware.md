---
id: AUTH-009
domain: auth
status: DONE
estimate: 2h
title: Auth Middleware (3-Tier)
---

## Description
Register three tiers of Fiber middleware in the correct order on the main app: (1) API key auth for `/v1/*` and `/v1beta/*` (with CLI token and loopback bypass), (2) JWT auth for `/api/*` (with CLI token bypass), (3) Dashboard JWT auth for `/dashboard/*`. Public paths (`/api/health`, `/api/init`, `/api/auth/*`, etc.) are exempt from all tiers.

## Input
- Fiber `*app.Group` routes already registered
- `ApiKeyRepo` for API key validation
- `ALWAYS_PROTECTED` paths set (e.g. `/api/shutdown`, `/api/settings/database`)
- `LOCAL_ONLY` paths set (e.g. `/api/cli-tools/*`, `/api/tunnel/*`)

## Output
- `SetupAuth(app *fiber.App, ...)` function that attaches three middleware groups to the Fiber app

## Logic
1. Define `publicPaths` set: `/api/health`, `/api/init`, `/api/locale`, `/api/auth/*`, `/api/version`, `/api/settings/require-login`.
2. Define `alwaysProtected` set: `/api/shutdown`, `/api/settings/database`, `/api/settings/oauth`.
3. Define `localOnly` set: `/api/cli-tools/*`, `/api/tunnel/*`.
4. Tier 1 — API key middleware (`ApiKeyAuth`):
   - Skip if `publicPaths`, `alwaysProtected`, or `localOnly` matched.
   - Check `x-9r-cli-token` header → if valid, `c.Next()`.
   - Check `c.IP()` for loopback (127.0.0.1, ::1, 10.x, 172.16-31.x, 192.168.x) → if loopback, `c.Next()`.
   - Check `Authorization: Bearer <key>` or `x-api-key` header → validate against `ApiKeyRepo`.
   - Otherwise return `401 {"error":"API key required for remote API access"}`.
5. Tier 2 — JWT middleware for `/api/*`:
   - Skip if `publicPaths` or `alwaysProtected` matched.
   - Skip if valid CLI token present.
   - Extract JWT from `Authorization: Bearer <token>` or `x-api-token` header.
   - Verify via `VerifyJWT`; on failure return `401 {"error":"invalid token"}`.
   - On success set `c.Locals("claims", claims)` and `c.Next()`.
6. Tier 3 — Dashboard middleware for `/dashboard/*`:
   - Always require JWT from `auth_token` cookie.
   - Verify via `VerifyJWT`; on failure redirect to `/login` with 302.
   - On success set `c.Locals("claims", claims)` and `c.Next()`.
7. Attach each tier as `app.Use(prefix, middleware)`.

## Acceptance Criteria
- [ ] All three tiers attach to the Fiber app without panic
- [ ] Public paths pass all three tiers without auth
- [ ] `/v1/chat/completions` returns 401 without valid API key from remote IP
- [ ] `/v1/chat/completions` returns 200 with valid API key
- [ ] `/api/chat/completions` returns 401 without valid JWT
- [ ] `/dashboard/` returns 302 redirect without valid JWT cookie
- [ ] Loopback (127.0.0.1) bypasses API key check on `/v1/*`
- [ ] CLI token bypasses both tier 1 and tier 2 checks

## Test Scenarios
| Scenario | Request | Expected |
|----------|---------|----------|
| Public path | GET /api/health | 200, no auth |
| Valid API key | GET /v1/chat/completions + Bearer key | 200 |
| No key (remote) | GET /v1/chat/completions | 401 |
| Loopback | GET /v1/chat/completions from 127.0.0.1 | 200 |
| CLI token | GET /v1/* + x-9r-cli-token header | 200 |
| Dashboard no cookie | GET /dashboard/ | 302 to /login |
| Valid JWT cookie | GET /dashboard/ + auth_token cookie | 200 |
