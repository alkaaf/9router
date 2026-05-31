# Atomic Task: be-1 — Add `idx_uh_apiKey` Index (DB Schema)

**Domain**: Backend
**Priority**: High
**Estimated effort**: 5 min

---

## Input

- `src/lib/db/schema.js` — `usageHistory` table definition
- Current `indexes` array with 4 existing indexes

## Output

- New index added to `schema.js`
- Performance improvement for per-key queries on `usageHistory`

## Process

1. Open `src/lib/db/schema.js`
2. Locate the `usageHistory` table definition
3. Find the `indexes` array
4. Add the new index entry:
   ```js
   "CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)",
   ```

### Exact Change

**File**: `src/lib/db/schema.js` (line ~126)

**Before**:
```js
indexes: [
  "CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(timestamp DESC)",
  "CREATE INDEX IF NOT EXISTS idx_uh_provider ON usageHistory(provider)",
  "CREATE INDEX IF NOT EXISTS idx_uh_model ON usageHistory(model)",
  "CREATE INDEX IF NOT EXISTS idx_uh_conn ON usageHistory(connectionId)",
],
```

**After**:
```js
indexes: [
  "CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(timestamp DESC)",
  "CREATE INDEX IF NOT EXISTS idx_uh_provider ON usageHistory(provider)",
  "CREATE INDEX IF NOT EXISTS idx_uh_model ON usageHistory(model)",
  "CREATE INDEX IF NOT EXISTS idx_uh_conn ON usageHistory(connectionId)",
  "CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)",
],
```

## Dependencies

- None (Phase 0 — Foundation)

## Success Criteria

- File saved with new index
- App starts without errors
- Index auto-creates on boot via `syncSchemaFromTables()`
