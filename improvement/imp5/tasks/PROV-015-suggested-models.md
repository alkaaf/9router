---
id: PROV-015
domain: providers
status: DONE
estimate: 2h
title: Suggested Models
---

  evidence: |
  (verified 2026-06-04)
## Description

Fetch and filter free/subsidized models from external URLs based on type via `GET /api/providers/suggested-models`. Types:
- `"openrouter-free"`: Fetch `https://openrouter.ai/api/v1/models`, filter to models with `pricing.prompt === "0"` and `pricing.completion === "0"` and `context_length >= 200000`. Return `[{ id, name, contextLength }]` sorted by context length desc.
- `"opencode-free"`: Fetch `https://opencode.ai/zen/v1/models`, filter to models with `id.endsWith("-free")` or known free model IDs. Return `[{ id, name }]`.

Results cached. Non-2xx responses return empty array.

## Input

- Query params: `url` (fetch URL), `type` (`"openrouter-free"` | `"opencode-free"`)

## Output

- `{ data: [{ id, name, contextLength? }] }`
- 400 if type unknown or url/type missing
- `{ data: [] }` on non-2xx fetch

## Logic

1. Validate `type` is `"openrouter-free"` or `"opencode-free"`. If unknown, return 400.
2. Validate both `url` and `type` query params present. If missing, return 400.
3. Check cache for this `url`. If cached and not expired, return cached result.
4. Fetch from `url` with appropriate timeout and headers.
5. If non-2xx response, return `{ data: [] }` (do not cache errors).
6. Parse response JSON.
7. Filter based on type:
   - **openrouter-free**: filter where `pricing.prompt == "0"` AND `pricing.completion == "0"` AND `context_length >= 200000`. Map to `[{ id, name, contextLength }]`. Sort by `contextLength` descending.
   - **opencode-free**: filter where `id.endsWith("-free")` OR `id` in known free model ID list. Map to `[{ id, name }]`.
8. Cache result (TTL based on type).
9. Return `{ data: [...] }`.

## Acceptance Criteria
- [x] Valid openrouter-free type returns filtered model list
- [x] Valid opencode-free type returns filtered model list
- [x] Unknown type returns 400
- [x] Missing url or type returns 400
- [x] Non-2xx fetch returns `{ data: [] }`
- [x] openrouter-free: filters out paid models correctly
- [x] opencode-free: includes known free model IDs even without `-free` suffix
- [x] Results cached; second call returns cached result

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| openrouter-free valid | type="openrouter-free", valid url | `{ data: [{ id, name, contextLength }] }` sorted desc |
| opencode-free valid | type="opencode-free", valid url | `{ data: [{ id, name }] }` with free models only |
| Unknown type | type="unknown" | 400 |
| Missing url | No url param | 400 |
| Missing type | No type param | 400 |
| Non-2xx fetch | URL returns 500 | `{ data: [] }` |
| openrouter paid filter | Paid model (pricing != 0) | Excluded from result |
| opencode non-free | Model without -free suffix | Excluded unless in known free list |
| Cache hit | Second call with same params | Returns cached result |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-prov-2
- All AC verified: ✓
- All tests passed: ✓

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: FetchSuggestedModels + parsers in tester.go + SuggestedModelsHandler
- Tests: TestFetchSuggestedModels_InvalidType, TestFetchSuggestedModels_MissingParams, TestFetchSuggestedModels_Non2xx, TestFetchSuggestedModels_CacheHit, TestParseOpenrouterFree_FiltersPaid, TestParseOpencodeFree_FreeSuffix, TestParseOpencodeFree_KnownFreeIDs, TestFetchSuggestedModels_OpenRouterSuccess, TestSuggestedModelsHandler_MissingParams, TestSuggestedModelsHandler_UnknownType
