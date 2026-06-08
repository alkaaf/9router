# AUTH Domain — Atomic Task Breakdown

> 9Router backend rewrite. Target: Go + Fiber. Reference: Node.js `src/lib/auth/` and `src/dashboardGuard.js`.

---

## AUTH-001 — JWT Secret Management

**Description**
Load/store JWT signing secret. On first boot generate a 32-byte random hex secret and write to `DATA_DIR/jwt-secret` with `0o600`. For tests and dev, read from `JWT_SECRET` env var. Cache in a module-level variable after first load.

**Input**
- `JWT_SECRET` env var (optional)
- `DATA_DIR` constant

**Output**
- `SECRET` — `[]byte` ready for `hmac.New`
- No error on missing file (generates and persists)

**Test Strategy**
- First load generates file; second load reads existing file and returns same value
- Env var takes precedence over file
- File permissions correct after generation

---

## AUTH-002 — JWT Token Creation (SignJWT)

**Description**
Issue a dashboard JWT with claims: `{"authenticated": true, "sub": "<username>", "exp": <24h>}`. Algorithm `HS256`. Use `golang-jwt/jwt/v5`.

**Input**
- Username string (from login)
- Optional extra claims map

**Output**
- Signed JWT string

**Test Strategy**
- Token is valid according to `jwtVerify`
- `sub` matches input username
- Expiry is ~24h from now
- Optional extra claims present

---

## AUTH-003 — JWT Token Verification (jwtVerify)

**Description**
Verify a dashboard JWT. Return `nil` if token is missing or invalid. Return `*jwt.RegisteredClaims` on success. Log verification errors at debug level (do not expose to caller).

**Input**
- JWT string

**Output**
- `(claims *jwt.RegisteredClaims, ok bool)`

**Test Strategy**
- Valid token returns claims + true
- Expired token returns false
- Malformed token returns false
- Missing token returns false

---

## AUTH-004 — CLI Token (Machine-ID-Based)

**Description**
Generate and validate the CLI token. Token = consistent machine ID salted with `"9r-cli-auth"`. Generation uses `MACHINE_ID` env or falls back to hostname + platform uuid. Validation is string equality.

**Input**
- Request headers (`x-9r-cli-token`)

**Output**
- `(valid bool)` — true if header value matches computed machine token

**Test Strategy**
- Token generated for same machine is valid
- Random token is invalid
- Empty/missing token is invalid

---

## AUTH-005 — CLI Token Validation Middleware

**Description**
Fiber middleware that extracts `x-9r-cli-token` header and validates it. Returns `401` if token is present but invalid. Passes through if token is absent (other auth methods on the route handle it).

**Input**
- Fiber `c.IP()` for loopback detection (optional extra check)
- Header value

**Output**
- `c.Next()` or `401` response

**Test Strategy**
- Valid CLI token passes (no extra checks)
- Invalid CLI token returns `401`
- Missing CLI token continues to next handler

---

## AUTH-006 — API Key Storage & Hashing (bcrypt)

**Description**
Store API keys as bcrypt hashes. On first creation generate a random key, hash with bcrypt cost 12, store both key (raw) and hash. Provide `ValidateAPIKey(rawKey string) bool` that runs bcrypt compare.

**Input**
- Raw API key string (on creation)

**Output**
- `generateApiKey() (raw, hashed string)`
- `validateApiKey(raw, hashed) bool`

**Test Strategy**
- Generated raw key validates against its own hash
- Wrong key does not validate
- Hash cost is 12

---

## AUTH-007 — API Key Validation Middleware (v1 tier)

**Description**
Fiber middleware for `/v1/*` and `/v1beta/*` routes. Check `Authorization: Bearer <key>` then `x-api-key` header. Accept API key (local requests bypass via loopback check). Return `401 {"error": "API key required for remote API access"}`.

**Input**
- Request headers
- `c.IP()` for loopback check

**Output**
- `c.Next()` or `401`

**Test Strategy**
- Valid API key passes
- Localhost request passes without key
- Remote request without key returns `401`
- CLI token also grants access to v1 tier

---

## AUTH-008 — Password Hashing (bcrypt)

**Description**
Wrap bcrypt password hashing. Hash cost 12. Compare function returns bool. Handle nil/empty stored hash gracefully (for uninitialized state).

**Input**
- Plaintext password

**Output**
- `hashPassword(password) (hash string, err error)`
- `verifyPassword(password, hash) bool`

**Test Strategy**
- Hash of "123456" matches bcrypt-compare result
- Wrong password returns false
- Empty hash returns false on compare

---

## AUTH-009 — Login Handler (Password + JWT Issue)

**Description**
POST `/api/auth/login` handler. Accept `{password: string}`. Compare bcrypt hash (or env fallback `"123456"`). On success: record success, issue dashboard JWT cookie, return `{success: true}`. On failure: record fail, return `{error, remainingBeforeLock}` or `429` if locked.

**Input**
- Request body: `{"password": "..."}`
- Settings from DB (`password`, `authMode`)

**Output**
- `{"success": true}` (200) or `{"error": "...", "remainingBeforeLock": N}` (401) or `{"retryAfter": N}` (429)

**Test Strategy**
- Correct password returns 200 + sets cookie
- Wrong password returns 401 + decrements remaining attempts
- Locked after 5 fails returns 429

---

## AUTH-010 — Login Rate Limiter (In-Memory)

**Description**
Progressive lockout. In-memory map keyed by IP. Per-IP state: `fails`, `lockUntil`, `lockLevel`, `lastFailAt`. Lock steps: 30s, 2m, 10m, 30m (level 0-3). Auto-reset if 1h since last fail and not locked. Max 5 fails before first lock.

**Input**
- Client IP (from `x-forwarded-for` first comma-sep value or `x-real-ip`)

**Output**
- `isLocked(ip) (locked bool, retryAfter int)`
- `recordFail(ip) remainingBeforeLock`
- `recordSuccess(ip)` (clears state)

**Test Strategy**
- 5 fails → locked for 30s
- Success clears state
- No fail in 1h resets counters
- Lockout escalates to 2m, 10m, 30m on repeated lock

---

## AUTH-011 — Cookie Management (Dashboard)

**Description**
`SetAuthCookie(token, request)` and `ClearAuthCookie()`. Cookie name `auth_token`. Properties: `httpOnly: true`, `secure: true` (if `x-forwarded-proto: https` OR `AUTH_COOKIE_SECURE=true`), `sameSite: lax`, `path: /`.

**Input**
- JWT token string
- Request (for protocol check)

**Output**
- Fiber cookie set via `c.Cookie()`
- `c.Cookie()` with `MaxAge: -1` for clear

**Test Strategy**
- Cookie set with correct attributes on HTTPS
- Cookie cleared properly
- Secure flag only set when appropriate

---

## AUTH-012 — Dashboard Auth Middleware

**Description**
Fiber middleware for `/dashboard/*`. Always require JWT from `auth_token` cookie. Redirect to `/login` on failure via 302. If `requireLogin=false` skip JWT check.

**Input**
- Cookie value for `auth_token`
- Settings from DB (`requireLogin`)

**Output**
- `c.Next()` or redirect to `/login`

**Test Strategy**
- Valid JWT continues
- Invalid/missing JWT redirects to `/login`
- `requireLogin=false` skips check

---

## AUTH-013 — Auth Middleware Orchestration (3-Tier Gate)

**Description**
Main Fiber app middleware setup in correct order:
1. `/v1/*, /v1beta/*` — API key auth (with CLI token and loopback bypass)
2. `/api/*` — JWT auth (public allow-list excluded) with CLI token bypass
3. `/dashboard/*` — JWT auth (always)
Public paths exempt: `/api/health`, `/api/init`, `/api/locale`, `/api/auth/*`, `/api/version`, `/api/settings/require-login`

**Input**
- `ALWAYS_PROTECTED` paths (`/api/shutdown`, `/api/settings/database` etc.)
- `LOCAL_ONLY` paths (`/api/cli-tools/*`, `/api/tunnel/*`)

**Output**
- Three configured middleware groups on Fiber app
- Local-only paths additionally check loopback + CLI token

**Test Strategy**
- Combined test per tier: correct auth method passes, wrong method returns 401
- Public paths always pass
- Always-protected paths require JWT regardless
- Local-only blocks remote without CLI token

---

## AUTH-014 — Logout Handler

**Description**
POST `/api/auth/logout`. Clear `auth_token` cookie. Return `{success: true}`.

**Input**
- Request

**Output**
- `{"success": true}` + cleared cookie

**Test Strategy**
- Cookie is cleared
- Returns 200 regardless of prior session state

---

## AUTH-015 — Auth Status Handler

**Description**
GET `/api/auth/status`. Return `{authenticated: bool, authMode: "password"|"oidc"}`. Authenticated if valid JWT or `requireLogin=false` in settings.

**Input**
- `auth_token` cookie

**Output**
- `{"authenticated": true/false, "authMode": "password"|"oidc"}`

**Test Strategy**
- Authenticated user returns `authenticated: true`
- No cookie + `requireLogin=true` returns `authenticated: false`
- `requireLogin=false` returns `authenticated: true`

---

## AUTH-016 — Auth Mode Check (Password vs OIDC)

**Description**
Detect `authMode` from settings. If `authMode == "oidc"` and OIDC is configured, password login returns `403`. Expose `IsOidcConfigured()` check.

**Input**
- Settings from DB (authMode, oidc config fields)

**Output**
- `IsOidcConfigured() bool`
- Password login returns `403` when OIDC is active

**Test Strategy**
- Password login blocked when OIDC is configured
- Auth status returns correct mode

---

## AUTH-017 — Tunnel/Tailscale Dashboard Access Guard

**Description**
Block login and dashboard access via tunnel/tailscale hostname if `tunnelDashboardAccess=false`. Extract host from request, resolve to tunnel and tailscale URLs from settings, compare.

**Input**
- Request host header, settings

**Output**
- Login: `403` if tunnel access disabled
- Dashboard middleware: redirect to `/login` if tunnel access disabled

**Test Strategy**
- Login via tunnel URL blocked when `tunnelDashboardAccess=false`
- Dashboard via tunnel URL redirects when disabled
- Same access allowed when enabled

---

## AUTH-018 — Settings Seeded Password (Default)

**Description**
When `settings.password` is `nil`/`empty`, treat login as `password=="123456"` OR env `INITIAL_PASSWORD`. After first successful login, transition to bcrypt-hashed password in settings.

**Input**
- `settings.password` (may be nil)
- `INITIAL_PASSWORD` env var
- Successful login event

**Output**
- `isValidPassword(password, settings) bool`
- On first login: store bcrypt hash in settings

**Test Strategy**
- Empty settings uses default "123456" password
- Env var takes precedence over default
- After first login, stored hash used

---

## AUTH-019 — Integration: Full Login Flow

**Description**
Full login flow: rate limit check → tunnel guard → OIDC check → password verify → JWT issue → cookie set. All pieces wired together.

**Input**
- Full HTTP POST to `/api/auth/login`

**Output**
- Valid JWT cookie set on success
- Appropriate error codes at each failure point

**Test Strategy**
- Happy path: 200 + valid cookie
- Locked: 429
- Wrong password: 401
- No attempts consumed on OIDC mode: 403

---

## AUTH-020 — Integration: API Tier Full Flow

**Description**
Full API tier flow: check CLI token → check loopback → check bearer API key → check x-api-key header. Block remote without API key. CLI token auto-authorizes. Loopback auto-authorizes.

**Input**
- Requests to `/v1/chat/completions` with various auth methods

**Output**
- 200 with valid auth, 401 without

**Test Strategy**
- Bearer token with valid API key: 200
- `x-api-key` with valid API key: 200
- CLI token: 200
- Localhost without auth: 200
- Remote without auth: 401
- Invalid key: 401

---

## Cross-Cutting Notes

- Uses `golang-jwt/jwt/v5` for all JWT operations
- Uses `golang.org/x/crypto/bcrypt` for password and API key hashing
- No external Redis/map store — in-memory rate limiting resets on process restart (consistent with Node.js behavior)
- All cookie operations use Fiber's `c.Cookie()` and `c.SetCookie()`
- Settings read from the DB domain (see `database.md`) — no auth-domain self-dependencies
- OIDC integration deferred to its own domain (`oauth-integration.md`)

## Domain Structure

```
domain/auth/
  jwt.go           # AUTH-001, AUTH-002, AUTH-003
  cli_token.go     # AUTH-004, AUTH-005
  api_key.go       # AUTH-006, AUTH-007
  password.go      # AUTH-008, AUTH-009
  login_limiter.go # AUTH-010
  cookie.go        # AUTH-011
  middlewares.go   # AUTH-012, AUTH-013
  handlers.go      # AUTH-009, AUTH-014, AUTH-015, AUTH-016, AUTH-017
  integration_test.go # AUTH-019, AUTH-020
```
