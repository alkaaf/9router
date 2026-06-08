---
id: AUTH-004
domain: auth
status: DONE
estimate: 1h
title: Bcrypt Hash Function
---

## Description
Implement `HashPassword(plaintext string) (string, error)` using bcrypt cost 12. Returns a bcrypt-encoded string suitable for storage in the `settings.password` column. Centralizes cost constant for the auth domain.

## Input
- `plaintext string` — user's plaintext password from the login form

## Output
- `(string, error)` — bcrypt hash in modular crypt format (e.g. `"$2a$12$..."`); error only on internal bcrypt failure

## Logic
1. Use `golang.org/x/crypto/bcrypt`.
2. Define `const bcryptCost = 12` at package level.
3. Call `bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)`.
4. Convert the returned `[]byte` to `string` and return `(hash, nil)`.
5. On error, wrap and return `( "", err )` — do not log PII.

## Acceptance Criteria
- [x] Generated hash verifies against the input plaintext via `bcrypt.CompareHashAndPassword`
- [x] Hash string starts with `"$2a$12$"` prefix (cost 12 marker)
- [x] Empty plaintext still returns a valid hash (callers must validate non-empty upstream)
- [x] Hash function is deterministic only in the sense that same input produces valid match — actual salt makes output unique
- [x] Two calls with same input produce different hashes (random salt)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Hash "123456" | "123456" | `$2a$12$...` hash that compares true |
| Cost 12 verification | any input | Hash prefix is `$2a$12$` |
| Two hashes differ | "123456" twice | Two different hash strings, both compare true |
| Long password | 72+ chars (bcrypt limit) | Either truncate per bcrypt spec or return clear error |

## Agent Log
- Started: 2026-06-04 09:38
- Completed: 2026-06-04 09:40
- Agent: agent-auth
- AC-001 verified: HashPassword("123456") round-trips through VerifyPassword
- AC-002 verified: hash prefix is $2a$12$
- AC-003 verified: empty string returns valid hash
- AC-004 verified: same input produces valid but non-identical hashes
- AC-005 verified: TestHashPassword_TwoCallsDiffer confirms different outputs
- Test scenarios: all pass

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/auth/bcrypt.go
