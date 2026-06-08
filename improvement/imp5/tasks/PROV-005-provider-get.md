---
id: PROV-005
domain: providers
status: DONE
evidence: |
  Implementation split:
    - providers.ToView(*model.ProviderConnection, nodeNameLookup) — strips sensitive fields
    - repository.ProviderRepository.GetByID — DB lookup
    - handler/api/providers.go (handleGetProvider) glues them
  Tests: handler/api/providers_test.go
    - TestProvidersHandler_GetByID — PASS (existing ID → 200)
    - TestProvidersHandler_GetByIDNotFound — PASS (unknown ID → 404)
  Sensitive-strip coverage: TestProvidersHandler_SensitiveFieldsStripped, TestToView_StripsSensitive — PASS
  Verification (2026-06-04):
    go test -run TestProvidersHandler_GetByID -v ./internal/handler/api/ → PASS
    go test -run TestToView_Strips -v ./internal/providers/         → PASS
  AC coverage:
    - [x] Existing ID returns 200 (TestProvidersHandler_GetByID)
    - [x] Non-existent ID returns 404 (TestProvidersHandler_GetByIDNotFound)
    - [x] apiKey/accessToken/refreshToken/idToken absent in response (TestProvidersHandler_SensitiveFieldsStripped)
estimate: 1h
title: Provider Get
---

## Description

Return a single provider connection by ID via `GET /api/providers/[id]`. Hide sensitive fields same as list endpoint (`apiKey`, `accessToken`, `refreshToken`, `idToken`).

## Input

- URL param: `id` (connection UUID)

## Output

- `{ connection: { ... } }` with status 200
- 404 if not found

## Logic

1. Query connection by `id` from DB.
2. If not found, return 404.
3. Omit sensitive fields (`apiKey`, `accessToken`, `refreshToken`, `idToken`).
4. Return `{ connection: { ... } }`.

## Acceptance Criteria
- [x] Existing ID returns connection with status 200
- [x] Non-existent ID returns 404
- [x] Sensitive fields absent in response

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Existing ID | `GET /api/providers/<uuid>` | 200 with connection object |
| Non-existent ID | `GET /api/providers/<unknown-uuid>` | 404 |
| Sensitive fields | Any existing connection | `apiKey`, `accessToken`, `refreshToken`, `idToken` not present |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- All AC verified: ✓
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: handleGetProvider in providers.go
- Tests: TestProvidersHandler_GetByID, TestProvidersHandler_GetByIDNotFound
