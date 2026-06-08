---
id: AUTH-015
domain: auth
status: DONE
estimate: 1h
title: Cookie Manager
---

## Description
Cookie helper functions for dashboard JWT session management. `SetAuthCookie(token, request)` sets an `auth_token` cookie with httpOnly, secure, sameSite=lax, path=/. `ClearAuthCookie()` removes the cookie by setting `MaxAge: -1`. The `secure` flag is conditional: true when the request arrived via HTTPS (via `x-forwarded-proto` header or `AUTH_COOKIE_SECURE=true` env), false otherwise.

## Input
- `SetAuthCookie(token string, c *fiber.Ctx)`:
  - `token` — signed JWT string
  - `c` — Fiber context (for inspecting `x-forwarded-proto`)
- `ClearAuthCookie(c *fiber.Ctx)`:
  - `c` — Fiber context

## Output
- `c.Cookie()` call sets or clears the `auth_token` cookie in the HTTP response

## Logic
1. Cookie name: `"auth_token"`.
2. Cookie path: `"/"`.
3. Cookie `httpOnly: true` — prevents JavaScript access.
4. Cookie `sameSite: "lax"` — allows top-level navigation GET to carry cookie.
5. Cookie `secure: bool`:
   - Check `c.Get("x-forwarded-proto")` — if `"https"`, secure=true.
   - Fall back to `os.Getenv("AUTH_COOKIE_SECURE") == "true"`.
   - Default to `false` in development.
6. `SetAuthCookie`:
   - `c.Cookie(&fiber.Cookie{Name: "auth_token", Value: token, Path: "/", HTTPOnly: true, Secure: secure, SameSite: "lax"})`.
   - No `MaxAge` or `Expires` set (session cookie, cleared on browser close).
7. `ClearAuthCookie`:
   - `c.Cookie(&fiber.Cookie{Name: "auth_token", Value: "", Path: "/", MaxAge: -1, HTTPOnly: true, Secure: secure, SameSite: "lax"})`.

## Acceptance Criteria
- [ ] `SetAuthCookie` sets cookie with correct name, path, httpOnly
- [ ] `secure=true` when `x-forwarded-proto: https`
- [ ] `secure=true` when `AUTH_COOKIE_SECURE=true`
- [ ] `secure=false` by default (no proto header, no env)
- [ ] `ClearAuthCookie` sets `MaxAge=-1`
- [ ] `sameSite` is `"lax"` on both set and clear
- [ ] Cookie path is `/` on both set and clear

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Set cookie (HTTPS) | x-forwarded-proto=https | Cookie with `Secure`, `HttpOnly`, `Path=/` |
| Set cookie (HTTP) | no proto header | Cookie without `Secure` flag |
| Set cookie (env override) | AUTH_COOKIE_SECURE=true | Cookie with `Secure` |
| Clear cookie | any context | Cookie `MaxAge=-1`, `Value=""` |
| Cookie name | any set call | Set-Cookie header name is `auth_token` |
