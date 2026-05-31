# Atomic Task: fet-2 — Test Period Selector Behavior

**Domain**: Frontend Testing
**Priority**: High
**Estimated effort**: 20 min

---

## Input

- `PerKeyUsagePage` component with `SegmentedControl`
- Mock fetch that captures request URLs

## Output

- Test cases for period selector interaction
- All tests passing

## Process

1. Setup: mock fetch, render page with initial data loaded
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Change period to "24h" | Click "24h" → `setPeriod("24h")` → fetch called with `?period=24h` |
| 2 | Change period to "today" | Click "Today" → fetch called with `?period=today` |
| 3 | Change period to "60d" | Click "60D" → fetch called with `?period=60d` |
| 4 | URL sync | When URL `?period=30d`, initial period = "30d" |
| 5 | URL change triggers update | Simulating URL change from `?period=7d` to `?period=24h` → period state updates |
| 6 | All 5 periods render | SegmentedControl shows: Today, 24h, 7D, 30D, 60D |

## Dependencies

- fe-2: PerKeyUsagePage (DONE)
- fet-1: Basic render tests (DONE)

## Success Criteria

- All 6 test cases pass
- Period change triggers correct API call with new period parameter
