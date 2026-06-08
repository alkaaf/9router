---
id: AUTH-008
domain: auth
status: DONE
estimate: 1h
title: CLI Token Validation
---

## Description
Implement CLI token generation and validation for machine-identity-based CLI authentication. Token is a consistent machine ID salted with `"9r-cli-auth"`. Validation is string equality against the computed token. Used by the CLI token middleware to allow CLI traffic without API keys or JWTs.

## Input
- `MACHINE_ID` env var (optional)
- Hostname (fallback) via `os.Hostname()`
- Platform-specific UUID (macOS: `ioreg -rd1 -c IOPlatformExpertDevice`; Linux: `/etc/machine-id`)
- Header: `x-9r-cli-token` from request

## Output
- `(valid bool)` — true if header value matches computed machine token
- `ComputeCLIToken() string` — deterministic token for the current machine

## Logic
1. `ComputeCLIToken()`:
   1. Read `MACHINE_ID` env var; if empty, try `os.Hostname()`.
   2. On macOS, attempt to read platform UUID via `ioreg`.
   3. Concatenate machine ID + `"9r-cli-auth"` and SHA-256 hash.
   4. Return hex-encoded hash as the CLI token.
2. `ValidateCLIToken(headerToken string) bool`:
   1. Return `false` for empty/missing header.
   2. Compute current machine token via `ComputeCLIToken()`.
   3. Return `headerToken == computedToken` (constant-time comparison via `subtle.ConstantTimeCompare`).

## Acceptance Criteria
- [ ] `ComputeCLIToken` returns same value across multiple calls on same machine
- [ ] `ValidateCLIToken` returns `true` for matching token, `false` for any mismatch
- [ ] Uses `crypto/subtle.ConstantTimeCompare` to prevent timing attacks
- [ ] macOS UUID path works without panic if `ioreg` is unavailable
- [ ] Empty/absent `MACHINE_ID` falls back to hostname without error

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Same machine | `ComputeCLIToken()` called twice | Returns identical string |
| Valid token | headerToken=computedToken | `true` |
| Invalid token | headerToken="random-string" | `false` |
| Empty token | headerToken="" | `false` |
| Missing env | MACHINE_ID="" | Falls back to hostname, non-empty result |
