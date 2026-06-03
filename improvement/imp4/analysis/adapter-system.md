# Database Adapter System Analysis — imp4

## 1. Adapter Selection Logic

**File:** `src/lib/db/driver.js`

Priority order is runtime-dependent:

### Bun Runtime (lines 57-59)
```
bun:sqlite → sql.js
```
- `tryBunSqlite()` (line 7-17): Checks `process.versions.bun`, dynamically imports `bun:sqlite` adapter
- `trySqlJs()` (line 45-53): Final fallback using `sql.js` (in-memory SQLite compiled to WASM)

### Node Runtime (lines 57-59)
```
better-sqlite3 → node:sqlite (≥22.5) → sql.js
```
- `tryBetterSqlite()` (line 19-29): First choice on Node (skips Bun — native bindings incompatible)
- `tryNodeSqlite()` (line 31-43): Built-in since Node 22.5.0, checks version explicitly (line 34-35: `maj < 22 || (maj === 22 && min < 5)`)
- `trySqlJs()` (line 45-53): Final fallback

### Singleton Pattern (lines 3-5, 76-85)
Uses `global._dbAdapter` to survive Next.js hot-reload. `getAdapter()` returns a promise (lazy init), `getAdapterSync()` throws if not initialized.

---

## 2. Each Adapter Implementation

### 2.1 bunSqliteAdapter (`src/lib/db/adapters/bunSqliteAdapter.js`)

| Property | Detail |
|----------|--------|
| Runtime | Bun only |
| Sync/Async | Async factory (`createBunSqliteAdapter`), sync operations |
| Native | Yes — Bun built-in `bun:sqlite` |
| Performance | Fastest under Bun (no transpilation layer) |

**Key characteristics:**
- Dynamic import of `bun:sqlite` (line 9)
- `Database(filePath, { create: true })` — creates DB if missing (line 10)
- Statement cache via `Map` (lines 13-21)
- Returns `{ changes, lastInsertRowid }` as Numbers (line 42) — explicitly coerced

### 2.2 betterSqliteAdapter (`src/lib/db/adapters/betterSqliteAdapter.js`)

| Property | Detail |
|----------|--------|
| Runtime | Node (excluded on Bun) |
| Sync/Async | Synchronous |
| Native | Yes — `better-sqlite3` C++ binding |
| Performance | Fastest on Node (native C++ wrapper) |

**Key characteristics:**
- Synchronous `new Database(filePath)` (line 8)
- Statement cache via `Map` (lines 12-21)
- Returns raw `better-sqlite3` statement results (not wrapped)
- No explicit `changes`/`lastInsertRowid` coercion — relies on native return values

### 2.3 nodeSqliteAdapter (`src/lib/db/adapters/nodeSqliteAdapter.js`)

| Property | Detail |
|----------|--------|
| Runtime | Node ≥22.5.0 (excluded on Bun) |
| Sync/Async | Async factory, sync operations |
| Native | Yes — built-in `node:sqlite` |
| Performance | Native, slightly slower than better-sqlite3 |

**Key characteristics:**
- Suppresses `ExperimentalWarning: SQLite` via `process.emit` override (lines 10-16)
- Uses `DatabaseSync` class (line 20)
- Statement cache via `Map` (lines 25-33)
- Transaction uses SAVEPOINT pattern (lines 64-76) because `node:sqlite` lacks native `transaction()` wrapper
- Returns `{ changes, lastInsertRowid }` as Numbers (line 55) — explicitly coerced

### 2.4 sqljsAdapter (`src/lib/db/adapters/sqljsAdapter.js`)

| Property | Detail |
|----------|--------|
| Runtime | Any (fallback) |
| Sync/Async | Async factory, sync operations |
| Native | No — WASM-based (`sql.js`) |
| Performance | Slowest — in-memory DB with disk persistence |

**Key characteristics:**
- Loads WASM SQLite via `initSqlJs()` (lines 7-11), caches the `SQL` global
- Reads existing DB file into memory on init (line 15): `fs.readFileSync(filePath)`
- Debounced file persistence — saves to disk after 100ms of inactivity (lines 21-39)
- No statement cache in `run/get/all` (creates fresh stmt each call, lines 46-81)
- Has a separate `prepare()` cache (lines 83-112) for prepared statement reuse
- `lastInsertRowid` fetched via extra query: `db.exec("SELECT last_insert_rowid() as id")` (line 52)
- No WAL, no checkpoint timer — in-memory only
- `paramsObj()` helper converts empty arrays to `undefined` (lines 41-44) because sql.js `bind()` needs `undefined` for no-params

---

## 3. SQLite-Specific Features

**File:** `src/lib/db/schema.js`, lines 4-12

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA temp_store = MEMORY;
PRAGMA mmap_size = 30000000;
PRAGMA cache_size = -64000;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
```

| PRAGMA | Value | Purpose |
|--------|-------|---------|
| `journal_mode` | `WAL` | Write-Ahead Logging — concurrent reads during writes |
| `synchronous` | `NORMAL` | Balance between durability and speed (fsync at checkpoint, not every commit) |
| `temp_store` | `MEMORY` | Temporary tables/indices in RAM |
| `mmap_size` | `30000000` (30MB) | Memory-mapped I/O for read-heavy workloads |
| `cache_size` | `-64000` (64MB, negative = KB) | Page cache size |
| `foreign_keys` | `ON` | Enforce foreign key constraints |
| `busy_timeout` | `5000` (5s) | Wait for locks instead of failing immediately |

Applied uniformly across all adapters via `db.exec(PRAGMA_SQL)` on init.

---

## 4. Adapter API Surface

All adapters expose this interface:

| Method | Signature | Description |
|--------|-----------|-------------|
| `run(sql, params?)` | `({ changes, lastInsertRowid })` | INSERT/UPDATE/DELETE |
| `get(sql, params?)` | `row | undefined` | Single row SELECT |
| `all(sql, params?)` | `row[]` | Multi-row SELECT |
| `exec(sql)` | `void` | DDL/multi-statement |
| `transaction(fn)` | `fn()` result | Atomic block |
| `checkpoint()` | `void` | Manual WAL checkpoint |
| `close()` | `void` | Cleanup + final checkpoint |
| `raw` | `db instance` | Access underlying driver |
| `prepare(sql)` | `{ run, get, all }` | Cached prepared statement |

### Differences in return shapes:
- **better-sqlite3**: Returns native statement results directly (no wrapping)
- **bun:sqlite, node:sqlite, sql.js**: Return `{ changes: Number, lastInsertRowid: Number }` — explicitly coerced to Number

---

## 5. Periodic WAL Checkpoint

**All adapters except sql.js** implement a 60-second checkpoint timer:

**File locations:**
- `betterSqliteAdapter.js` line 5, 24-27
- `bunSqliteAdapter.js` line 5, 23-26
- `nodeSqliteAdapter.js` line 5, 36-39

```js
const CHECKPOINT_INTERVAL_MS = 60 * 1000;
const checkpointTimer = setInterval(() => {
  try { db.pragma("wal_checkpoint(TRUNCATE)"); } catch {}
}, CHECKPOINT_INTERVAL_MS);
if (typeof checkpointTimer.unref === "function") checkpointTimer.unref();
```

**Purpose:** `TRUNCATE` mode resets the WAL file to zero bytes after checkpointing, preventing unbounded growth of `-wal`/`-shm` files for backup/copy scenarios.

**Shutdown sequence (all adapters):**
1. Run final `wal_checkpoint(TRUNCATE)`
2. Clear statement cache
3. Close database connection
4. Registered on `beforeExit`, `SIGINT`, `SIGTERM`

**sql.js** has no WAL checkpoint (in-memory DB), but debounces disk persistence (100ms).

---

## 6. Migration Considerations for PostgreSQL

### 6.1 Adapter Interface → PostgreSQL Translation

| Adapter Method | PostgreSQL Equivalent |
|----------------|----------------------|
| `run(sql, params)` | `client.query(sql, params)` — for INSERT/UPDATE/DELETE |
| `get(sql, params)` | `(await client.query(sql, params)).rows[0]` |
| `all(sql, params)` | `(await client.query(sql, params)).rows` |
| `exec(sql)` | `await client.query(sql)` — for DDL |
| `transaction(fn)` | `await client.query('BEGIN'); try { r = await fn(); await client.query('COMMIT'); } catch { ROLLBACK }` |
| `checkpoint()` | No equivalent — remove |
| `close()` | `await client.end()` |
| `raw` | `client` object itself |
| `prepare(sql)` | PostgreSQL uses server-side prepared statements differently; `client.query(sql, params)` handles param binding |

### 6.2 SQLite-Specific Features → PostgreSQL Mapping

| Feature | SQLite | PostgreSQL | Action |
|---------|--------|-----------|--------|
| WAL mode | `journal_mode=WAL` | Native MVCC | Remove — PostgreSQL is always MVCC |
| `synchronous=NORMAL` | Checkpoint-based durability | `synchronous=on` (default) | Map to PostgreSQL `synchronous` setting |
| `busy_timeout=5000` | Retry on lock | Row-level locks, no busy timeout needed | Remove |
| `mmap_size=30MB` | Memory-mapped I/O | `effective_cache_size` | Map to `effective_cache_size` or `work_mem` |
| `cache_size=-64000` | 64MB page cache | `shared_buffers` | Map to `shared_buffers` |
| `temp_store=MEMORY` | Temp tables in RAM | `temp_tablespaces`, `work_mem` | Remove or map to `work_mem` |
| `foreign_keys=ON` | FK enforcement | Native FK enforcement | No change needed |

### 6.3 Schema Changes Required

1. **Column types:** SQLite's loose typing (`TEXT`, `INTEGER`, `REAL`) maps directly to PostgreSQL. However:
   - `INTEGER PRIMARY KEY` in SQLite = alias for ROWID → maps to `SERIAL`/`BIGSERIAL` in PostgreSQL
   - `TEXT NOT NULL` → `TEXT NOT NULL` (same)
   - `REAL DEFAULT 0` → `REAL DEFAULT 0` (same)

2. **Auto-increment:** `usageHistory.id INTEGER PRIMARY KEY AUTOINCREMENT` → `SERIAL PRIMARY KEY` or `BIGSERIAL`

3. **Composite primary keys:** `kv` table uses `PRIMARY KEY (scope, key)` inline syntax — works as-is in PostgreSQL

4. **CHECK constraints:** `settings.id INTEGER PRIMARY KEY CHECK (id = 1)` — supported in PostgreSQL

5. **Indexes:** All `CREATE INDEX IF NOT EXISTS` syntax is compatible

### 6.4 Adapter Pattern for PostgreSQL

Recommended approach:
- Create a `postgresAdapter.js` implementing the same interface
- Use `pg` or `postgres.js` library
- Remove WAL checkpoint logic entirely
- Replace statement caching with PostgreSQL's native prepared statements
- Connection pooling via `pg.Pool` instead of singleton

### 6.5 Key Risks

1. **sql.js fallback behavior:** In serverless/K8s environments, sql.js (WASM) may have cold-start penalties. PostgreSQL eliminates this concern.

2. **Transaction semantics:** `node:sqlite` uses SAVEPOINT for transactions. PostgreSQL supports native BEGIN/COMMIT/ROLLBACK — simpler but needs testing for nested transaction patterns.

3. **`changes`/`lastInsertRowid` coercion:** bun:sqlite and node:sqlite explicitly coerce to `Number`. PostgreSQL returns these as strings by default — ensure consistent coercion.

4. **File-based persistence → connection-based:** All adapters use file paths. PostgreSQL requires connection strings and handles persistence server-side.
