# kvStore.js — Source Code Analysis

## Storage/Retrieval mechanism

`makeKv(scope)` returns a scoped key-value store object with methods: `get`, `getAll`, `set`, `setMany`, `remove`, `clear`.

### get(key, fallback)
```js
const row = db.get(`SELECT value FROM kv WHERE scope = ? AND key = ?`, [scope, key]);
if (!row) return fallback;
const val = isPg(db) ? row.value : parseJson(row.value, fallback);
return val ?? fallback;
```

### set(key, value)
```js
db.run(`INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
  [scope, key, isPg(db) ? value : stringifyJson(value)]);
```

## JSONB behavior — critical difference between PG and SQLite

### SQLite path
- The `kv.value` column is `TEXT`.
- `set()` calls `stringifyJson(value)` before storing — serializes JS objects to JSON strings.
- `get()` calls `parseJson(row.value, fallback)` — deserializes JSON strings back to JS objects.
- Round-trip: JS object → JSON string → stored as TEXT → JSON string → JS object.

### Postgres path
- The `kv.value` column is `JSONB`.
- `set()` passes `value` **directly** (no `stringifyJson`) — stores JS object natively as JSONB.
- `get()` returns `row.value` **directly** — pg driver auto-parses JSONB to JS object.
- Round-trip: JS object → stored as JSONB → JS object (no serialization step).

### Key insight
On Postgres, the kvStore correctly bypasses `stringifyJson`/`parseJson` because pg's JSONB driver handles serialization natively. The `isPg(db)` check at line 5 drives this branching.

**However**, this creates an asymmetry: callers that bypass kvStore and directly `db.run()` an INSERT with `JSON.stringify(obj)` into a JSONB column will get double-encoded values (JSON string wrapped inside JSONB). This matters for the usageRepo tests, which manually construct INSERT statements using `JSON.stringify()`.

## Schema
```sql
CREATE TABLE IF NOT EXISTS kv (
  scope TEXT NOT NULL,
  key   TEXT NOT NULL,
  value JSONB NOT NULL,     -- PG: JSONB; SQLite: TEXT
  PRIMARY KEY (scope, key)
);
```
