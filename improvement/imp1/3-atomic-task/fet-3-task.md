# Atomic Task: fet-3 — Test Sort Functionality in UsageTable

**Domain**: Frontend Testing
**Priority**: Medium
**Estimated effort**: 25 min

---

## Input

- `PerKeyUsagePage` component with sortable `UsageTable`
- Mock data with multiple model entries (different request counts, costs)

## Output

- Test cases for sort interactions
- All tests passing

## Process

1. Setup: render page with mock data containing 3+ model entries with varying `requests`, `cost`, `promptTokens`
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Initial sort | Table renders sorted by `requests` descending (default `sortBy="requests"`, `sortOrder="desc"`) |
| 2 | Click "Model" header | Sorts by `name` (field "name"), desc order. Rows reorder alphabetically. |
| 3 | Click same header again | Toggles to asc order. Rows reorder reverse alphabetically. |
| 4 | Click different header | Switches to new field, desc order. Previous sort order reset. |
| 5 | Sort by cost | Click cost column → entries sorted by `cost` descending |
| 6 | Sort by tokens | Click tokens column → entries sorted by `promptTokens` descending |
| 7 | Grouped data sort | `groupedByModel` memo re-computes when `sortBy` or `sortOrder` changes |

## Dependencies

- fe-5: UsageTable integration with sorting (DONE)
- fet-1: Basic render tests (DONE)

## Success Criteria

- All 7 test cases pass
- Sort state persists during re-renders
- Grouped data updates correctly on sort change
