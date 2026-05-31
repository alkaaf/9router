# Atomic Task: api-5 — Test Edge Cases

**Domain**: API Testing
**Priority**: Medium
**Estimated effort**: 25 min

---

## Input

- All 3 per-key API endpoints
- Test database in various states (empty, partial, deleted keys)

## Output

- Edge case test suite
- All tests passing

## Process

1. Setup: prepare edge case scenarios:
   - Key with zero usage (exists in `apiKeys` but no entries in `usageHistory`)
   - Very old key (60+ days of data)
   - Key with only `usageHistory` entries (no `usageDaily` aggregation yet)
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Key with zero usage | All endpoints return valid structure with zero values (not null, not error) |
| 2 | Very large period | `GET /per-key/{uuid}?period=60d` returns within 5 seconds |
| 3 | Deleted key | After deleting key from DB, `GET /per-key/{uuid}` → 404 |
| 4 | Concurrent period params | `?period=7d&period=30d` → uses first or last value consistently |
| 5 | Missing period param | No `?period=` → defaults to "7d" |
| 6 | History with no entries | `history` array is `[]`, not `null` or `undefined` |
| 7 | Chart with partial data | Key with usage on only 2 of 7 days → remaining days show `tokens: 0, cost: 0` |
| 8 | SQL injection attempt | `keyId` with SQL chars → treated as UUID lookup, returns 404 (no injection) |
| 9 | Special characters in key name | Key with unicode/emoji name → renders correctly in JSON |
| 10 | Large response | Key with 10,000+ history entries → `history` endpoint returns paginated (limit enforced) |

## Dependencies

- api-4: Data accuracy tests (DONE)

## Success Criteria

- All 10 test cases pass
- No crashes or unhandled exceptions in edge cases
- Graceful degradation for missing/empty data
