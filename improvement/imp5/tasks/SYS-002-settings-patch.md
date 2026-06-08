---
id: SYS-002
domain: settings
status: DONE
estimate: 3h
title: PATCH /api/settings — Update settings with side effects
---

## Description

Merge-patch the settings singleton row. Handles password changes with bcrypt hashing and current-password verification. Detects OIDC secret clear (empty string deletes the field). Triggers runtime side effects on specific field changes: outbound proxy changes call `applyOutboundProxyEnv()`, combo strategy changes call `resetComboRotation()`. Returns same redacted shape as GET.

## Input

```json
{
  "requireLogin": true,
  "outboundProxyEnabled": true,
  "outboundProxyUrl": "http://proxy:8080",
  "comboStrategy": "sticky",
  "comboStickyRoundRobinLimit": 10,
  "comboStrategies": [],
  "newPassword": "s3cr3t",
  "currentPassword": "old-password",
  "oidcClientSecret": ""
}
```

## Output

Same redacted shape as SYS-001 GET (200).

## Logic

1. Parse the merge-patch body (partial update — only present fields are updated).
2. **Password handling:**
   - If `newPassword` is present:
     - If a password already exists in DB, `currentPassword` must be provided and match the stored bcrypt hash.
     - If no password exists yet, `currentPassword` is optional (default "123456" accepted for first-time setup).
     - Hash `newPassword` with bcrypt before storing.
   - If `newPassword` is absent, password is not modified.
3. **OIDC secret clearing:** If `oidcClientSecret` is present and is empty string or whitespace-only, delete the field from the stored JSON.
4. **Outbound proxy side effect:** If any of `outboundProxyEnabled`, `outboundProxyUrl`, or `outboundNoProxy` changed, call `applyOutboundProxyEnv()` to update process-level env vars.
5. **Combo strategy side effect:** If any of `comboStrategy`, `comboStickyRoundRobinLimit`, or `comboStrategies` changed, call `resetComboRotation()` to reset in-memory rotation state.
6. Merge updated fields into the singleton row and persist.
7. Return the redacted response (same shape as SYS-001).

## Acceptance Criteria

- [x] Endpoint is registered at `PATCH /api/settings`
- [x] Returns 200 with redacted settings shape on success
- [x] Password change requires `newPassword` in body
- [x] When password exists, missing `currentPassword` returns 400
- [x] Incorrect `currentPassword` returns 401
- [x] First-time password allows default "123456" as `currentPassword`
- [x] `oidcClientSecret` set to empty/whitespace deletes the field from DB
- [x] Outbound proxy field changes trigger `applyOutboundProxyEnv()` side effect
- [x] Combo strategy field changes trigger `resetComboRotation()` side effect
- [x] Only present fields in the patch body are updated (merge, not replace)
- [x] Response never contains `password` or `oidcClientSecret`

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-sys
- AC-001 verified: SettingsPatchHandler exported from internal/handler/api + internal/settings
- AC-002 verified: TestSettingsPatch_HappyPath returns 200
- AC-003 verified: TestSettingsPatch_PasswordChangeFirstTime + TestSettingsPatch_PasswordChangeExisting
- AC-004 verified: TestSettingsPatch_MissingCurrentPassword returns 400
- AC-005 verified: TestSettingsPatch_WrongCurrentPassword returns 401
- AC-006 verified: TestSettingsPatch_FirstTimeDefaultPassword accepts "123456"
- AC-007 verified: TestSettingsPatch_OIDCSecretClear + TestSettingsPatch_OIDCSecretWhitespace
- AC-008 verified: TestSettingsPatch_ProxySideEffect calls ApplyOutboundProxyEnv
- AC-009 verified: TestSettingsPatch_ComboSideEffect calls ResetComboRotation
- AC-010 verified: TestSettingsPatch_PartialUpdate preserves untouched fields
- AC-011 verified: TestSettingsPatch_ResponseRedaction strips secrets

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Happy path | Valid partial patch | 200 with updated redacted settings |
| Password change (first time) | `newPassword` only, no existing password | 200, password hashed and stored |
| Password change (existing) | `newPassword` + correct `currentPassword` | 200, password updated |
| Missing currentPassword | `newPassword` only, password already exists | 400 |
| Wrong currentPassword | `newPassword` + wrong `currentPassword` | 401 |
| OIDC secret clear | `oidcClientSecret: ""` | Field deleted from DB |
| OIDC secret whitespace | `oidcClientSecret: "   "` | Field deleted from DB |
| Proxy side effect | Change `outboundProxyUrl` | `applyOutboundProxyEnv()` called |
| Combo side effect | Change `comboStrategy` | `resetComboRotation()` called |
| Partial update | Only `requireLogin` in body | Only `requireLogin` changed, others untouched |
| No password fields | Body without `newPassword` | Password unchanged |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (18/18 PASS — 9 GET + 9 PATCH)
- Code location: internal/settings/handler.go + internal/settings/patch.go + internal/handler/api/settings.go + internal/settings/handler_test.go + internal/settings/patch_test.go
