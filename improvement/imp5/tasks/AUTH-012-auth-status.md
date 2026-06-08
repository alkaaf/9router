---
id: AUTH-012
domain: auth
status: DONE
estimate: 1h
title: Auth Status Handler (GET /api/auth/status)
---

## Description
GET `/api/auth/status` handler. Returns `{"authenticated": bool, "authMode": "password" | "oidc"}`. A user is considered authenticated if they have a valid JWT cookie or if `requireLogin=false` in settings (open dashboard).

## Input
- `auth_token` cookie from request
- Settings from DB: `RequireLogin bool`, `AuthMode string`

## Output
- `200 {"authenticated": true, "authMode": "password"}` — valid session
- `200 {"authenticated": true, "authMode": "oidc"}` — OIDC configured
- `200 {"authenticated": false, "authMode": "password"}` — no session, login required

## Logic
1. Read settings `RequireLogin` and `AuthMode`.
2. If `RequireLogin` is `false`, return `{"authenticated": true, "authMode": authMode}` immediately.
3. Extract `auth_token` cookie.
4. If cookie is empty, return `{"authenticated": false, "authMode": authMode}`.
5. Verify JWT via `VerifyJWT(cookieValue)`.
6. If valid, return `{"authenticated": true, "authMode": authMode}`.
7. If invalid/expired, return `{"authenticated": false, "authMode": authMode}`.

## Acceptance Criteria
- [ ] Returns `authenticated: true` with valid JWT cookie
- [ ] Returns `authenticated: false` with no cookie when `RequireLogin=true`
- [ ] Returns `authenticated: true` with no cookie when `RequireLogin=false`
- [ ] Returns `authenticated: false` with expired JWT cookie
- [ ] `authMode` field always reflects current settings value

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid JWT cookie | auth_token=valid JWT | `{"authenticated":true,"authMode":"password"}` |
| No cookie, login required | RequireLogin=true | `{"authenticated":false,"authMode":"password"}` |
| No cookie, login not required | RequireLogin=false | `{"authenticated":true,"authMode":"password"}` |
| Expired cookie | auth_token=expired JWT | `{"authenticated":false,"authMode":"password"}` |
| OIDC mode | AuthMode="oidc" | `{"authenticated":false,"authMode":"oidc"}` |
