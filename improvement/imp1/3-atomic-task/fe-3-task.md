# Atomic Task: fe-3 — Integrate OverviewCards for KPI Display

**Domain**: Frontend
**Priority**: High
**Estimated effort**: 10 min

---

## Input

- `src/app/(dashboard)/dashboard/usage/components/OverviewCards` — existing component
- `PerKeyUsagePage` from fe-2
- API response `data.stats` from `/api/usage/per-key/[keyId]`

## Output

- OverviewCards rendered in PerKeyUsagePage with stats from API
- 4 KPI cards: Total Requests, Input Tokens, Output Tokens, Cost

## Process

### Step 1: Import OverviewCards

Add to imports in `page.js`:
```js
import OverviewCards from "@/app/(dashboard)/dashboard/usage/components/OverviewCards";
```

### Step 2: Add OverviewCards to JSX

Insert after the period selector in the page's return:
```jsx
{/* KPI Cards */}
<OverviewCards stats={data.stats} />
```

### Step 3: Verify props match

The API response provides `stats` with:
- `totalRequests`
- `totalPromptTokens`
- `totalCompletionTokens`
- `totalCost`

`OverviewCards` expects a `stats` prop with the same shape — no transformation needed.

## Dependencies

- fe-2: PerKeyUsagePage shell (DONE)
- `OverviewCards` component (existing, no changes)

## Success Criteria

- 4 KPI cards render below period selector
- Cards show correct values from API response
- Cards match styling of existing Usage page
- No prop transformation needed (API response matches component expectations)
