---
id: PROV-003
domain: providers
status: DONE
estimate: 2h
title: Provider List
---

## Description

Return all provider connections as a JSON array via `GET /api/providers`. Hide sensitive fields (`apiKey`, `accessToken`, `refreshToken`, `idToken`). For compatible providers (`openai-compatible-*`, `anthropic-compatible-*`), enrich `name` from the node registry or `providerSpecificData.nodeName`. Supports optional `isActive` filter.

## Input

- Query params: none (list all) or filter `{ isActive: bool }`

## Output

- `{ connections: [{ id, provider, authType, name, priority, globalPriority, defaultModel, testStatus, lastError, ... }] }`
- Sensitive fields (`apiKey`, `accessToken`, `refreshToken`, `idToken`) omitted from all connection objects

## Logic

1. Query connections from DB (GORM model, connections table).
2. If `isActive` query param is `true`, filter to `isActive = true`.
3. For each connection, omit sensitive fields before returning.
4. For `openai-compatible-*` and `anthropic-compatible-*` providers, enrich `name` from node registry (`providerSpecificData.nodeName`) if available.
5. Return array wrapped in `{ connections: [...] }`.

## Acceptance Criteria
- [x] Returns all connections when no filter
- [x] Returns only active connections when `isActive=true`
- [x] Sensitive fields (`apiKey`, `accessToken`, `refreshToken`, `idToken`) absent in response
- [x] Compatible provider name enriched from node registry or providerSpecificData

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| List all | `GET /api/providers` | All connections with sensitive fields stripped |
| Filter active | `GET /api/providers?isActive=true` | Only connections where `isActive=true` |
| Sensitive fields | Any request | `apiKey`, `accessToken`, `refreshToken`, `idToken` not present |
| Compatible enrich | openai-compatible-* connection | `name` enriched from node registry |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- AC-1 verified: TestProvidersHandler_ListAll passes
- AC-2 verified: TestProvidersHandler_ListFilterActive passes
- AC-3 verified: TestProvidersHandler_SensitiveFieldsStripped passes
- AC-4 verified: TestProvidersHandler_CompatibleNameEnriched passes
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/handler/api/providers.go (handleListProviders) + internal/providers/service.go (ToView, stripSensitive)
