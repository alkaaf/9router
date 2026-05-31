# Atomic Task: api-3 — Test `GET /api/usage/per-key/[keyId]/history`

**Domain**: API Testing
**Priority**: Medium
**Estimated effort**: 20 min

---

## Input

- `src/app/api/usage/per-key/[keyId]/history/route.js`
- Test database with 20+ usage history rows for a single key

## Output

- Integration tests for history endpoint with pagination
- All tests passing

## Process

1. Setup: insert 25 usage history rows for one key, 10 rows for another key
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Default pagination | `GET /api/usage/per-key/{uuid}/history` → 200. Returns 50 rows max (default limit). |
| 2 | Custom limit | `GET /.../history?limit=5` → 200. Returns exactly 5 rows. |
| 3 | Max limit cap | `GET /.../history?limit=500` → 200. Returns 200 rows (capped). |
| 4 | Offset pagination | `GET /.../history?limit=5&offset=5` → Returns rows 6-10. |
| 5 | Offset beyond data | `GET /.../history?limit=5&offset=100` → Returns empty `history` array. |
| 6 | Non-existent key | `GET /.../history` (bad uuid) → 404 |
| 7 | Response schema | Response has `keyId`, `history` (array), `limit`, `offset` |
| 8 | History item fields | Each item has: `id`, `timestamp`, `model`, `provider`, `promptTokens`, `completionTokens`, `cost`, `connectionId` |
| 9 | Only filtered key data | With 25 rows for key A and 10 for key B, requesting key A returns exactly 25 rows (not key B's data) |
| 10 | Order | History returned in chronological order (oldest first via `ORDER BY id ASC`) |

## Dependencies

- be-7: History API route (DONE)
- api-1: API testing setup (DONE)

## Success Criteria

- All 10 test cases pass
- Pagination works correctly with apiKey filter
- Max limit enforcement at 200
