# Per-Key "Usage per User" Feature — Test Results

**Date:** 2026-05-30
**Feature:** Usage per User (per-key usage dashboard)
**Test Framework:** Vitest 4.1.7 with jsdom environment
**Config:** `tests/vitest.config.js`

## Summary

| Suite | File | Tests | Result |
|-------|------|-------|--------|
| Backend | `usageRepo.perKey.test.js` | 10 | ✅ PASS |
| API | `api.perKey.test.js` | 16 | ✅ PASS |
| Frontend | `perKey.frontend.test.js` | 24 | ✅ PASS |
| **Total** | | **50** | **✅ 50/50 PASS** |

## Command to Run

```bash
npx vitest run tests/unit/usageRepo.perKey.test.js tests/unit/api.perKey.test.js tests/unit/perKey.frontend.test.js --config tests/vitest.config.js
```

## Test Coverage

### Backend (usageRepo.perKey.test.js) — 10 tests
- `getUsageStats` with `filter.apiKey` — aggregation, totals, byProvider breakdown
- `getUsageHistory` with `filter.apiKey` — daily breakdown
- `getChartData` with `filter.apiKey` — chart data aggregation
- Error handling for empty/missing data

### API (api.perKey.test.js) — 16 tests
- `GET /api/usage/per-key/[keyId]` — full response, period filtering
- `GET /api/usage/per-key/[keyId]/history` — history endpoint
- `GET /api/usage/per-key/[keyId]/chart` — chart endpoint
- Error handling (invalid key, missing params, server errors)

### Frontend (perKey.frontend.test.js) — 24 tests
- `fet-1`: Page render states (loading, error, loaded)
- `fet-2`: Period selector behavior (options, switching, default)
- `fet-3`: Sort functionality (table headers, toggle direction)
- `fet-4`: Sidebar accordion navigation (toggle, key links, active state)
- `fet-5`: PerKeyChart (toggle Tokens/Cost, no-data state)
- `fet-6`: Layout and styling (flex container, headings, KPI labels)

## Bugs Found & Fixed During Testing

1. **totalRequests overwritten by byProvider sum** — `usageRepo.js` line 630 was overwriting filtered `totalRequests` with the full aggregate. Fixed to preserve filtered totals.
2. **React.act polyfill** — React 19's `react-dom-test-utils` delegates to `React.act` causing infinite recursion. Fixed with custom polyfill in `tests/setup.js`.
3. **Mock response queuing** — `vi.clearAllMocks()` doesn't clear queued `mockResolvedValueOnce` responses. Added `mockFetch.mockReset()` to beforeEach.
4. **Path alias resolution** — Backend tests need `--config tests/vitest.config.js` for `@/` alias to resolve.

## Notes

- Tests use `React.createElement()` instead of JSX to avoid Vite import analysis parsing issues in `.js` files.
- Frontend tests run in `jsdom` environment; backend/API tests run in `node` environment.
- All per-key feature tests pass with 0 failures.
