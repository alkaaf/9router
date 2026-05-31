# Atomic Task: fet-6 — Test Responsive Layout & Styling

**Domain**: Frontend Testing
**Priority**: Low
**Estimated effort**: 20 min

---

## Input

- `PerKeyUsagePage` component
- `Sidebar.js` accordion
- Tailwind CSS classes in use

## Output

- Visual/structural tests for responsive behavior
- All tests passing

## Process

1. Setup: render page at different viewport widths (or verify CSS classes)
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Period selector width | `SegmentedControl` has `className="w-full sm:w-auto"` — full width on mobile, auto on desktop |
| 2 | Page container | Root div has `className="flex min-w-0 flex-col gap-6 px-1 sm:px-0"` — responsive padding |
| 3 | Card responsive padding | `PerKeyChart` Card has `className="... p-3 sm:p-4"` — smaller padding on mobile |
| 4 | Chart responsive | `ResponsiveContainer` with `width="100%"` — adapts to container |
| 5 | Table responsive | `UsageTable` has horizontal scroll on small screens |
| 6 | Sidebar on mobile | Accordion works on narrow viewport |
| 7 | Text truncation | Long key names don't overflow — `min-w-0` on flex containers enables truncation |
| 8 | Code badge styling | Masked key in `<code>` has `className="text-xs bg-surface-2 px-1.5 py-0.5 rounded"` |

## Dependencies

- fe-6: Styling applied (DONE)
- fet-1 through fet-5 (render tests for context)

## Success Criteria

- All 8 structural tests pass
- CSS classes match design system patterns
- No layout breakage at common viewport sizes
