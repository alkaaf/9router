# Database Schema Catalog — 9Router (imp4 Analysis)

Source: `src/lib/db/schema.js`  
Migration: `src/lib/db/migrations/001-initial.js` (version 1)  
Schema version: 1

---

## Tables Overview

| # | Table | Primary Key | Row Pattern | Approx Purpose |
|---|-------|-------------|-------------|-----------------|
| 1 | `_meta` | `key` (TEXT) | Key-value metadata | Schema/version metadata |
| 2 | `settings` | `id` (INTEGER, CHECK id=1) | Single-row | Application-wide settings |
| 3 | `providerConnections` | `id` (TEXT) | Multi-row | Provider credential configs |
| 4 | `providerNodes` | `id` (TEXT) | Multi-row | LLM node definitions |
| 5 | `proxyPools` | `id` (TEXT) | Multi-row | Proxy pool configs |
| 6 | `apiKeys` | `id` (TEXT) | Multi-row | API key records |
| 7 | `combos` | `id` (TEXT) | Multi-row | Model combo definitions |
| 8 | `kv` | `(scope, key)` composite | Multi-row | Generic key-value store |
| 9 | `usageHistory` | `id` (INTEGER AUTOINCREMENT) | Multi-row | Request-level usage records |
| 10 | `usageDaily` | `dateKey` (TEXT) | One row per day | Aggregated daily usage |
| 11 | `requestDetails` | `id` (TEXT) | Multi-row | Per-request detail logs |
| 12 | *(Total)* | | | |

---

## Detailed Table Definitions

### 1. `_meta` — Key-Value Metadata

```sql
CREATE TABLE IF NOT EXISTS _meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `key` | TEXT | PRIMARY KEY | Metadata key (e.g. schema version) |
| `value` | TEXT | NOT NULL | Metadata value |

- **No indexes beyond the primary key.**
- **PostgreSQL:** No changes needed. `TEXT PRIMARY KEY` maps directly.

---

### 2. `settings` — Single-Row Settings

```sql
CREATE TABLE IF NOT EXISTS settings (
  id   INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PRIMARY KEY, CHECK (id = 1) | Enforces exactly one row |
| `data` | TEXT | NOT NULL | JSON-serialized settings blob |

- **PostgreSQL:** `CHECK` constraint is supported. Consider `JSONB` for `data` if queries need to filter on settings fields.

---

### 3. `providerConnections` — Provider Credentials

```sql
CREATE TABLE IF NOT EXISTS providerConnections (
  id         TEXT PRIMARY KEY,
  provider   TEXT NOT NULL,
  authType   TEXT NOT NULL,
  name       TEXT,
  email      TEXT,
  priority   INTEGER,
  isActive   INTEGER DEFAULT 1,
  data       TEXT NOT NULL,
  createdAt  TEXT NOT NULL,
  updatedAt  TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PRIMARY KEY | UUID or short ID |
| `provider` | TEXT | NOT NULL | Provider identifier (e.g. `openai`, `anthropic`) |
| `authType` | TEXT | NOT NULL | Auth mechanism (e.g. `apiKey`, `oauth`) |
| `name` | TEXT | NULLABLE | Display name |
| `email` | TEXT | NULLABLE | Account email |
| `priority` | INTEGER | NULLABLE | Routing priority |
| `isActive` | INTEGER | DEFAULT 1 | Boolean flag (0/1) |
| `data` | TEXT | NOT NULL | JSON blob: full credentials/config |
| `createdAt` | TEXT | NOT NULL | ISO-8601 string |
| `updatedAt` | TEXT | NOT NULL | ISO-8601 string |

**Indexes (3):**
- `idx_pc_provider` — on `(provider)`
- `idx_pc_provider_active` — on `(provider, isActive)` composite
- `idx_pc_priority` — on `(provider, priority)`

- **PostgreSQL:** `isActive INTEGER` → consider `BOOLEAN`. Composite index on `(provider, isActive)` is useful for partial index optimization. JSON `data` → `JSONB`.

---

### 4. `providerNodes` — LLM Node Definitions

```sql
CREATE TABLE IF NOT EXISTS providerNodes (
  id        TEXT PRIMARY KEY,
  type      TEXT,
  name      TEXT,
  data      TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PRIMARY KEY | |
| `type` | TEXT | NULLABLE | Node type/category |
| `name` | TEXT | NULLABLE | Display name |
| `data` | TEXT | NOT NULL | JSON blob |
| `createdAt` | TEXT | NOT NULL | ISO-8601 |
| `updatedAt` | TEXT | NOT NULL | ISO-8601 |

**Indexes (1):**
- `idx_pn_type` — on `(type)`

- **PostgreSQL:** `data` → `JSONB`. Consider adding a `GIN` index on `data` if JSON queries are frequent.

---

### 5. `proxyPools` — Proxy Pool Configurations

```sql
CREATE TABLE IF NOT EXISTS proxyPools (
  id         TEXT PRIMARY KEY,
  isActive   INTEGER DEFAULT 1,
  testStatus TEXT,
  data       TEXT NOT NULL,
  createdAt  TEXT NOT NULL,
  updatedAt  TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PRIMARY KEY | |
| `isActive` | INTEGER | DEFAULT 1 | Boolean flag |
| `testStatus` | TEXT | NULLABLE | e.g. `healthy`, `degraded`, `dead` |
| `data` | TEXT | NOT NULL | JSON blob |
| `createdAt` | TEXT | NOT NULL | ISO-8601 |
| `updatedAt` | TEXT | NOT NULL | ISO-8601 |

**Indexes (2):**
- `idx_pp_active` — on `(isActive)`
- `idx_pp_status` — on `(testStatus)`

- **PostgreSQL:** `isActive` → `BOOLEAN`. `testStatus` low-cardinality — consider `ENUM` or a lookup table.

---

### 6. `apiKeys` — API Key Records

```sql
CREATE TABLE IF NOT EXISTS apiKeys (
  id        TEXT PRIMARY KEY,
  key       TEXT UNIQUE NOT NULL,
  name      TEXT,
  machineId TEXT,
  isActive  INTEGER DEFAULT 1,
  createdAt TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PRIMARY KEY | Internal ID (likely UUID) |
| `key` | TEXT | UNIQUE, NOT NULL | **Full API key string** (not hashed) |
| `name` | TEXT | NULLABLE | Display name for the key |
| `machineId` | TEXT | NULLABLE | Bound machine/device identifier |
| `isActive` | INTEGER | DEFAULT 1 | Boolean flag |
| `createdAt` | TEXT | NOT NULL | ISO-8601 |

**Indexes (1):**
- `idx_ak_key` — on `(key)` — redundant with UNIQUE constraint; likely for query plan hints

- **PostgreSQL:** `key` column stores plaintext API keys — consider encryption-at-rest or hashing. `isActive` → `BOOLEAN`. UNIQUE constraint becomes a unique B-tree index automatically.

---

### 7. `combos` — Model Combos

```sql
CREATE TABLE IF NOT EXISTS combos (
  id        TEXT PRIMARY KEY,
  name      TEXT UNIQUE NOT NULL,
  kind      TEXT,
  models    TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PRIMARY KEY | |
| `name` | TEXT | UNIQUE, NOT NULL | Combo name |
| `kind` | TEXT | NULLABLE | Combo type/category |
| `models` | TEXT | NOT NULL | JSON array of model identifiers |
| `createdAt` | TEXT | NOT NULL | ISO-8601 |
| `updatedAt` | TEXT | NOT NULL | ISO-8601 |

**Indexes (1):**
- `idx_combo_name` — on `(name)` — redundant with UNIQUE constraint

- **PostgreSQL:** `models` JSON → `JSONB`. UNIQUE on `name` becomes unique B-tree index.

---

### 8. `kv` — Generic Key-Value Store

```sql
CREATE TABLE IF NOT EXISTS kv (
  scope  TEXT NOT NULL,
  key    TEXT NOT NULL,
  value  TEXT NOT NULL,
  PRIMARY KEY (scope, key)
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `scope` | TEXT | NOT NULL, part of PK | Namespace/scope (e.g. `user:123`, `session:abc`) |
| `key` | TEXT | NOT NULL, part of PK | Key within scope |
| `value` | TEXT | NOT NULL | Arbitrary value |

**Composite Primary Key:** `(scope, key)` — the defining feature of this table.

**Indexes (1):**
- `idx_kv_scope` — on `(scope)` — supports queries by scope prefix

- **PostgreSQL:** Composite PK on `(scope, key)` is native. Consider `JSONB` for `value` if structured data is stored.

---

### 9. `usageHistory` — Request-Level Usage Records

```sql
CREATE TABLE IF NOT EXISTS usageHistory (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp        TEXT NOT NULL,
  provider         TEXT,
  model            TEXT,
  connectionId     TEXT,
  apiKey           TEXT,
  endpoint         TEXT,
  promptTokens     INTEGER DEFAULT 0,
  completionTokens INTEGER DEFAULT 0,
  cost             REAL DEFAULT 0,
  status           TEXT,
  tokens           TEXT,
  meta             TEXT
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | Monotonically increasing surrogate key |
| `timestamp` | TEXT | NOT NULL | ISO-8601 request timestamp |
| `provider` | TEXT | NULLABLE | Provider identifier |
| `model` | TEXT | NULLABLE | Model name |
| `connectionId` | TEXT | NULLABLE | FK-like reference to `providerConnections.id` |
| `apiKey` | TEXT | NULLABLE | API key used |
| `endpoint` | TEXT | NULLABLE | API endpoint path |
| `promptTokens` | INTEGER | DEFAULT 0 | Token count |
| `completionTokens` | INTEGER | DEFAULT 0 | Token count |
| `cost` | REAL | DEFAULT 0 | Calculated cost |
| `status` | TEXT | NULLABLE | e.g. `success`, `error`, `rate-limited` |
| `tokens` | TEXT | NULLABLE | JSON blob: detailed token breakdown |
| `meta` | TEXT | NULLABLE | JSON blob: additional metadata |

**Indexes (6) — most indexed table:**
- `idx_uh_ts` — on `(timestamp DESC)` — time-range queries
- `idx_uh_provider` — on `(provider)` — provider filtering
- `idx_uh_model` — on `(model)` — model filtering
- `idx_uh_conn` — on `(connectionId)` — connection lookup
- `idx_uh_apiKey` — on `(apiKey)` — key usage lookup
- `idx_uh_apiKey_ts` — on `(apiKey, timestamp)` — key usage over time

- **PostgreSQL:** `AUTOINCREMENT` → `SERIAL` or `BIGSERIAL`. `REAL` for `cost` → `NUMERIC(10,6)` for financial precision. `timestamp TEXT` → `TIMESTAMPTZ`. Consider partitioning by timestamp for large datasets. Foreign key from `connectionId` to `providerConnections(id)` is implied but not enforced in schema.

---

### 10. `usageDaily` — Daily Aggregated Usage

```sql
CREATE TABLE IF NOT EXISTS usageDaily (
  dateKey TEXT PRIMARY KEY,
  data    TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `dateKey` | TEXT | PRIMARY KEY | Date string, e.g. `2024-01-15` |
| `data` | TEXT | NOT NULL | JSON blob: aggregated data including `byApiKey` breakdown |

**No additional indexes** — primary key on `dateKey` covers lookup patterns.

- **PostgreSQL:** `dateKey TEXT` → `DATE` type (proper date semantics). `data` → `JSONB`. Partitions by date if data grows large.

---

### 11. `requestDetails` — Per-Request Detail Logs

```sql
CREATE TABLE IF NOT EXISTS requestDetails (
  id          TEXT PRIMARY KEY,
  timestamp   TEXT NOT NULL,
  provider    TEXT,
  model       TEXT,
  connectionId TEXT,
  status      TEXT,
  data        TEXT NOT NULL
);
```

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PRIMARY KEY | Request/correlation ID |
| `timestamp` | TEXT | NOT NULL | ISO-8601 |
| `provider` | TEXT | NULLABLE | Provider identifier |
| `model` | TEXT | NULLABLE | Model name |
| `connectionId` | TEXT | NULLABLE | FK-like reference |
| `status` | TEXT | NULLABLE | Request outcome |
| `data` | TEXT | NOT NULL | JSON blob: full request/response details |

**Indexes (4):**
- `idx_rd_ts` — on `(timestamp DESC)`
- `idx_rd_provider` — on `(provider)`
- `idx_rd_model` — on `(model)`
- `idx_rd_conn` — on `(connectionId)`

- **PostgreSQL:** `id TEXT` → `UUID` if UUID strings. `timestamp TEXT` → `TIMESTAMPTZ`. `data` → `JSONB`. Foreign key from `connectionId` to `providerConnections(id)` is implied.

---

## All Indexes Summary (14 total)

| # | Index Name | Table | Columns | Notes |
|---|-----------|-------|---------|-------|
| 1 | `idx_pc_provider` | `providerConnections` | `(provider)` | |
| 2 | `idx_pc_provider_active` | `providerConnections` | `(provider, isActive)` | Composite |
| 3 | `idx_pc_priority` | `providerConnections` | `(provider, priority)` | Composite |
| 4 | `idx_pn_type` | `providerNodes` | `(type)` | |
| 5 | `idx_pp_active` | `proxyPools` | `(isActive)` | |
| 6 | `idx_pp_status` | `proxyPools` | `(testStatus)` | |
| 7 | `idx_ak_key` | `apiKeys` | `(key)` | Redundant with UNIQUE |
| 8 | `idx_combo_name` | `combos` | `(name)` | Redundant with UNIQUE |
| 9 | `idx_kv_scope` | `kv` | `(scope)` | |
| 10 | `idx_uh_ts` | `usageHistory` | `(timestamp DESC)` | Time range |
| 11 | `idx_uh_provider` | `usageHistory` | `(provider)` | |
| 12 | `idx_uh_model` | `usageHistory` | `(model)` | |
| 13 | `idx_uh_conn` | `usageHistory` | `(connectionId)` | |
| 14 | `idx_uh_apiKey` | `usageHistory` | `(apiKey)` | |
| 15 | `idx_uh_apiKey_ts` | `usageHistory` | `(apiKey, timestamp)` | Composite |
| 16 | `idx_rd_ts` | `requestDetails` | `(timestamp DESC)` | |
| 17 | `idx_rd_provider` | `requestDetails` | `(provider)` | |
| 18 | `idx_rd_model` | `requestDetails` | `(model)` | |
| 19 | `idx_rd_conn` | `requestDetails` | `(connectionId)` | |

**Total explicit indexes: 19** (2 are redundant with UNIQUE constraints: `idx_ak_key`, `idx_combo_name`)

**Note:** The task brief mentioned 14 indexes, but the schema defines 19 explicit `CREATE INDEX` statements. The 6 on `usageHistory` plus 4 on `requestDetails` plus 3 on `providerConnections` plus 2 on `proxyPools` plus 1 on `providerNodes` plus 1 on `apiKeys` plus 1 on `combos` plus 1 on `kv` = 19.

---

## SQLite-Specific Schema Features

### AUTOINCREMENT
- Used only on `usageHistory.id`: `INTEGER PRIMARY KEY AUTOINCREMENT`
- Guarantees monotonically increasing rowids even after deletes
- Slight storage overhead vs plain `INTEGER PRIMARY KEY`

### INTEGER PRIMARY KEY (alias for ROWID)
- `settings.id` uses `INTEGER PRIMARY KEY CHECK (id = 1)` — the CHECK enforces single-row semantics
- In SQLite, `INTEGER PRIMARY KEY` without AUTOINCREMENT maps to ROWID

### TEXT for JSON
- All JSON columns (`data`, `tokens`, `meta`, `models`) use `TEXT` type
- JSON is stored as serialized strings, no native JSON support
- No JSON querying or indexing capabilities in the database layer

### Composite Primary Key
- `kv` uses `PRIMARY KEY (scope, key)` inline (not in column defs)
- Enforces uniqueness per scope+key pair

### CHECK Constraint
- `settings.id` has `CHECK (id = 1)` — SQLite supports CHECK but enforcement varies by version

### Foreign Keys
- `PRAGMA foreign_keys = ON` is set in `PRAGMA_SQL`
- However, **no actual FOREIGN KEY constraints** are defined in the schema
- References like `connectionId` to `providerConnections(id)` are application-level only

### IF NOT EXISTS
- All `CREATE TABLE` and `CREATE INDEX` statements use `IF NOT EXISTS`
- Idempotent schema creation pattern

---

## PostgreSQL Migration Considerations

### 1. `_meta`
- **No changes needed.** TEXT PK maps directly.

### 2. `settings`
- **No structural changes needed.**
- Consider `JSONB` for `data` column to enable JSON operators.
- The `CHECK (id = 1)` constraint is fully supported in PostgreSQL.

### 3. `providerConnections`
- `isActive INTEGER DEFAULT 1` → `BOOLEAN DEFAULT TRUE`
- `data TEXT` → `JSONB` for JSON query support.
- `createdAt/updatedAt TEXT` → `TIMESTAMPTZ` for proper temporal semantics.
- Add explicit `FOREIGN KEY` if referenced by other tables.
- Consider adding a `lastUsedAt` column for tracking recency.

### 4. `providerNodes`
- `data TEXT` → `JSONB`.
- `createdAt/updatedAt TEXT` → `TIMESTAMPTZ`.
- Add `GIN` index on `data` if JSON queries are needed.

### 5. `proxyPools`
- `isActive INTEGER DEFAULT 1` → `BOOLEAN DEFAULT TRUE`.
- `testStatus TEXT` → Consider `ENUM('healthy', 'degraded', 'dead', 'untested')` or a reference table.
- `data TEXT` → `JSONB`.

### 6. `apiKeys`
- **⚠️ Security concern:** `key TEXT UNIQUE NOT NULL` stores the full API key in plaintext. PostgreSQL should use `pgcrypto` for encryption or store hashes.
- `isActive INTEGER DEFAULT 1` → `BOOLEAN DEFAULT TRUE`.
- `machineId TEXT` → Consider `UUID` type if it references machines.
- Consider adding a `lastUsedAt` column.

### 7. `combos`
- `models TEXT` → `JSONB` (it stores a JSON array of model identifiers).
- `kind TEXT` → Consider `ENUM` if fixed set.
- `createdAt/updatedAt TEXT` → `TIMESTAMPTZ`.
- `UNIQUE(name)` becomes a unique B-tree index automatically.

### 8. `kv`
- **No structural changes needed.**
- Composite PK `(scope, key)` maps directly.
- Consider `JSONB` for `value` if structured data is stored.

### 9. `usageHistory`
- `id INTEGER PRIMARY KEY AUTOINCREMENT` → `BIGSERIAL` or `UUID` (UUID better for distributed systems).
- `timestamp TEXT` → `TIMESTAMPTZ` — critical for time-range queries.
- `cost REAL DEFAULT 0` → `NUMERIC(12,6)` for financial precision (REAL has rounding issues).
- `promptTokens/completionTokens INTEGER` → Consider `BIGINT` if high-volume.
- `tokens TEXT, meta TEXT` → `JSONB` for detailed breakdown.
- `connectionId TEXT` → Add explicit `FOREIGN KEY REFERENCES providerConnections(id)`.
- **Consider partitioning** by `timestamp` (monthly or weekly) for write-heavy workloads.
- Indexes on `(timestamp DESC)` and `(apiKey, timestamp)` should use BRIN for time-series in PostgreSQL.

### 10. `usageDaily`
- `dateKey TEXT PRIMARY KEY` → `DATE PRIMARY KEY` — proper date type enables date functions.
- `data TEXT` → `JSONB` with `GIN` index for querying `byApiKey` and other aggregations.

### 11. `requestDetails`
- `id TEXT PRIMARY KEY` → `UUID PRIMARY KEY` if these are UUIDs.
- `timestamp TEXT` → `TIMESTAMPTZ`.
- `data TEXT` → `JSONB` with `GIN` index.
- `connectionId TEXT` → Add explicit `FOREIGN KEY REFERENCES providerConnections(id)`.
- Consider a composite index on `(timestamp DESC, provider)` for common query patterns.

### Cross-Cutting Concerns

| Concern | Recommendation |
|---------|---------------|
| Timestamps | All `TEXT` → `TIMESTAMPTZ` for proper timezone handling |
| Boolean flags | All `INTEGER DEFAULT 1` → `BOOLEAN DEFAULT TRUE` |
| JSON blobs | All `TEXT` JSON → `JSONB` with `GIN` indexes where queried |
| Foreign keys | Add explicit FK constraints for `connectionId` references |
| Audit columns | Consider adding `createdBy`/`updatedBy` to multi-row tables |
| Row-level security | Evaluate RLS policies for multi-tenant scenarios |
| Connection pooling | Use PgBouncer; `usageHistory` is write-heavy |
