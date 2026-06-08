---
id: AUTH-010
domain: auth
status: DONE
estimate: 2h
title: Login Handler (POST /api/auth/login)
---

## Description
POST `/api/auth/login` handler. Accepts `{"password": "..."}` in the request body. Checks password against bcrypt hash (or default fallback). On success: issue dashboard JWT, set auth cookie, return `{"success": true}`. On failure: record fail in rate limiter, return `{"error": "...", "remainingBeforeLock": N}` or `429` if locked.

## Input
- Request body: `{"password": "..."}` (string, required)
- Settings from DB: `PasswordHash string` (nullable), `AuthMode string`
- Rate limiter: `RecordFail(ip) (remaining int)`, `RecordSuccess(ip)`, `IsLocked(ip) (locked bool, retryAfter int)`
- `SignJWT(username)` from AUTH-002
- `SetAuthCookie(token, c)` from AUTH-015

## Output
- `200 {"success": true}` — on correct password, JWT cookie set
- `401 {"error": "invalid password", "remainingBeforeLock": N}` — on wrong password, attempts remaining
- `429 {"error": "locked", "retryAfter": N}` — on lockout
- `400 {"error": "missing password"}` — no password in body

## Logic
1. Parse JSON body; return `400` if password field missing.
2. Extract client IP from `x-forwarded-for` (first value) or `x-real-ip` or `c.IP()`.
3. Check `IsLocked(ip)`; if locked return `429` with `retryAfter` seconds.
4. Check settings `authMode`; if OIDC, return `403 {"error":"use OIDC login"}`.
5. Determine expected password: if `settings.PasswordHash` is non-empty, compare via `bcrypt.CompareHashAndPassword`; if empty, compare against `"123456"` or `INITIAL_PASSWORD` env.
6. If password matches:
   - `RecordSuccess(ip)` — clears rate limiter state.
   - If `settings.PasswordHash` is empty (first login): bcrypt-hash the password and persist to settings via DB domain.
   - `token := SignJWT(username_from_settings)`.
   - `SetAuthCookie(token, c)`.
   - Return `200 {"success": true}`.
7. If password does not match:
   - `remaining := RecordFail(ip)`.
   - If `remaining <= 0`, return `429` with `retryAfter` from lock step.
   - Return `401 {"error": "invalid password", "remainingBeforeLock": remaining}`.

## Acceptance Criteria
- [ ] Correct password returns 200 + sets `auth_token` cookie
- [ ] Wrong password returns 401 + decrements remaining counter
- [ ] 5 wrong attempts triggers 429 lockout
- [ ] Successful login clears rate limiter state
- [ ] First login (no hash in settings) persists bcrypt hash
- [ ] OIDC mode returns 403 before password check
- [ ] `INITIAL_PASSWORD` env var overrides default "123456"

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Correct password | body={"password":"123456"} | 200 + cookie set |
| Wrong password | body={"password":"wrong"} | 401 + remainingBeforeLock > 0 |
| Lockout after 5 fails | 5th wrong attempt | 429 + retryAfter > 0 |
| Successful after fail | correct password after 2 fails | 200 + limiter reset |
| Missing password | body={} | 400 |
| OIDC mode | authMode="oidc" | 403 |
