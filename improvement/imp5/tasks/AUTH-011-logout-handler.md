---
id: AUTH-011
domain: auth
status: DONE
estimate: 1h
title: Logout Handler (POST /api/auth/logout)
---

## Description
POST `/api/auth/logout` handler. Clears the `auth_token` cookie by setting `MaxAge: -1` and returns `{"success": true}`. Does not invalidate the JWT server-side (JWT remains valid until expiry — consistent with stateless design).

## Input
- Request `*fiber.Ctx`

## Output
- `200 {"success": true}` with cleared `auth_token` cookie

## Logic
1. Call `ClearAuthCookie(c)` (sets `auth_token` cookie with `MaxAge: -1`).
2. Return `200` with JSON body `{"success": true}`.
3. Do not check auth state — logout is idempotent; return 200 even without an active session.

## Acceptance Criteria
- [ ] Returns 200 with `{"success": true}` regardless of session state
- [ ] Clears `auth_token` cookie on response
- [ ] Does not return 401 for unauthenticated users (idempotent)
- [ ] Cookie `MaxAge` is -1 (browser deletes it)
- [ ] Cookie path is `/` to cover all routes

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Authenticated logout | Valid session cookie present | 200 + cookie cleared |
| Unauthenticated logout | No session cookie | 200 + cookie cleared (idempotent) |
| Cookie cleared | After logout response | Cookie has `Max-Age=-1`, `Path=/` |
