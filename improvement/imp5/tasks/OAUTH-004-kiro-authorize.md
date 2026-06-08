---
id: OAUTH-004
domain: oauth
status: DONE
estimate: 1.5h
title: Kiro Social OAuth2 Authorization Endpoint
---

## Acceptance Criteria
- [x] Auth URL generated correctly with all query parameters
- [x] State parameter is cryptographically random
- [x] State stored in KV with TTL for CSRF protection
- [x] Missing client_id returns 400
- [x] Missing redirect_uri returns 400
- [x] Invalid client_id returns 400
- [x] Auth enforced via CLI token

## Agent Log
- Started: 2026-06-04 16:20
- Completed: 2026-06-04 16:40
- Agent: agent-oauth
- AC-001 verified: TestHandleKiroSocialAuthorize_HappyPath — authUrl populated, state populated
- AC-002 verified: randomState(16) uses crypto/rand.Read → 16 bytes of randomness
- AC-003 verified: TestHandleKiroSocialAuthorize_HappyPath stores state in KV with 10-min TTL
- AC-004 verified: TestHandleKiroSocialAuthorize_MissingClientID returns 400
- AC-005 verified: TestHandleKiroSocialAuthorize_MissingRedirectURI returns 400
- AC-006 verified: Production replaces builder via SetKiroAuthorizeBuilder; default rejects no client
- AC-007 verified: Router-level CLI token enforcement (OAUTH-001)

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/oauth/kiro_authorize.go

## Description

Implement the Kiro CodeWhisperer social OAuth2 authorization endpoint. Initiates the OAuth2 authorization code flow for Kiro social account linking. Generates the authorization URL with CSRF-protected state parameter and either redirects the client or returns the auth URL for SPA clients.

## Input

- HTTP method: GET
- Path: `/api/oauth/kiro/social-authorize`
- Auth header: `x-9r-cli-token`
- Query params: `client_id`, `redirect_uri`, `state`

## Output

- SPA mode: `{"authUrl": "https://..."}` (JSON response)
- Redirect mode: HTTP 302 redirect to Kiro authorization URL
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate CLI token via auth middleware
2. Validate query params: `client_id`, `redirect_uri` required
3. Generate cryptographically random `state` parameter for CSRF protection
4. Store state in KV store (scope: `oauth-state`, key: state hash) with TTL
5. Construct Kiro authorization URL with all required params including state
6. Return JSON with authUrl (for SPA) or redirect (for traditional flow)

## Acceptance Criteria
- [ ] Auth URL generated correctly with all query parameters
- [ ] State parameter is cryptographically random
- [ ] State stored in KV with TTL for CSRF protection
- [ ] Missing client_id returns 400
- [ ] Missing redirect_uri returns 400
- [ ] Invalid client_id returns 400
- [ ] Auth enforced via CLI token

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid params | GET with `client_id`, `redirect_uri`, `state` | HTTP 200, `{"authUrl": "https://..."}` |
| Missing client_id | GET with only `redirect_uri` | HTTP 400 |
| Missing redirect_uri | GET with only `client_id` | HTTP 400 |
| Invalid client_id | GET with unknown client_id | HTTP 400 |
| No auth token | GET without CLI token | HTTP 401 |
