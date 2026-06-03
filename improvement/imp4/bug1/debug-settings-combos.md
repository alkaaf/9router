# Debug Report: `db.all(...).map is not a function`

## Symptoms

Two API endpoints fail with `db.all(...).map is not a function` (or similar) when 9Router uses the PostgreSQL adapter:

1. `POST /api/settings/database` → triggers `exportDb()` → chains `.map()` on `db.all()` result
2. `GET /api/combos` → `getCombos()` → chains `.map()` on `db.all()` result

## Root Cause

**Missing `await` on `db.all()` / `db.get()` / `db.run()` calls in all non-async repo functions.**

The PostgreSQL adapter (`src/lib/db/adapters/postgresAdapter.js`) exposes **async** methods:

```js
// postgresAdapter.js line 224-226
async all(sql, params) {
  const r = await pool.query(convertPlaceholders(sql), params);
  return r.rows.map(transformRowKeys);  // returns Promise<Array>
}
```

The better-sqlite3 adapter (`src/lib/db/adapters/betterSqliteAdapter.js`) exposes **sync** methods:

```js
// betterSqliteAdapter.js line 43
all(sql, params = []) { return prepare(sql).all(params); }  // returns Array directly
```

When running with SQLite, the sync return is fine. When running with PostgreSQL, the repos get a **Promise** instead of an array, and calling `.map()` on a `Promise` throws `TypeError: db.all(...).map is not a function`.

## Affected Files (all non-await calls)

### Critical (causes the reported errors)

| File | Function | Missing await on |
|------|----------|-----------------|
| `src/lib/db/index.js` | `exportDb()` | `db.all()` (8 occurrences, lines 77-91) |
| `src/lib/db/repos/combosRepo.js` | `getCombos()` | `db.all()` (line 19) |
| `src/lib/db/repos/combosRepo.js` | `getComboById()` | `db.get()` (line 25) |
| `src/lib/db/repos/combosRepo.js` | `getComboByName()` | `db.get()` (line 31) |
| `src/lib/db/repos/combosRepo.js` | `createCombo()` | `db.run()` (line 46) |
| `src/lib/db/repos/combosRepo.js` | `updateCombo()` | `db.get()` (line 57), `db.run()` (line 60) |
| `src/lib/db/repos/combosRepo.js` | `deleteCombo()` | `db.run()` (line 71) |

### High (same pattern, other repos)

| File | Function | Missing awaits |
|------|----------|----------------|
| `src/lib/db/repos/settingsRepo.js` | `readRaw()`, `updateSettings()` | `db.get()`, `db.run()` (lines 47, 91, 99) |
| `src/lib/db/repos/connectionsRepo.js` | `getProviderConnections()`, `getProviderConnectionById()`, `reorderInTx()`, `createProviderConnection()`, `updateProviderConnection()`, `deleteProviderConnection()`, `deleteProviderConnectionsByProvider()`, `cleanupProviderConnections()` | `db.all()`, `db.get()`, `db.run()` (lines 66, 74, 80, 87, 97, 161, 176, 178, 187, 188, 208) |
| `src/lib/db/repos/nodesRepo.js` | `getProviderNodes()`, `getProviderNodeById()`, `createProviderNode()`, `updateProviderNode()`, `deleteProviderNode()` | `db.all()`, `db.get()`, `db.run()` (lines 47, 52, 76, 89, 92) |
| `src/lib/db/repos/proxyPoolsRepo.js` | `getProxyPools()`, `getProxyPoolById()`, `createProxyPool()`, `updateProxyPool()`, `deleteProxyPool()` | `db.all()`, `db.get()`, `db.run()` (lines 49, 56, 84, 97, 100) |
| `src/lib/db/repos/apiKeysRepo.js` | `getApiKeys()`, `getApiKeyById()`, `createApiKey()`, `updateApiKey()`, `deleteApiKey()`, `validateApiKey()` | `db.all()`, `db.get()`, `db.run()` (lines 27, 33, 51, 62, 65, 76, 82) |
| `src/lib/db/repos/disabledModelsRepo.js` | All functions | `db.all()`, `db.get()`, `db.run()` (8 calls) |
| `src/lib/db/repos/pricingRepo.js` | All functions | `db.get()`, `db.run()` (6 calls) |
| `src/lib/db/repos/aliasRepo.js` | `addCustomModel()` | `db.get()`, `db.run()` (lines 38, 41) |
| `src/lib/db/repos/usageRepo.js` | All non-async functions | `db.all()`, `db.run()` (15+ calls, lines 114, 294, 303, 324, 399, 447, 505, 655, 686, 797, 818, 839, 867, 955, 964) |

### Helpers with same issue

| File | Function | Missing awaits |
|------|----------|----------------|
| `src/lib/db/helpers/kvStore.js` | `get()`, `getAll()`, `set()`, `setMany()`, `remove()`, `clear()` | `db.get()`, `db.all()`, `db.run()` |

### Only correctly-async repo

`src/lib/db/repos/requestDetailsRepo.js` — all `db.all()`, `db.get()`, `db.run()` calls properly use `await` (lines 102, 108, 110, 155, 163, 177).

## Fix Strategy

The fix requires two changes to each affected repo function:

1. **Add `await`** to all `db.all()`, `db.get()`, `db.run()` calls
2. **Functions called inside `db.transaction(fn)** — the transaction callback must **not** be made async (it runs sync inside the adapter's transaction wrapper). Instead, the repo functions calling `db.transaction(fn)` must `await` the transaction call itself

For example, `combosRepo.js` fix:

```js
// BEFORE (buggy):
export async function getCombos() {
  const db = await getAdapter();
  const rows = db.all(`SELECT * FROM combos ORDER BY createdAt ASC`);
  return rows.map(rowToCombo);
}

// AFTER (fixed):
export async function getCombos() {
  const db = await getAdapter();
  const rows = await db.all(`SELECT * FROM combos ORDER BY createdAt ASC`);
  return rows.map(rowToCombo);
}
```

For transaction-based functions:

```js
// BEFORE (buggy):
export async function createCombo(data) {
  const db = await getAdapter();
  // ...
  db.transaction(() => {
    // ...
    db.run(`INSERT INTO combos...`);
  });
  return combo;
}

// AFTER (fixed):
export async function createCombo(data) {
  const db = await getAdapter();
  // ...
  await db.transaction(async (txn) => {
    // Inside transaction, all calls must use `txn`
    await txn.run(`INSERT INTO combos...`);
  });
  return combo;
}
```

**Note on transaction callbacks**: The postgres adapter's `transaction()` method accepts `async fn` and awaits it internally (line 248: `await client.query("BEGIN")`). But inside the callback, `db.get()`, `db.run()`, etc. returned Promises, so they still need `await` — OR switch to the transaction adapter (`txn`) passed as the argument and `await` those.

## Why the error says `.map is not a function`

When `db.all()` is called without `await` on postgres, it returns a `Promise<Array>`. JavaScript then tries to call `.map()` on that `Promise` object. `Promise.prototype.map` does not exist (only `Promise.all`, `Promise.race`, etc.), so the error is `TypeError: db.all(...).map is not a function`.

## Why auth returned "Unauthorized" first

`src/dashboardGuard.js` (line 189-192) intercepts `/api/settings` and `/api/combos` requests. It calls `await isAuthenticated(request)` → `await loadSettings()` → `await getSettings()` → `await readRaw()`. If `readRaw()` hits the broken code path (e.g., when settings need to be read), it throws and `loadSettings()` catches the error and returns `null`. With no settings, `isAuthenticated()` returns false, and the guard returns `{"error": "Unauthorized"}` before the actual route handler even runs.

The real `.map is not a function` error would surface when:
1. `requireLogin = false` is set in the database (so auth passes)
2. Or when calling the endpoints from a properly authenticated session

## Additional Note on `parseJson`

`src/lib/db/helpers/jsonCol.js` `parseJson()` accepts non-string values:

```js
export function parseJson(str, fallback = null) {
  if (str == null) return fallback;
  if (typeof str !== "string") return str;  // pg JSONB returns object — passes through safely
  try { return JSON.parse(str); } catch { return fallback; }
}
```

This is safe — PostgreSQL JSONB columns return JS objects (already parsed), and `parseJson()` just returns them as-is. No issue there.
