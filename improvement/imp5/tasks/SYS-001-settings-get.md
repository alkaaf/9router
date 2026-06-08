---
id: SYS-001
domain: settings
status: DONE
estimate: 2h
title: GET /api/settings — Read settings with secret redaction
---

## Description

Return the current settings singleton row with sensitive fields stripped. Reads from the `settings` table. Adds computed flags `oidcConfigured`, `enableRequestLogs`, `enableTranslator`, and `hasPassword`. Response must include `Cache-Control: no-store`.

## Input

- No path parameters or query params
- Auth: optional (dashboard auth or API key)

## Output

```json
{
  "requireLogin": true,
  "outboundProxyEnabled": false,
  "outboundProxyUrl": "",
  "outboundNoProxy": "",
  "comboStrategy": "round-robin",
  "comboStickyRoundRobinLimit": 10,
  "comboStrategies": [],
  "oidcIssuerUrl": "",
  "oidcClientId": "",
  "oidcConfigured": false,
  "tunnelDashboardAccess": true,
  "tunnelUrl": "",
  "tailscaleUrl": "",
  "enableRequestLogs": false,
  "enableTranslator": false,
  "hasPassword": false
}
```

**Absent (redacted):** `password`, `oidcClientSecret`

## Logic

1. Read the singleton settings row from the `settings` table (JSON column or TEXT with JSON parse).
2. Strip sensitive fields: `password`, `oidcClientSecret` must never appear in the response.
3. Compute `oidcConfigured`: `true` only when `oidcIssuerUrl`, `oidcClientId`, and `oidcClientSecret` are all non-empty.
4. Compute `hasPassword`: `true` when the `password` field is non-empty.
5. Read env vars `ENABLE_REQUEST_LOGS` and `ENABLE_TRANSLATOR` (or equivalent) and surface them as `enableRequestLogs` and `enableTranslator`.
6. Set `Cache-Control: no-store` header on the response.
7. Return the redacted, enriched settings object.

## Acceptance Criteria

- [x] Endpoint is registered at `GET /api/settings`
- [x] Returns 200 with the redacted settings shape
- [x] `password` and `oidcClientSecret` are never present in response
- [x] `oidcConfigured` is `true` only when all three OIDC fields are non-empty
- [x] `hasPassword` reflects whether password is set in DB
- [x] `enableRequestLogs` reflects the `ENABLE_REQUEST_LOGS` env var
- [x] `enableTranslator` reflects the `ENABLE_TRANSLATOR` env var
- [x] `Cache-Control: no-store` header is set
- [x] Missing settings row returns sensible defaults

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-sys
- AC-001 verified: SettingsGetHandler exported from internal/handler/api and internal/settings
- AC-002 verified: TestSettingsGet_HappyPath returns 200
- AC-003 verified: TestSettingsGet_Redaction confirms redacted values never leak
- AC-004 verified: TestSettingsGet_OIDCConfigured + TestSettingsGet_OIDCMissingField cover all branches
- AC-005 verified: TestSettingsGet_HasPassword + TestSettingsGet_NoPassword
- AC-006 verified: TestSettingsGet_EnvFlagsOn with ENABLE_REQUEST_LOGS=1
- AC-007 verified: TestSettingsGet_EnvFlagsOn with ENABLE_TRANSLATOR=true
- AC-008 verified: TestSettingsGet_HappyPath checks Cache-Control header
- AC-009 verified: TestSettingsGet_MissingRow with empty store returns sensible defaults

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Happy path | GET /api/settings (authenticated) | 200 with redacted settings, `Cache-Control: no-store` |
| OIDC configured | All OIDC fields set | `oidcConfigured: true` |
| OIDC missing field | Any OIDC field empty | `oidcConfigured: false` |
| Password set | `password` field non-empty in DB | `hasPassword: true` |
| No password | `password` field empty/missing | `hasPassword: false` |
| Env flags on | `ENABLE_REQUEST_LOGS=1` | `enableRequestLogs: true` |
| Missing row | No settings row in DB | Default values, `hasPassword: false` |
| Unauthenticated | No auth header | 401 (or passes if auth is optional) |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (9/9 PASS)
- Code location: internal/settings/handler.go + internal/handler/api/settings.go + internal/settings/handler_test.go
