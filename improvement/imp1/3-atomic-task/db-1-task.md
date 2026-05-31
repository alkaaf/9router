# Atomic Task: db-1 — Add `idx_uh_apiKey` Index to Schema

**Domain**: Database
**Priority**: High
**Estimated effort**: 5 min

---

## Input

- `src/lib/db/schema.js` — `usageHistory` table definition with `indexes` array
- Current indexes: `idx_uh_ts`, `idx_uh_provider`, `idx_uh_model`, `idx_uh_conn`

## Output

- New index entry in `schema.js` indexes array
- Index auto-created on next app boot via `syncSchemaFromTables()`

## Process

1. Open `src/lib/db/schema.js`
2. Find the `usageHistory` table's `indexes` array (around line 126)
3. Add new index entry:
   ```js
   "CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)",
   ```
4. Save file

### Exact Change

**File**: `src/lib/db/schema.js:126`

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

- None (first task in Phase 0)

## Success Criteria

- File saved with new index entry
- On next app boot, `syncSchemaFromTables()` creates the index
- No syntax errors in `schema.js`
