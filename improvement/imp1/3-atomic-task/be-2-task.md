# Atomic Task: be-2 — Extend `getUsageStats()` with `filter.apiKey`

**Domain**: Backend
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- `src/lib/db/repos/usageRepo.js` — `getUsageStats(period = "all")` function (line ~319)
- Filtering strategy from `analysis-9-data-layer-filtering.md`

## Output

- `getUsageStats()` accepts optional `filter` parameter with `apiKey` support
- Backward compatible — calling without filter returns same results as before
- ~15 lines of conditional filter logic added

## Process

### Step 1: Add filter parameter to function signature

**File**: `src/lib/db/repos/usageRepo.js:319`

**Before**:
```js
export async function getUsageStats(period = "all") {
```

**After**:
```js
export async function getUsageStats(period = "all", filter = {}) {
  const filterApiKey = filter.apiKey || null;
```

### Step 2: Add filter to daily summary loop (line ~476)

**Before**:
```js
for (const [akKey, ak] of Object.entries(day.byApiKey || {})) {
```

**After**:
```js
for (const [akKey, ak] of Object.entries(day.byApiKey || {})) {
  if (filterApiKey && ak.apiKey !== filterApiKey) continue;
```

### Step 3: Add filter to overlay query (line ~512)

**Before**:
```js
const overlaySql = `SELECT ...`;
const overlayParams = [new Date(overlayCutoff).toISOString()];
```

**After**:
```js
const overlaySql = filterApiKey
  ? `SELECT ... WHERE apiKey = ?`
  : `SELECT ...`;
const overlayParams = filterApiKey
  ? [new Date(overlayCutoff).toISOString(), filterApiKey]
  : [new Date(overlayCutoff).toISOString()];
```

### Step 4: Add filter to live history SQL (line ~550)

**Before**:
```js
`SELECT ... FROM usageHistory WHERE timestamp >= ?`,
[cutoff]
```

**After**:
```js
`SELECT ... FROM usageHistory WHERE timestamp >= ?${filterApiKey ? " AND apiKey = ?" : ""}`,
filterApiKey ? [cutoff, filterApiKey] : [cutoff]
```

### Step 5: Add filter to live history JS loop (line ~595)

**Before**:
```js
if (r.apiKey && typeof r.apiKey === "string") {
```

**After**:
```js
if (r.apiKey && typeof r.apiKey === "string") {
  if (filterApiKey && r.apiKey !== filterApiKey) continue;
```

**And**:
```js
} else {
```

**After**:
```js
} else {
  if (filterApiKey) continue;
```

## Dependencies

- be-1: `idx_uh_apiKey` index added (DONE)

## Success Criteria

- `getUsageStats("7d")` returns same results as before (backward compatible)
- `getUsageStats("7d", { apiKey: "sk-..." })` returns only matching key's data
- `getUsageStats("today", { apiKey: "sk-..." })` filters live history correctly
- `lastUsed` timestamps reflect only filtered key's data
- No existing callers broken
