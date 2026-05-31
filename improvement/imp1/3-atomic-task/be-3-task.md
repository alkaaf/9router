# Atomic Task: be-3 — Extend `getChartData()` with `filter.apiKey`

**Domain**: Backend
**Priority**: High
**Estimated effort**: 25 min

---

## Input

- `src/lib/db/repos/usageRepo.js` — `getChartData(period, filter = {})` function (line ~620)
- Filtering strategy: today/24h uses SQL `WHERE apiKey = ?`, 7d+ parses `usageDaily.byApiKey`

## Output

- `getChartData()` accepts optional `filter.apiKey` parameter
- Today/24h: SQL query includes `AND apiKey = ?`
- 7d/30d/60d: Parses `dayData.byApiKey` JSON, sums only matching apiKey entries
- Backward compatible

## Process

### Step 1: Add filter parameter (line ~631)

**Before**:
```js
export async function getChartData(period = "7d", filter = {}) {
```

**After**:
```js
export async function getChartData(period = "7d", filter = {}) {
  const filterApiKey = filter.apiKey || null;
```

### Step 2: Add filter to today/24h SQL query (line ~671)

**Before**:
```js
`SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ?`,
filterApiKey ? [new Date(startTime).toISOString(), filterApiKey] : [new Date(startTime).toISOString()]
```

**After**:
```js
`SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ?${filterApiKey ? " AND apiKey = ?" : ""}`,
filterApiKey
  ? [new Date(startTime).toISOString(), filterApiKey]
  : [new Date(startTime).toISOString()]
```

### Step 3: Add filter to 7d/30d/60d JSON parsing (line ~700)

**Before**:
```js
const tokens = dayData ? (dayData.promptTokens || 0) + (dayData.completionTokens || 0) : 0;
const cost = dayData ? (dayData.cost || 0) : 0;
```

**After**:
```js
let tokens = 0;
let cost = 0;
if (dayData) {
  if (filterApiKey) {
    for (const ak of Object.values(dayData.byApiKey || {})) {
      if (ak.apiKey === filterApiKey) {
        tokens += (ak.promptTokens || 0) + (ak.completionTokens || 0);
        cost += ak.cost || 0;
      }
    }
  } else {
    tokens = (dayData.promptTokens || 0) + (dayData.completionTokens || 0);
    cost = dayData.cost || 0;
  }
}
```

## Dependencies

- be-1: `idx_uh_apiKey` index added (DONE)

## Success Criteria

- `getChartData("7d")` returns combined data for all keys (unchanged)
- `getChartData("7d", { apiKey: "sk-..." })` returns only matching key's data per day
- `getChartData("today", { apiKey: "sk-..." })` filters via SQL
- `tokens = promptTokens + completionTokens` per bucket
- `cost` matches sum of matching `byApiKey` entries
