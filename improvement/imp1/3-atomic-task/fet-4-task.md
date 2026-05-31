# Atomic Task: fet-4 — Test Sidebar Accordion Navigation

**Domain**: Frontend Testing
**Priority**: Medium
**Estimated effort**: 20 min

---

## Input

- `src/shared/components/Sidebar.js` — extended with "Usage per User" accordion
- Mock API response for `/api/keys`
- Router mock for navigation

## Output

- Test cases for sidebar accordion behavior
- All tests passing

## Process

1. Setup: render Sidebar component, mock `/api/keys` response with 3 API keys
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | Initial render | "Usage per User" section visible but collapsed (accordion closed) |
| 2 | Click to expand | Accordion opens, API keys list appears |
| 3 | API fetch on expand | `fetch('/api/keys')` called when accordion first opens |
| 4 | API keys render | Each key shows name and masked key, linked to `/dashboard/usage-per-key/[keyId]` |
| 5 | Click API key link | Navigates to `/dashboard/usage-per-key/{keyId}` |
| 6 | Click to collapse | Accordion closes, list hides |
| 7 | Re-expand doesn't re-fetch | Second expand doesn't call `/api/keys` again (cached) |
| 8 | Empty keys list | If API returns empty array, accordion shows "No API keys" or empty state |
| 9 | API error | If `/api/keys` fails, accordion still opens but shows empty/error state |

## Dependencies

- fe-1: Sidebar accordion (DONE)

## Success Criteria

- All 9 test cases pass
- Accordion follows same pattern as existing accordions in sidebar
