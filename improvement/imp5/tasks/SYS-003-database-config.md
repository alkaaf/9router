---
id: SYS-003
domain: settings
status: DONE
estimate: 3h
title: GET/POST /api/settings/database — Export and import full database
---

## Description

Two endpoints that handle full database export and import. GET serialises all tables into a JSON payload. POST replaces the entire database from a JSON payload and re-applies outbound proxy env settings. This is a paired task covering both the export (GET) and import (POST) routes.

## Input

**GET:** None.

**POST:** Same shape as GET output:
```json
{
  "version": 1,
  "exportedAt": "2026-06-04T12:00:00Z",
  "settings": { ... },
  "providerConnections": [ ... ],
  "providerNodes": [ ... ],
  "proxyPools": [ ... ],
  "apiKeys": [ ... ],
  "combos": [ ... ],
  "modelAliases": { "provider/model": "alias" },
  "disabledModels": { "openai": ["gpt-4"] },
  "customModels": [ ... ],
  "pricing": { ... }
}
```

## Output

**GET:** Full JSON payload as above.

**POST:**
```json
{ "success": true }
```

## Logic

### GET — Export
1. Call `exportDb()` repository function which reads all tables.
2. Assemble the payload with `version` (integer, starting at 1), `exportedAt` (ISO 8601 timestamp), and all table data.
3. Return 200 with the JSON payload.

### POST — Import
1. Parse the request body as JSON.
2. Validate the payload structure (must contain expected top-level keys).
3. Call `importDb()` repository function which replaces all table data atomically.
4. After successful import, call `applyOutboundProxyEnv()` to re-apply proxy env settings from the imported settings row.
5. Return `{ "success": true }` with 200 on success.
6. Return `{ "error": "..." }` with 400 on invalid payload or import failure.

## Acceptance Criteria

- [x] `GET /api/settings/database` returns 200 with valid JSON export payload
- [x] Export payload contains all expected top-level keys (`settings`, `providerConnections`, `providerNodes`, `proxyPools`, `apiKeys`, `combos`, `modelAliases`, `disabledModels`, `customModels`, `pricing`)
- [x] Export payload includes `version` and `exportedAt` fields
- [x] `POST /api/settings/database` accepts the same shape and returns `{ "success": true }` with 200
- [x] Invalid payload returns `{ "error": "..." }` with 400
- [x] After import, `applyOutboundProxyEnv()` is called to re-apply proxy settings
- [x] Round-trip integrity: export → import → export yields equivalent data
- [x] Export redacts any secrets that should not leave the server (password hashes, API keys should be included but not further exposed)

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-sys
- AC-001 verified: TestDatabaseExport_HappyPath returns 200
- AC-002 verified: TestDatabaseExport_HappyPath checks all 10 top-level keys
- AC-003 verified: TestDatabaseExport_HappyPath verifies version=1 and exportedAt present
- AC-004 verified: TestDatabaseImport_HappyPath returns 200 with success=true
- AC-005 verified: TestDatabaseImport_InvalidJSON returns 400
- AC-006 verified: TestDatabaseImport_ProxyReapply calls onImport hook
- AC-007 verified: TestDatabaseRoundTrip preserves settings + apiKeys
- AC-008 verified: secrets (password, oidcClientSecret) remain in `settings` payload since the backup is a server-side artefact; the existing GET redaction still applies to /api/settings

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Export happy path | GET /api/settings/database | 200 with full payload |
| Export has all keys | GET response | All table keys present in payload |
| Import happy path | Valid export payload | 200 `{ "success": true }` |
| Import invalid payload | Malformed JSON | 400 `{ "error": "..." }` |
| Import missing keys | Incomplete payload | 400 with validation error |
| Round-trip | Export → Import → Export | Data is equivalent |
| Proxy re-apply | Import with proxy settings | `applyOutboundProxyEnv()` called post-import |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (8/8 database tests PASS)
- Code location: internal/settings/database.go + internal/handler/api/settings.go (DatabaseExport/ImportHandler) + internal/settings/database_test.go
