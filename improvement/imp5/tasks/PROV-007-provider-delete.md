---
id: PROV-007
domain: providers
status: DONE
evidence: |
  Implementation:
    - repository.ProviderRepository.Delete (returns ErrProviderNotFound on no row)
    - handler/api/providers.go (handleDeleteProvider) maps ErrProviderNotFound → 404
  Tests: handler/api/providers_test.go
    - TestProvidersHandler_Delete         — PASS (existing ID → 200 + success message)
    - TestProvidersHandler_DeleteNotFound — PASS (unknown ID → 404)
  Verification (2026-06-04):
    go test -run TestProvidersHandler_Delete -v ./internal/handler/api/ → PASS
  AC coverage:
    - [x] Existing ID deleted and returns 200 (TestProvidersHandler_Delete)
    - [x] Non-existent ID returns 404 (TestProvidersHandler_DeleteNotFound)
estimate: 1h
title: Provider Delete
---

## Description

Delete a provider connection by ID via `DELETE /api/providers/[id]`.

## Input

- URL param: `id` (connection UUID)

## Output

- `{ message: "Connection deleted successfully" }` with status 200
- 404 if not found

## Logic

1. Query connection by `id` from DB.
2. If not found, return 404.
3. Delete connection from DB.
4. Return `{ message: "Connection deleted successfully" }` with status 200.

## Acceptance Criteria
- [x] Existing ID deleted and returns 200
- [x] Non-existent ID returns 404

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Existing ID | `DELETE /api/providers/<uuid>` | 200 with success message |
| Non-existent ID | `DELETE /api/providers/<unknown-uuid>` | 404 |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- All AC verified: ✓
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: handleDeleteProvider
- Tests: TestProvidersHandler_Delete, TestProvidersHandler_DeleteNotFound
