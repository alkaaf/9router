# Analysis 8: Database Migrations — How the Database Is Created & Managed

## Overview

Analisis bagaimana database SQLite di 9Router diciptakan, di-run, dan di-manage. Termasuk: schema definition, migration system, driver abstraction, dan cara menambahkan perubahan skema baru.

## Database File Location

**File**: `src/lib/db/paths.js`

```
<DATA_DIR>/db/data.sqlite          ← Database file utama
<DATA_DIR>/db/backups/             ← Backup directory
<DB_DIR>/.migrated-from-json       ← Marker file (setelah import dari legacy JSON)
```

Legacy JSON files (jika ada): `<DATA_DIR>/db.json`, `<DATA_DIR>/usage.json`, `<DATA_DIR>/disabledModels.json`, `<DATA_DIR>/request-details.json`

## Migration System Structure

### Directory

```
src/lib/db/migrations/
├── index.js              ← Registry — imports semua migrations, exports sorted array
└── 001-initial.js        ← Initial schema creation (satu-satunya migration saat ini)
```

### Registry (`src/lib/db/migrations/index.js`)

```js
import m001 from "./001-initial.js";
export const MIGRATIONS = [m001].sort((a, b) => a.version - b.version);
export function latestVersion() {
  return MIGRATIONS.length ? MIGRATIONS[MIGRATIONS.length - 1].version : 0;
}
```

Setiap migration module export object dengan:
- `version: number` — monotonically increasing
- `name: string` — human-readable label
- `up(db): void` — function yang menerima adapter dan menjalankan DDL

### Initial Migration (`src/lib/db/migrations/001-initial.js`)

```js
import { TABLES, buildCreateTableSql } from "../schema.js";
export default {
  version: 1,
  name: "initial",
  up(db) {
    for (const [name, def] of Object.entries(TABLES)) {
      db.exec(buildCreateTableSql(name, def));  // CREATE TABLE IF NOT EXISTS
      for (const idx of def.indexes || []) db.exec(idx);  // CREATE INDEX IF NOT EXISTS
    }
  },
};
```

Iterasi semua table di `schema.js` dan jalankan `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS` untuk masing-masing.

## Schema Definition

**File**: `src/lib/db/schema.js` (157 lines)

Tabel didefinisikan declaratively sebagai object `TABLES`:

```js
export const TABLES = {
  _meta: {
    columns: { key: "TEXT PRIMARY KEY", value: "TEXT" },
  },
  usageHistory: {
    columns: {
      id: "INTEGER PRIMARY KEY AUTOINCREMENT",
      timestamp: "TEXT",
      provider: "TEXT",
      model: "TEXT",
      connectionId: "TEXT",
      apiKey: "TEXT",
      endpoint: "TEXT",
      promptTokens: "INTEGER DEFAULT 0",
      completionTokens: "INTEGER DEFAULT 0",
      cost: "REAL DEFAULT 0",
      status: "INTEGER",
      tokens: "INTEGER DEFAULT 0",
      meta: "TEXT",
    },
    indexes: [
      "CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(timestamp DESC)",
      "CREATE INDEX IF NOT EXISTS idx_uh_provider ON usageHistory(provider)",
      "CREATE INDEX IF NOT EXISTS idx_uh_model ON usageHistory(model)",
      "CREATE INDEX IF NOT EXISTS idx_uh_conn ON usageHistory(connectionId)",
    ],
  },
  // ... 9 tabel lainnya
};
```

**Tables yang ada (10):**

| Table | Columns | Indexes |
|-------|---------|---------|
| `_meta` | key (PK), value | — |
| `settings` | id (PK, CHECK=1), data | — |
| `providerConnections` | 11 columns | 3 indexes |
| `providerNodes` | 5 columns | 1 index |
| `proxyPools` | 5 columns | 2 indexes |
| `apiKeys` | 6 columns | 1 index (`idx_ak_key`) |
| `combos` | 5 columns | 1 index |
| `kv` | scope, key, value (PK: scope+key) | 1 index |
| `usageHistory` | 12 columns | 4 indexes |
| `usageDaily` | dateKey (PK), data | — |
| `requestDetails` | 6 columns | 3 indexes |

## Migration Runner

**File**: `src/lib/db/migrate.js` (287 lines)

### Entry Point: `runMigrationOnce(adapter)`

Dipanggil tepat SATU KALI per adapter instance (guarded oleh `WeakSet` di line 15).

### 4 Tahap Eksekusi:

#### Tahap 1 — Versioned Migrations (line 57)

```js
function runVersionedMigrations(adapter) {
  // 1. Bootstrap _meta table
  adapter.exec(buildCreateTableSql("_meta", TABLES._meta));
  
  // 2. Baca schemaVersion saat ini dari _meta
  const current = parseInt(getMetaSync(adapter, "schemaVersion", "0"), 10) || 0;
  
  // 3. Tentukan target version
  const target = latestVersion();  // dari migrations/index.js
  
  // 4. Jalankan semua migration yang belum applied
  const pending = MIGRATIONS.filter((m) => m.version > current);
  for (const m of pending) {
    adapter.transaction(() => {
      m.up(adapter);                          // jalankan DDL
      setMetaSync(adapter, "schemaVersion", m.version);  // update version
    });
  }
}
```

**Skip-version safe**: Jika `_meta` = 0 dan ada migration 1, 2, 3 → semua 3 dijalankan berurutan.

#### Tahap 2 — Additive Schema Sync (line 79)

```js
function syncSchemaFromTables(adapter) {
  for (const [tableName, def] of Object.entries(TABLES)) {
    // CREATE TABLE IF NOT EXISTS
    adapter.exec(buildCreateTableSql(tableName, def));
    
    // Diff columns via PRAGMA table_info
    // ALTER TABLE ADD COLUMN untuk kolom yang belum ada
    
    // CREATE INDEX IF NOT EXISTS untuk indexes baru
  }
}
```

**Best-effort additive sync** — secara otomatis:
- Buat table yang belum ada
- Tambah kolom yang belum ada (`ALTER TABLE ADD COLUMN`)
- Buat index yang belum ada (`CREATE INDEX IF NOT EXISTS`)

**Catatan**: SQLite tidak allow `PRIMARY KEY` / `UNIQUE` constraint di `ALTER TABLE ADD COLUMN`, jadi helper tersebut stripping constraint tersebut saat sync kolom.

#### Tahap 3 — Legacy JSON Import (line 231)

Jika database fresh (tidak ada data di `_meta`):
- Deteksi file legacy JSON (`db.json`, `usage.json`, `disabledModels.json`, `request-details.json`)
- Import ke SQLite
- Tulis marker `.migrated-from-json`
- Backup file asli

Jika ada assertion gagal → rollback, marker tidak ditulis, boot berikutnya retry.

#### Tahap 4 — App Version Backup (line 272)

Jika `appVersion` di `_meta` beda dengan `package.json` version:
- Backup `data.sqlite` dengan timestamp
- Safety net sebelum migrasi besar

## Driver System

**File**: `src/lib/db/driver.js` (85 lines)

### `getAdapter()` — Lazy Singleton

```js
const global._dbAdapter = { instance: null, initPromise: null };

export async function getAdapter() {
  if (global._dbAdapter.instance) return global._dbAdapter.instance;
  if (global._dbAdapter.initPromise) return global._dbAdapter.initPromise;
  
  global._dbAdapter.initPromise = (async () => {
    ensureDirs();                                    // Buat DATA_DIR/db/backups/
    const driver = await pickDriver();               // Coba driver dalam urutan prioritas
    const db = await driver();                       // Initialize database
    await runMigrationOnce(db);                      // Jalankan migrations + sync
    return db;
  })();
  
  return global._dbAdapter.initPromise;
}
```

### Driver Priority (tried in order):

| Runtime | 1st Choice | 2nd Choice |
|---------|-----------|-----------|
| **Bun** | `bun:sqlite` | `sql.js` |
| **Node** | `better-sqlite3` | `node:sqlite` (>=22.5) → `sql.js` |

Setiap driver factory di-import dynamically dan return `null` jika gagal; yang berikutnya dicoba.

### Adapter Interface (semua driver sama):

```js
{
  run(sql, params),      // Execute non-query
  get(sql, params),      // Fetch single row
  all(sql, params),      // Fetch all rows
  exec(sql),             // Execute DDL/multiple statements
  transaction(fn),       // Atomic transaction
  checkpoint(),
  close(),
  raw                    // Underlying database object
}
```

**Khusus `sqljsAdapter`**: Debounce saves ke disk setiap 100ms (karena in-memory database).

### `getAdapterSync()`

Untuk synchronous context (hanya bisa dipanggil SETELAH `getAdapter()` sudah di-init):

```js
export function getAdapterSync() {
  const instance = global._dbAdapter?.instance;
  if (!instance) throw new Error("DB not initialized");
  return instance;
}
```

## Bagaimana Migrations Terpicu?

**Otomatis, tanpa perlu trigger manual.**

Call chain:

```
Repository function (e.g. getSettings())
  → getAdapter()
    → initAdapter()
      → ensureDirs()
      → pickDriver() → better-sqlite3 / node:sqlite / sql.js
      → runMigrationOnce(db)
        → runVersionedMigrations()     // Jalankan migration versi
        → syncSchemaFromTables()       // Sync kolom/index tambahan
        → importLegacyJson()           // Import JSON legacy jika fresh DB
        → backupIfVersionChanged()     // Backup jika app version berubah
```

Trigger pertama: Pertama kali `getAdapter()` dipanggil. Biasanya dari:

- `src/lib/db/index.js:169` — `initDb()` export yang memanggil `getAdapter()`
- `src/shared/services/initializeApp.js:31` — `initDbHooks()` yang memicu init

Setelah pertama kali, adapter di-cache di `global._dbAdapter.instance` — migration tidak pernah dijalankan ulang.

## _meta Table — Version Tracking

**Table**: `_meta` dengan schema sederhana:
- `key TEXT PRIMARY KEY` — nama metadata
- `value TEXT` — nilainya

**Keys yang disimpan:**
- `schemaVersion` — versi migration terakhir yang applied
- `appVersion` — versi aplikasi saat migration terakhir jalan
- (keys lain untuk runtime state)

## Cara Menambahkan Perubahan Skema Baru

### Opsi A: Additive Changes (kolom/index baru) — Tanpa Migration File

**Cukup tambah di `src/lib/db/schema.js`:**

```js
// Contoh: Tambah index pada usageHistory.apiKey
usageHistory: {
  columns: { /* existing columns */ },
  indexes: [
    "CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(timestamp DESC)",
    "CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)",  // NEW
  ],
},
```

**Proses:**
1. Edit `schema.js` — tambah column atau index
2. `syncSchemaFromTables()` akan otomatis detect perbedaan via `PRAGMA table_info`
3. `ALTER TABLE ADD COLUMN` / `CREATE INDEX IF NOT EXISTS` dijalankan saat boot berikutnya
4. Tidak perlu migration file, tidak perlu edit `migrations/index.js`

### Opsi B: Destructive/Transformative Changes — Migration File Required

Untuk perubahan yang tidak additive (drop column, rename table, data migration):

1. **Buat migration file**: `src/lib/db/migrations/002-add-xxx.js`

```js
export default {
  version: 2,
  name: "add-apiKey-index-to-usageHistory",
  up(db) {
    db.transaction(() => {
      db.exec("CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)");
      // Data transformation bisa ditaruh di sini
    });
  },
};
```

2. **Register di `index.js`**:

```js
import m001 from "./001-initial.js";
import m002 from "./002-add-apiKey-index.js";
export const MIGRATIONS = [m001, m002].sort((a, b) => a.version - b.version);
```

3. **Boot berikutnya** — `runVersionedMigrations()` detect `_meta.schemaVersion = 1`, target = 2, jalankan m002

## Rekomendasi untuk Fitur "Usage per User"

### Yang Perlu Diubah

| Perubahan | File | Metode |
|-----------|------|--------|
| Tambah index `idx_uh_apiKey` di `usageHistory` | `src/lib/db/schema.js` | **Opsi A** — tambah di `indexes` array |
| (Opsional) Tambah filter `apiKey` di `getUsageHistory()` | `src/lib/db/repos/usageRepo.js` | Code change, bukan migration |
| (Opsional) Tambah kolom `apiKeyId` (FK ke `apiKeys`) | `src/lib/db/schema.js` + migration | **Opsi B** — perlu migration untuk data consistency |

### Index yang Sudah Ada di usageHistory

```
idx_uh_ts       — timestamp DESC
idx_uh_provider — provider
idx_uh_model    — model
idx_uh_conn     — connectionId
```

**Belum ada index pada `apiKey`** — ini bottleneck untuk query `WHERE apiKey = ?`. Tambah index ini sangat recommended.

### Cara Tambah Index (Praktis)

Edit `src/lib/db/schema.js`, cari `usageHistory` definition, tambah index:

```js
usageHistory: {
  columns: { /* ... */ },
  indexes: [
    "CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(timestamp DESC)",
    "CREATE INDEX IF NOT EXISTS idx_uh_provider ON usageHistory(provider)",
    "CREATE INDEX IF NOT EXISTS idx_uh_model ON usageHistory(model)",
    "CREATE INDEX IF NOT EXISTS idx_uh_conn ON usageHistory(connectionId)",
    "CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)",  // TAMBAH INI
  ],
},
```

Next boot → `syncSchemaFromTables()` akan menjalankan `CREATE INDEX IF NOT EXISTS` otomatis.

## File Terkait

| File | Peran |
|------|-------|
| `src/lib/db/schema.js` | Schema declarative — TABLES object + buildCreateTableSql() |
| `src/lib/db/migrations/index.js` | Migration registry — imports + sort + latestVersion() |
| `src/lib/db/migrations/001-initial.js` | Initial schema — creates all 10 tables + indexes |
| `src/lib/db/migrate.js` | Migration runner — 4 tahap (versioned, sync, legacy import, backup) |
| `src/lib/db/driver.js` | Driver abstraction — getAdapter() lazy singleton + driver selection |
| `src/lib/db/paths.js` | File paths — DATA_FILE, BACKUPS_DIR |
| `src/lib/db/repos/usageRepo.js` | Usage repository — uses getAdapter() untuk queries |
| `src/lib/db/repos/apiKeysRepo.js` | API keys repository — uses getAdapter() |
| `src/lib/db/index.js` | DB barrel — exports initDb() sebagai trigger pertama |
