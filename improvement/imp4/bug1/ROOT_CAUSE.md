# Bug #1: `db.all(...).map is not a function` — ROOT CAUSE

## Summary

All 500 errors across `/api/settings/database`, `/api/combos`, `/api/providers`, `/api/provider-nodes`, `/api/models/availability` share **one root cause**: repo functions call `db.all()`, `db.get()`, `db.run()`, `db.transaction()` **WITHOUT `await`**.

This worked with SQLite because `better-sqlite3` provides a **synchronous** API. But the PostgreSQL adapter uses `pg`, which is **fully asynchronous** — all methods return Promises.

When `await` is missing, the code receives a Promise object instead of the actual result, causing:
- `Promise.map is not a function` — when calling `.map()` on a Promise
- `Promise.then is not a function` — when calling `.then()` on a Promise
- Silent data loss — when `db.run()` is not awaited, the INSERT/UPDATE never executes

## Root Cause Chain

```
API Route (async) → Repo function (async, but missing await)
  → db.all(...) → returns Promise, not array
  → .map() on Promise → TypeError: .map is not a function
```

## Affected Files (all repos + helpers)

| File | Missing await calls | Severity |
|------|-------------------|----------|
| `src/lib/db/index.js` (exportDb) | 11 calls (all db.all without await, line 77-91) | CRITICAL |
| `src/lib/db/repos/combosRepo.js` | 8 calls (all db calls inside async functions, no await) | CRITICAL |
| `src/lib/db/repos/apiKeysRepo.js` | 8 calls | HIGH |
| `src/lib/db/repos/nodesRepo.js` | 7 calls | HIGH |
| `src/lib/db/repos/pricingRepo.js` | 8 calls | HIGH |
| `src/lib/db/repos/disabledModelsRepo.js` | 10 calls | HIGH |
| `src/lib/db/repos/connectionsRepo.js` | ~12 calls | HIGH |
| `src/lib/db/repos/settingsRepo.js` | 4 calls | HIGH |
| `src/lib/db/helpers/metaStore.js` | 4 calls | MEDIUM |

**Total: ~80+ missing `await` calls across 10 files.**

## Why SQLite Worked

better-sqlite3 (SQLite): all methods are **synchronous**, return values directly.
```js
const rows = db.all("SELECT * FROM combos");  // returns array immediately
return rows.map(rowToCombo);  // works
```

## Why PostgreSQL Breaks

pg (PostgreSQL): all methods are **async**, return Promises.
```js
const rows = db.all("SELECT * FROM combos");  // returns Promise<object>, not array
return rows.map(rowToCombo);  // TypeError: .map is not a function
```

## Two Fix Strategies

### Option A: Add `await` everywhere (repository layer fix)

Add `await` to every `db.all()`, `db.get()`, `db.run()` call in every repo.

**Pros**: Clean, explicit, works with both adapters.
**Cons**: ~80 changes across 10 files. High risk of missing one. Must be consistent.

### Option B: Wrap pg adapter to expose sync-like API

Create a sync-wrapped version of the postgres adapter that blocks on Promises using `process.binding('async_wrap')` or a synchronous event loop.

**Pros**: Minimal changes to repo layer.
**Cons**: Complex, fragile, blocks the event loop. Not recommended.

### Option C: Auto-await via a sync wrapper utility (RECOMMENDED)

Create a `toSync()` utility that wraps async pg adapter calls to return values synchronously using `AsyncResource` + `sync domestic-wait`.

Actually simpler: **Create a `syncify(db)` wrapper** that returns a new db-like object where every async method blocks until resolved.

Wait — simplest solution:

### Option D: The pragmatic fix — make all repo functions consistently async + await all calls

Add `await` to ALL db calls in the repo functions. The repo functions are already `async`, so `await` is valid.

The specific critical fixes for the failing APIs:

1. **`exportDb`** in `src/lib/db/index.js` lines 77-91: add `await` to all 8 `db.all()` calls
2. **`combosRepo.js`**: lines 19, 25, 31, 46 — add `await` to `db.all`, `db.get`, `db.run`
3. **`connectionsRepo.js`**: add `await` to all db calls
4. **`nodesRepo.js`**: add `await` to all db calls

But `db.transaction()` is special — its callback runs synchronously in SQLite but uses client.query() in PG. For PG, the callback must be `async` and we must `await` the transaction call.

## The transaction() Problem

```js
// SQLite pattern (sync, callback runs synchronously):
db.transaction(() => {
  const row = db.get(`SELECT * FROM combos WHERE id = ?`, [id]);
  db.run(`UPDATE combos SET ...`);
});

// PG pattern (async, callback must be async, transaction must be awaited):
await db.transaction(async (txn) => {
  const row = await txn.get(`SELECT * FROM combos WHERE id = ?`, [id]);
  await txn.run(`UPDATE combos SET ...`);
});
```

The SQLite `transaction()` blocks until the callback completes. The PG `transaction()` is async and the callback MUST be async with `await` inside.

## Recommended Fix Plan

1. **Phase 1**: Fix critical path — `exportDb()` in `src/lib/db/index.js` (line 77-91)
2. **Phase 2**: Fix `combosRepo.js` — all 8 calls, especially inside transaction callbacks
3. **Phase 3**: Fix `connectionsRepo.js` — all db calls including transaction callbacks
4. **Phase 4**: Fix `nodesRepo.js` — all db calls including transaction callbacks
5. **Phase 5**: Fix remaining repos (apiKeysRepo, settingsRepo, pricingRepo, disabledModelsRepo, aliasRepo)
6. **Phase 6**: Fix `metaStore.js` helpers

Key principle: **inside `db.transaction(async (txn) => {...})` callback, ALL `txn.get/run/all` MUST be awaited**.

## Verification

After fixing, curl these endpoints should return 200:
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:20128/api/combos
curl -s -o /dev/null -w "%{http_code}" http://localhost:20128/api/providers
curl -s -o /dev/null -w "%{http_code}" http://localhost:20128/api/provider-nodes
curl -s -o /dev/null -w "%{http_code}" http://localhost:20128/api/models/availability
```

## Status

- Root cause identified: MISSING `await` on async db calls
- Scope: ~80+ calls across 10 files
- Complexity: HIGH — requires careful modification of every repo function
- Risk: Missing one `await` = subtle bug (silent INSERT failure or wrong data)
