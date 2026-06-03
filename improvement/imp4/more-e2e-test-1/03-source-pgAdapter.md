# postgresAdapter.js — Source Code Analysis

## Core methods

### run(sql, params)
- Converts `?` placeholders to `$1, $2, ...` via `convertPlaceholders()`.
- Calls `pool.query(convertedSql, params)`.
- Returns `{ changes: Number(r.rowCount), lastInsertRowid: ... }` — coerces `rowCount` and `id` to Number.
- `lastInsertRowid` is only populated when the SQL includes `RETURNING <id>`.

### get(sql, params)
- Same placeholder conversion, calls `pool.query()`.
- Returns `r.rows[0]` (first row) or `undefined` if no results.

### all(sql, params)
- Same pattern, returns `r.rows` (full array).

### exec(sql, params)
- Splits SQL on `;` boundaries (`sql.split(/;\s*(?:\n|$)/)`) to support multi-statement DDL bundles.
- Executes each statement sequentially via `pool.query()`.
- Returns `{ lastInsertRowid, changes }` from the last statement's result.
- Important: `convertPlaceholders()` is applied to each split statement independently. If params are provided, they apply to ALL statements — this works for DDL (which has no params) but could be problematic for mixed SQL.

### transaction(fn)
- Checks out a client from the pool.
- Calls `BEGIN`, executes `fn(txAdapter)`, then `COMMIT`.
- On error: `ROLLBACK`, then re-throws.
- Finally: releases client back to pool.
- The `txAdapter` returned by `createTransactionAdapter()` pins all queries to the same client.
- Nested transactions: The txn adapter's `transaction()` method uses `SAVEPOINT`/`RELEASE SAVEPOINT`/`ROLLBACK TO SAVEPOINT` instead of `BEGIN`/`COMMIT` to avoid "already in transaction" errors.

### Transaction adapter methods
The `createTransactionAdapter(client, parent)` creates a lightweight wrapper:
- run/get/all/exec: All call `client.query()` (same connection).
- transaction(fn): Uses savepoints.
- prepare(sql): Delegates to `parent.prepare(sql, client)` for per-connection prepared statements.

## Placeholder conversion
```js
function convertPlaceholders(sql) {
  let i = 0;
  return sql.replace(/\?/g, () => `$${++i}`);
}
```
Simple sequential replacement — counts all `?` characters. Does NOT skip over `?` inside string literals. Callers are expected to use parameter binding for all user data.

## Prepared statements
Cached on `global._pgPool.prepared` (a Map). The cache stores the converted SQL (with `$1, $2` placeholders) to avoid re-translation on hot paths.

## Type handling notes
- rowCount from pg is coerced to Number() — matches SQLite adapter behavior.
- Column names from pg are returned as-is (pg preserves case by quoting identifiers; unquoted identifiers are lowercased).
- JSONB values are auto-serialized/deserialized by the pg driver.
- BOOLEAN values are passed as JS true/false and mapped natively.
- TIMESTAMPTZ values are returned as JS Date objects or ISO strings depending on the pg driver configuration.
