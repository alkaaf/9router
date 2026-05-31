# Atomic Task: fet-5 — Test PerKeyChart Component

**Domain**: Frontend Testing
**Priority**: Medium
**Estimated effort**: 25 min

---

## Input

- `PerKeyChart` component in `page.js`
- Mock chart data (tokens + cost arrays)
- Recharts testing utilities

## Output

- Test cases for chart rendering and interaction
- All tests passing

## Process

1. Setup: render `PerKeyChart` with mock `chartData` and mock fetch for chart API
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Initial render (tokens mode) | Chart renders with "Tokens" button active, Area shows `tokens` dataKey, stroke `#6366f1` |
| 2 | Initial render (cost mode) | Chart renders with "Cost" button active, Area shows `cost` dataKey, stroke `#f59e0b` |
| 3 | Toggle to cost | Click "Cost" → `viewMode` changes to "cost", Area switches to cost data |
| 4 | Toggle back to tokens | Click "Tokens" → `viewMode` changes to "tokens", Area switches back |
| 5 | Y-axis formatter (tokens) | Y-axis ticks formatted as `K`/`M` (e.g., "1.5K", "2.3M") when in tokens mode |
| 6 | Y-axis formatter (cost) | Y-axis ticks formatted as `$X.XX` when in cost mode |
| 7 | Tooltip formatter | Hovering shows formatted value: tokens → "1.5K Tokens", cost → "$2.50 Cost" |
| 8 | No data state | Empty `chartData` → shows "No data for this period" message |
| 9 | Period change triggers re-fetch | When `period` prop changes, fetch called with new period |
| 10 | Gradient fills | Chart has gradient definitions for both tokens (indigo) and cost (amber) |

## Dependencies

- fe-4: PerKeyChart component (DONE)
- fet-1: Basic render tests (DONE)

## Success Criteria

- All 10 test cases pass
- Chart correctly switches between tokens and cost views
- Formatters produce correct output for all value ranges
