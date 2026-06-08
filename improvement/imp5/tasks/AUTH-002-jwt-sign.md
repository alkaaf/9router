---
id: AUTH-002
domain: auth
status: DONE
estimate: 1h
title: JWT Sign Function
---

## Agent Log
- Started: 2026-06-04 09:30
- Completed: 2026-06-04 09:35
- Agent: agent-auth
- AC-001 verified: token round-trips through ParseWithClaims with same secret
- AC-002 verified: sub=admin parsed correctly
- AC-003 verified: exp delta ~24h within 1-minute tolerance
- AC-004 verified: authenticated claim is true
- AC-005 verified: role=superadmin extras found in raw claims
- AC-006 verified: wrong secret causes parse failure

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/auth/sign.go


## Description
Implement `SignJWT(username string, extraClaims map[string]interface{}) (string, error)` which creates and signs a dashboard JWT using HS256. Sets `authenticated: true`, `sub` to the username, and `exp` to 24 hours from now. Optionally merges extra claims into the token payload.

## Input
- `username string` — the subject of the token (from login form)
- `extraClaims map[string]interface{}` — optional additional claims (may be nil)
- Module-level `SECRET []byte` loaded by AUTH-001

## Output
- `(string, error)` — signed JWT token string or error from `jwt.Sign`

## Logic
1. Create `DashboardClaims` with `Subject: username`, `Authenticated: true`.
2. Merge any `extraClaims` into the `RegisteredClaims` map.
3. Set `exp` to `time.Now().Add(24 * time.Hour)`.
4. Call `jwt.SignWithClaims(context.Background(), hs256, claims)` using the cached `SECRET`.
5. Return the compact token string.

## Acceptance Criteria
- [ ] Token passes `jwt.ParseWithClaims` using same secret
- [ ] `sub` claim equals input username
- [ ] `exp` is ~24h in the future (within 1 minute tolerance)
- [ ] `authenticated` claim is `true`
- [ ] Extra claims are present when provided
- [ ] HS256 algorithm enforced (rejects if secret mismatches)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Basic sign | user="admin" | Valid HS256 token with sub="admin" |
| Sign with extras | extraClaims={"role":"superadmin"} | Token contains role claim on parse |
| Wrong secret verify | token signed with secret A, verify with secret B | Verification fails |
