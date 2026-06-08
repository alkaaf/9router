---
id: AUTH-001
domain: auth
status: DONE
estimate: 1h
title: JWT Struct Definition
---

## Description
Define Go structs for JWT claims used across the auth domain. Provides typed containers for dashboard JWT payloads and error responses, eliminating stringly-typed JSON handling in handlers and middleware.

## Input
No direct input; these are struct definitions consumed by all other AUTH tasks.

## Output
- `DashboardClaims` struct: `Authenticated bool`, `Subject string`, `ExpiresAt int64`
- `LoginResponse` struct: `Success bool`, `Error string`, `RemainingBeforeLock int`
- `AuthStatusResponse` struct: `Authenticated bool`, `AuthMode string`

## Logic
1. Define `DashboardClaims` embedding `jwt.RegisteredClaims` for standard JWT fields.
2. Add custom fields: `Authenticated bool` and `Subject string` mapped to JWT `sub` claim.
3. Define response structs with JSON tags matching the existing Node.js API contract.
4. Export all types from `pkg/auth/` (or equivalent) so handlers and middleware can import them.

## Acceptance Criteria
- [x] `DashboardClaims` compiles with `golang-jwt/jwt/v5` `Claims` interface
- [x] JSON tags on response structs match `{"success":true}`, `{"error":"..."}`, `{"authenticated":true,"authMode":"password"}`
- [x] All structs in a single file `jwt.go` under `domain/auth/`
- [x] `go vet` passes on the file

## Agent Log
- Started: 2026-06-04 09:20
- Completed: 2026-06-04 09:25
- Agent: agent-auth
- AC-001 verified: go build ./internal/auth/ succeeds, DashboardClaims embeds jwt.RegisteredClaims
- AC-002 verified: TestLoginResponse_Marshal_Success + TestAuthStatusResponse_Marshal pass
- AC-003 verified: Single file internal/auth/jwt.go with all structs
- AC-004 verified: go vet ./internal/auth/ passes

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/auth/jwt.go

