# Domain 4: Repository Migration (Easy Repos)

## Overview

Migrate 10 repositories to use the PostgreSQL adapter. For these repos, the change is
mechanical: remove `parseJson`/`stringifyJson` calls (JSONB handles serialization
transparently), ensure `?` placeholders are used (adapter translates), and verify
`db.run` return shapes are normalized by the adapter. No SQL logic changes needed.

The 10 repos: settingsRepo, apiKeys, connections, proxyPools, requestDetails,
requestLogs (via requestDetails), batchRequests (part of usage), rateLimits (via kv),
rules (via kv), profiles (via kv).

Actually, based on the repo inventory: settingsRepo, apiKeysRepo, connectionsRepo,
proxyPoolsRepo, nodesRepo, combosRepo, aliasRepo (kv), pricingRepo (kv),
disabledModelsRepo (kv), requestDetailsRepo. That's 10 repos (11 files counting kv
helpers separately).

## Scope

- **In scope**: All repos except usageRepo.js (Domain 5). Remove JSON parse/stringify,
  verify adapter interface conformance, add integration tests.
- **Out of scope**: usageRepo.js (Domain 5), schema creation (Domain 2), K8s (Domain 6)

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `src/lib/db/repos/settingsRepo.js` | MODIFY | ~-5 (remove parseJson) |
| `src/lib/db/repos/apiKeysRepo.js` | MODIFY | ~-3 |
| `src/lib/db/repos/connectionsRepo.js` | MODIFY | ~-8 |
| `src/lib/db/repos/proxyPoolsRepo.js` | MODIFY | ~-5 |
| `src/lib/db/repos/nodesRepo.js` | MODIFY | ~-5 |
| `src/lib/db/repos/combosRepo.js` | MODIFY | ~-4 |
| `src/lib/db/repos/aliasRepo.js` | MODIFY | ~-3 |
| `src/lib/db/repos/pricingRepo.js` | MODIFY | ~-4 |
| `src/lib/db/repos/disabledModelsRepo.js` | MODIFY | ~-3 |
| `src/lib/db/repos/requestDetailsRepo.js` | MODIFY | ~-6 |
| `src/lib/db/helpers/kvStore.js` | MODIFY | ~-3 |
| `src/lib/db/helpers/metaStore.js` | MODIFY | ~-2 |
| `src/lib/db/helpers/jsonCol.js` | MODIFY (no-op for PG) | +5 guard |

## Dependencies

Domains 1, 2, 3 must be complete (adapter works, schema exists, driver selects it).

## Sub-Tasks

1. **Migrate settingsRepo.js**
   - Description: Remove `parseJson(row.data, {})` from `readRaw()` — PostgreSQL
     adapter returns JSONB as JS object directly, so `row.data` is already an object.
     Remove `stringifyJson(merged)` from `updateSettings()` — pass JS object directly.
     The `mergeWithDefaults()` logic stays unchanged.
   - Acceptance: `getSettings()` returns merged defaults + DB values. `updateSettings()`
     persists and re-reads correctly. 5 s cache still works.
   - Risk: Low

2. **Migrate apiKeysRepo.js**
   - Description: Remove `parseJson`/`stringifyJson` (no JSON columns in this table).
     Verify `validateApiKey` hot path: `SELECT isActive FROM apiKeys WHERE key = ?`
     returns a row with `isActive` as a Boolean (not 0/1). The `rowToKey` mapping
     must handle both `BOOLEAN` and `INTEGER` return types.
   - Acceptance: `validateApiKey("some-key")` returns `true`/`false` (Boolean).
     `res.changes` from `deleteApiKey` is normalized by adapter (Number).
   - Risk: Low (but this is the auth hot path — test carefully)

3. **Migrate connectionsRepo.js**
   - Description: Remove `parseJson`/`stringifyJson` from `data` column handling.
     Verify `reorderInTx()` N+1 UPDATE loop works within async transaction. The
     JS-side sort by `priority` stays (no SQL ORDER BY change needed — PostgreSQL
     can sort in JS just fine for <100 rows).
   - Acceptance: Create, read, update, delete, reorder all work. Transaction rollback
     on error works.
   - Risk: Low

4. **Migrate proxyPoolsRepo.js, nodesRepo.js, combosRepo.js**
   - Description: Same pattern — remove JSON parse/stringify from `data`/`models`
     columns. Verify CRUD operations.
   - Acceptance: All CRUD operations produce identical return shapes as SQLite path.
   - Risk: Low

5. **Migrate kv-based repos (aliasRepo, pricingRepo, disabledModelsRepo)**
   - Description: These repos use `kvStore` helper. Update `kvStore.js` to skip
     `parseJson`/`stringifyJson` when using PostgreSQL (JSONB returns objects). The
     composite PK upsert `ON CONFLICT(scope, key)` syntax is identical in PostgreSQL
     (just requires parens: `ON CONFLICT (scope, key)` — but the existing code already
     uses parens in kvStore).
   - Acceptance: `kv.get()`, `kv.set()`, `kv.remove()`, `kv.getAll()`, `kv.clear()`
     all work identically.
   - Risk: Low

6. **Migrate requestDetailsRepo.js**
   - Description: Remove `parseJson` from `data` column. Verify the self-pruning
     DELETE subquery works in PostgreSQL. The UPSERT pattern `ON CONFLICT(id) DO
     UPDATE SET ...` is identical.
   - Acceptance: `saveRequestDetail()` queues and flushes. `getRequestDetails()`
     paginates correctly. Old rows are trimmed when `COUNT(*) > maxRecords`.
   - Risk: Low

7. **Update jsonCol.js helpers**
   - Description: Add a guard: if the adapter is PostgreSQL, `parseJson(text, default)`
     returns `text ?? default` (already an object). `stringifyJson(obj)` returns
     `obj` directly (pg handles serialization). This keeps the helper working for
     both backends without touching every call site.
   - Acceptance: All existing `parseJson`/`stringifyJson` call sites work unchanged
     under both SQLite and PostgreSQL.
   - Risk: Low (this is the key abstraction that makes the migration mechanical)

8. **Run existing unit tests against PostgreSQL**
   - Description: For each repo, run the existing test suite with `POSTGRES_URL`
     pointing to a test database. Fix any adapter-shape mismatches.
   - Acceptance: All existing unit tests pass with both SQLite (default) and
     PostgreSQL (POSTGRES_URL set) adapters.
   - Risk: Medium (uncovered edge cases may surface)

## Effort Estimate

3-5 days (can be parallelized across team members by repo)

## Risk Level

Low — these are mechanical changes with well-defined acceptance criteria.

## Testing Requirements

- Unit tests: Yes — each repo's existing tests must pass with both adapters
- Integration tests: Yes — add integration tests for each repo against PostgreSQL
- Manual testing: Full smoke test — create connections, make API calls, verify
  usage recorded, check dashboard
