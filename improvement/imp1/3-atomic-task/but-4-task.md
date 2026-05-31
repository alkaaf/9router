# Atomic Task: but-4 — Test `getApiKeyById()` Key Resolution

**Domain**: Backend Unit Testing
**Priority**: Medium
**Estimated effort**: 10 min

---

## Input

- `src/lib/db/repos/apiKeysRepo.js` — `getApiKeyById(id)` function
- Test database with `apiKeys` table entries

## Output

- Test cases for key resolution
- All tests passing

## Process

1. Setup: insert 2 API key rows with known UUIDs and full key strings
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | `getApiKeyById(validUuid)` | Returns object with `id`, `key` (full string), `name`, `isActive`, `createdAt` |
| 2 | `getApiKeyById("nonexistent-uuid")` | Returns `undefined` or `null` |
| 3 | `getApiKeyById("")` (empty string) | Returns `undefined` or `null` — no crash |
| 4 | Verify `key` field is FULL string | `key` starts with `sk-`, contains machineId + UUID + CRC — not truncated |

## Dependencies

- `apiKeysRepo.js` (existing, no changes needed)

## Success Criteria

- All 4 test cases pass
- Confirms UUID → full key resolution works for API route handlers
