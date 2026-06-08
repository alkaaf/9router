---
id: OAUTH-001
domain: oauth
status: DONE
estimate: 1h
title: Generic OAuth Provider/Action Router
---

## Description

Implement the generic OAuth framework router for `/api/oauth/[provider]/[action]`. This is the base dispatcher that routes all OAuth endpoint requests to provider-specific handler logic based on URL parameters. Returns 404 for unknown provider/action combinations. All OAuth endpoints return JSON responses and use CLI token authentication.

## Input

- HTTP method: GET or POST
- Path: `/api/oauth/:provider/:action` (Fiber params)
- Auth header: `x-9r-cli-token` (CLI token)
- Request body: Provider-specific (varies by action)

## Output

- JSON response in provider-specific format on success
- Error JSON on failure
- HTTP 404 for unknown provider/action combos
- HTTP 401 for missing/invalid CLI token

## Logic

1. Extract `provider` and `action` from Fiber URL params
2. Validate CLI token via auth middleware
3. Look up registered provider handler map (provider -> action -> handler function)
4. Dispatch to the matched handler function
5. Return handler's JSON response or 404 if no match found
6. Ensure all responses follow the Node.js endpoint format for frontend compatibility

## Acceptance Criteria
- [x] Router registers all OAuth provider endpoints under `/api/oauth/`
- [x] Known provider/action combos route to correct handler
- [x] Unknown provider returns 404
- [x] Unknown action for known provider returns 404
- [x] CLI token auth enforced on all OAuth routes
- [x] Response format matches Node.js endpoint behavior
- [x] Error responses are consistent JSON structure

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid provider/action | POST /api/oauth/gitlab/pat with valid token | JSON response from gitlab/pat handler |
| Unknown provider | POST /api/oauth/unknown/pat | HTTP 404 |
| Unknown action | POST /api/oauth/gitlab/unknown | HTTP 404 |
| Missing auth header | POST /api/oauth/gitlab/pat (no token) | HTTP 401 |
| Invalid auth token | POST /api/oauth/gitlab/pat (bad token) | HTTP 401 |

## Agent Log
- Started: 2026-06-04 16:20
- Completed: 2026-06-04 16:40
- Agent: agent-oauth
- AC-001 verified: DefaultRouter registers gitlab/pat, cursor/auto-import, kiro/social-authorize, kiro/social-exchange, codex/import-token, iflow/cookie
- AC-002 verified: TestRouter_Dispatch_KnownPair exercises known (provider,action) pair through Router → returns handler result
- AC-003 verified: TestRouter_Dispatch_UnknownProvider returns ErrNotFound → HTTP 404 via OAuthHandler
- AC-004 verified: TestRouter_Dispatch_UnknownAction returns ErrNotFound → HTTP 404
- AC-005 verified: OAuthHandler calls validator.Validate before Dispatch; 401 tested for missing and invalid token
- AC-006 verified: writeOAuthOK writes {result} JSON; matches Node.js NextResponse.json shape
- AC-007 verified: writeOAuthError writes {"error":"...","code":"..."} — matches Node.js dashboardGuard.js error envelope

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (15 test cases in internal/oauth/router_test.go + 8 in internal/handler/api/oauth_test.go)
- Code location: internal/oauth/router.go, internal/oauth/auth.go, internal/oauth/registry.go, internal/oauth/handlers.go, internal/handler/api/oauth.go
