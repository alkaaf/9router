# Atomic Task: be-4 — Extend `getUsageHistory()` with `filter.apiKey`

**Domain**: Backend
**Priority**: High
**Estimated effort**: 10 min

---

## Input

- `src/lib/db/repos/usageRepo.js` — `getUsageHistory(filter = {})` function (line ~289)
- Existing filter parameters: `provider`, `model`, `startDate`, `endDate`

## Output

- `getUsageHistory()` accepts `filter.apiKey` parameter
- SQL query includes `AND apiKey = ?` when filter provided
- Backward compatible — no filter = all rows

## Process

### Step 1: Add apiKey filter condition (line ~296)

**Before**:
```js
if (filter.startDate) { conds.push("timestamp >= ?"); params.push(new Date(filter.startDate).toISOString()); }
if (filter.endDate) { conds.push("timestamp <= ?"); params.push(new Date(filter.endDate).toISOString()); }
```

**After**:
```js
if (filter.startDate) { conds.push("timestamp >= ?"); params.push(new Date(filter.startDate).toISOString()); }
if (filter.endDate) { conds.push("timestamp <= ?"); params.push(new Date(filter.endDate).toISOString()); }
if (filter.apiKey) { conds.push("apiKey = ?"); params.push(filter.apiKey); }
```

### Exact diff (2 lines added):

```diff
   if (filter.endDate) { conds.push("timestamp <= ?"); params.push(new Date(filter.endDate).toISOString()); }
+  if (filter.apiKey) { conds.push("apiKey = ?"); params.push(filter.apiKey); }
   const where = conds.length ? " WHERE " + conds.join(" AND ") : "";
```

## Dependencies

- be-1: `idx_uh_apiKey` index added (DONE) — ensures query performance

## Success Criteria

- `getUsageHistory({ apiKey: "sk-..." })` returns only rows for that key
- `getUsageHistory({ apiKey: "sk-...", provider: "openai" })` combines both filters
- `getUsageHistory({})` returns all rows (unchanged)
- `limit` and `offset` pagination still work with apiKey filter
