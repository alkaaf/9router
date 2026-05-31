# Atomic Task: fet-1 — Test PerKeyUsagePage Renders Correctly

**Domain**: Frontend Testing
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` — `PerKeyUsagePage` component
- Mock API responses for `/api/usage/per-key/[keyId]`
- Testing library (React Testing Library or similar)

## Output

- Test cases covering page render states
- All tests passing

## Process

1. Setup: mock `fetch` for API calls, mock `useParams` and `useSearchParams`
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Loading state | Page renders `<CardSkeleton />` while `loading === true` |
| 2 | Data loaded | Page renders header with key name, masked key in `<code>`, `OverviewCards`, `PerKeyChart`, `UsageTable` |
| 3 | Error state (fetch fails) | Page shows "Failed to load usage data." message |
| 4 | Error state (404 key) | API returns 404 → page shows error message |
| 5 | Key name display | Header shows `data.keyName` or "Unnamed Key" fallback |
| 6 | Masked key display | Shows `data.keyMasked` in `<code>` tag with proper truncation format |
| 7 | Period from URL | When `?period=30d`, period state initializes to "30d" |

## Dependencies

- fe-2: PerKeyUsagePage component (DONE)
- Project test framework configured

## Success Criteria

- All 7 test cases pass
- Page renders all expected sections when data is available
