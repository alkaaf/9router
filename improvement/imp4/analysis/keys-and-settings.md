# imp4 Analysis: API Keys & Settings Storage

**Scope:** K8s/PostgreSQL migration impact on API key management and settings storage.
**Files analyzed:** `src/lib/db/repos/apiKeysRepo.js`, `src/lib/db/repos/settingsRepo.js`, `src/lib/db/schema.js`, `src/lib/db/migrations/001-initial.js`, `src/lib/db/driver.js`, `src/sse/services/auth.js`, `src/dashboardGuard.js`.

---

## 1. API Key Storage

### 1.1 Schema (`src/lib/db/schema.js:74-84`)

```js
apiKeys: {
  columns: {
    id: "TEXT PRIMARY KEY",          // UUID v4 string
    key: "TEXT UNIQUE NOT NULL",     // full API key string (not hashed!)
    name: "TEXT",                    // human label
    machineId: "TEXT",               // device/machine id bound to key
    isActive: "INTEGER DEFAULT 1",   // SQLite boolean (0/1)
    createdAt: "TEXT NOT NULL",      // ISO timestamp
  },
  indexes: ["CREATE INDEX IF NOT EXISTS idx_ak_key ON apiKeys(key)"],
}
```

**Key observations:**
- `id` is a UUID v4 generated via `uuid` package (`apiKeysRepo.js:34`).
- `key` is stored in **plaintext** — the full token the client sends over the wire. The DB row is the single source of truth and there is no second copy anywhere (no `keyHash`).
- `UNIQUE` constraint on `key` plus a dedicated B-tree index `idx_ak_key` for hot-path lookup.
- `isActive` is SQLite's `INTEGER` (0/1) — there is no PostgreSQL-style `BOOLEAN`. Any `BOOLEAN` mapping must convert.
- `createdAt` is ISO-8601 text, not a `TIMESTAMP` column.

### 1.2 CRUD Surface (`src/lib/db/repos/apiKeysRepo.js`)

| Function | SQL | Notes |
|---|---|---|
| `getApiKeys()` (L16) | `SELECT * FROM apiKeys ORDER BY createdAt ASC` | Dashboard list. Low frequency. |
| `getApiKeyById(id)` (L22) | `SELECT * FROM apiKeys WHERE id = ?` | UUID lookup. Used by per-key usage endpoints. |
| `createApiKey(name, machineId)` (L28) | `INSERT INTO apiKeys(id, key, name, machineId, isActive, createdAt) VALUES(?,?,?,?,?,?)` | Key is generated via `generateApiKeyWithMachine(machineId)` from `@/shared/utils/apiKey`. |
| `updateApiKey(id, data)` (L48) | Read-modify-write inside a `db.transaction(() => { ... })` block. | The merge keeps `key` from the existing row when caller didn't pass one — important: no key rotation path. |
| `deleteApiKey(id)` (L64) | `DELETE FROM apiKeys WHERE id = ?` | Returns `(res?.changes ?? 0) > 0`. |
| `validateApiKey(key)` (L70) | `SELECT isActive FROM apiKeys WHERE key = ?` | **Hot path.** Returns boolean only (not the row). |

**`validateApiKey` is the only "hot" function.** Every other CRUD op is dashboard-bound (low QPS).

### 1.3 Authentication Flow

Two call sites use `validateApiKey`:

1. **`src/dashboardGuard.js:115`** — `hasValidApiKey(request)` — gates public LLM API access (`canAccessPublicLlmApi`).
2. **`src/sse/services/auth.js:307`** — `isValidApiKey(apiKey)` — SSE handlers (e.g., `/v1/chat/completions` stream).

Key extraction (`dashboardGuard.js:106-110`):
- `Authorization: Bearer <key>` header
- `x-api-key: <key>` header

Both authenticate the *full key string*, then check `isActive`. There is no machineId binding enforced server-side at validation time — `machineId` is recorded on create but `validateApiKey` ignores it. The same key works from any client.

`getApiKeyById` is used by usage/per-key endpoints (`src/app/api/usage/per-key/[keyId]/{,history,chart}/route.js`) — these are dashboard reads, not auth hot path.

---

## 2. Settings Storage

### 2.1 Schema (`src/lib/db/schema.js:24-29`)

```js
settings: {
  columns: {
    id: "INTEGER PRIMARY KEY CHECK (id = 1)",  // single-row pattern
    data: "TEXT NOT NULL",                      // JSON blob
  },
}
```

**Single-row pattern with a CHECK constraint** to enforce exactly one row. The whole configuration object is serialized as JSON into one `TEXT` column. There are no per-key indexes — every read is `WHERE id = 1`.

### 2.2 Repository (`src/lib/db/repos/settingsRepo.js`)

**Defaults (L7-40):** 33 keys total, including auth (`requireLogin`, `authMode`, OIDC settings), routing (`providerStrategies`, `comboStrategy`), observability, and proxy config.

**In-process cache (L42-43, L70-81):**
- TTL-based cache: `SETTINGS_CACHE_TTL_MS = 5000` (5 s).
- Stored on `global._settingsCache` to survive Next.js dev hot-reload.
- `getSettings()` returns cached value if age < 5 s; otherwise reads from DB and merges with defaults.

**Hot-path read (L70-81):**
```js
const raw = await readRaw();        // SELECT data FROM settings WHERE id = 1
const merged = mergeWithDefaults(raw);
settingsCache.data = merged;
settingsCache.ts = Date.now();
return merged;
```

**Write (L84-99):** `updateSettings(updates)` does a read-merge-write inside `db.transaction()` using `INSERT ... ON CONFLICT(id) DO UPDATE SET data = excluded.data`. After a successful write, it **eagerly invalidates the in-process cache** with the new merged value. This is critical: without eager invalidation, other replicas in K8s (each with their own in-process cache) would serve stale settings for up to 5 s.

**Helpers (L101-114):** `isCloudEnabled()` and `getCloudUrl()` are thin wrappers around `getSettings()`.

### 2.3 Read Frequency

Per the imp3 analysis referenced in the task, `getSettings()` is called **3× per request** (or more — `getCloudUrl()` is also called by some handlers). The 5 s in-process cache makes the DB hit rate ~1 read per 5 s per pod, but the *function call* rate is high.

With multiple K8s pods, each pod has its own 5 s cache, so total DB read QPS is `pods / 5 s` — typically a handful of reads per second even at moderate traffic.

---

## 3. Security Considerations for K8s/PostgreSQL

### 3.1 API Keys (Plaintext)

| Risk | Impact | Mitigation |
|---|---|---|
| **Plaintext key in DB** | DB read access = full key compromise. | At-rest encryption (PostgreSQL TDE / disk-level LUKS / cloud provider encryption). Restrict `SELECT` on the `apiKeys` table to the app role only — no replicas, no analyst role. |
| **Key in network traffic** | MITM exposure. | TLS to PostgreSQL (required, not optional — even intra-cluster). No `sslmode=disable`. |
| **Key in `pg_dump` / logical backups** | Backup file = key leak. | Encrypt backups; restrict who can pull them. Consider column-level encryption (`pgcrypto`) for `key` so even DBA with full DB access can't read raw keys. |
| **Logs** | Some logs print request headers / auth context. | Audit log paths for any `console.log(apiKey)` or `console.log(req.headers)`. |
| **No hashing** | Cannot add hashing without breaking every existing key. | Hashing is a hard break — every client must rotate. **Recommendation:** do not introduce hashing in imp4; instead rely on `pgcrypto` column encryption or stay plaintext + tight access control. Defer hashing to a separate "v2" key format. |
| **No `machineId` enforcement at validation** | Stolen key works from anywhere. | Out of scope for storage layer, but flag for follow-up. |
| **Connection string in env / K8s Secret** | Creds leak via etcd / kubectl describe. | Use a sealed/external secret store (Sealed Secrets, External Secrets Operator → AWS Secrets Manager / Vault). Never put `DATABASE_URL` in `ConfigMap`. |

### 3.2 Settings (JSON Blob)

- The settings JSON contains OIDC client secret, outbound proxy URL, etc. (`oidcClientSecret`, `outboundProxyUrl`). Same encryption-at-rest requirement applies.
- The 5 s in-process cache is **per-pod** and not shared. In multi-replica K8s, settings changes propagate in 0–5 s per pod (eager invalidation helps the pod that wrote; other pods still wait up to 5 s). For faster propagation, see §4.2.

### 3.3 Shared-PostgreSQL-Specific Risks

- **Connection pooling at app tier becomes load-bearing.** SQLite uses a single-process embedded driver. PostgreSQL needs a pool (e.g., `pg.Pool` with `pgcat`/`pgbouncer`/`odyssey` in front) to keep the connection count bounded across many pod replicas. Without it, 50 pods × 10 connections = 500 backend connections → Postgres max_connections ceiling.
- **Multi-pod writes to settings are serialized by `db.transaction()`** today. With PostgreSQL, the equivalent is `SELECT ... FOR UPDATE` or an `UPDATE ... WHERE id = 1 RETURNING data` to avoid lost updates. The current `INSERT ... ON CONFLICT DO UPDATE` is fine for last-writer-wins, but the in-process optimistic merge in JS is racy across pods — two pods each read the same baseline, both merge, both write → one's update is lost. This is a pre-existing bug that worsens with horizontal scale.
- **No row-level locking means `validateApiKey` is read-only and safe** — no write contention on the hot path.
- **Backups now go to S3/object storage, not a local file.** Must be encrypted; access controlled via IAM, not POSIX perms.

---

## 4. PostgreSQL Migration Notes

### 4.1 API Keys

- **Schema mapping** (mostly trivial):
  - `id TEXT PRIMARY KEY` → `id UUID PRIMARY KEY` (cleaner; or keep `TEXT`).
  - `key TEXT UNIQUE NOT NULL` → `key TEXT UNIQUE NOT NULL` (or `BYTEA` if using `pgcrypto`).
  - `isActive INTEGER DEFAULT 1` → `isActive BOOLEAN DEFAULT TRUE` (or keep `SMALLINT`).
  - `createdAt TEXT` → `createdAt TIMESTAMPTZ` (cleaner; or keep text for zero-touch migration).
- **Index:** `idx_ak_key` is already a B-tree on a unique column — `CREATE UNIQUE INDEX` is implicit, so the explicit index can be dropped to save a redundant lookup. Either way, the lookup remains O(log n).
- **Connection pool sizing:** `validateApiKey` runs on every authenticated request. At e.g. 1k RPS with 10 ms p50 query time, you need ≥ 10 connections. Pool of 20–30 per pod is safe.
- **Optional: caching layer.** The 5 s in-process cache used for settings could be extended to API key lookups (with explicit invalidation on `updateApiKey` / `deleteApiKey`). A Redis cache shared across pods would prevent each pod from hitting Postgres on every request. Trade-off: one more hop, one more failure mode.
- **Hot-path optimization:** the current `validateApiKey` returns a `boolean`. If it ever needs to return the row (for `machineId` enforcement, audit log, per-key rate limit), prefer `SELECT id, isActive, machineId FROM apiKeys WHERE key = $1` — still uses the unique index.
- **Key hashing is a v2 problem.** It changes the key contract, breaks rotation, and the current code returns the full key on creation (caller must save it). Defer.

### 4.2 Settings

- **Connection pooling** for the per-request reads: each pod should hold a small pool (5–10 connections). The 5 s in-process cache keeps actual DB reads low.
- **Cross-pod cache invalidation:** the in-process `global._settingsCache` is per-pod. Options:
  1. Accept up to 5 s staleness across pods (current behavior, simplest).
  2. Use PostgreSQL `LISTEN/NOTIFY` from the writer pod to fan out invalidations to all pods.
  3. Move cache to Redis with a 5 s TTL, deleted by the writer on `updateSettings`.
  4. Cache in `etcd` / `Consul` (overkill for a single key).
  Recommendation: keep in-process cache for read perf, add LISTEN/NOTIFY or Redis pub/sub for invalidation.
- **JSON column:** PostgreSQL has native `JSONB`, which gives you GIN-indexable access and avoids the JS-side `parseJson` / `stringifyJson` round-trip. Migration of existing row: `UPDATE settings SET data = data::jsonb WHERE id = 1`. Code change: `parseJson(row.data, {})` → `row.data ?? {}` (Postgres returns parsed object directly).
- **Concurrency on writes:** the current `db.transaction` + JS-side merge is racy across pods. Replace with one of:
  - `UPDATE settings SET data = data || $1::jsonb WHERE id = 1 RETURNING data` (atomic merge in SQL).
  - `SELECT ... FOR UPDATE` then merge + write inside the transaction.
  - Optimistic concurrency: add a `version` column, retry on conflict.
- **Read-merge with defaults** can stay in JS or move to SQL with `COALESCE` / a view. JS is fine — the defaults table is in code anyway.

### 4.3 Connection String

- `DATABASE_URL` from env, fed in via K8s `Secret`. No special driver selection logic — Postgres is the only driver.
- TLS: `sslmode=require` minimum; `verify-full` with a CA bundle for production.
- Auth: prefer IAM / SCRAM-SHA-256 over password. Rotate quarterly.

---

## 5. Summary of Hot Paths

| Operation | Frequency | Today (SQLite) | Tomorrow (Postgres) |
|---|---|---|---|
| `validateApiKey` | Every authenticated request | Local file, ~µs | Network round-trip, ~ms; needs connection pool, optionally Redis cache |
| `getSettings` (call) | 3×+ per request | In-process cache after first call | Same — cache is in JS, not DB |
| `getSettings` (DB read) | ~1 per 5 s per pod | Local file | Network; consider LISTEN/NOTIFY for cross-pod invalidation |
| `getApiKeyById` | Dashboard, low | Local file | Network; no perf concern |
| `createApiKey` / `updateApiKey` / `deleteApiKey` | Dashboard, low | Local file | Network; consider row-locking on `updateApiKey` merge |

---

## 6. Recommendations (ranked)

1. **TLS to Postgres, encrypted at rest, secrets in K8s Secret / external store.** Non-negotiable.
2. **Connection pool at app tier** (or a sidecar pgbouncer). Without this, 10 pods will exhaust Postgres `max_connections`.
3. **Replace `INTEGER` with `BOOLEAN`** in `apiKeys.isActive` for Postgres-native semantics. Trivial code change (`rowToKey` already handles both).
4. **Migrate `settings.data` to `JSONB`** — drops the JS parse/stringify overhead and enables atomic SQL-side merge.
5. **Atomic write for `updateSettings`** — `UPDATE ... SET data = data || $1::jsonb RETURNING data` to fix the cross-pod lost-update race.
6. **Add LISTEN/NOTIFY or Redis pub/sub for cross-pod cache invalidation** — the eager in-process invalidation is local-only.
7. **Defer key hashing** to a v2 key format — breaking change for every existing key.
8. **Restrict `SELECT` on `apiKeys`** to the app role only — DBA and read-replica roles should not see raw keys. Combined with `pgcrypto` column encryption if defense-in-depth is needed.
9. **Audit logs for any `console.log` of request headers / auth context** before going live.
10. **Decide on machineId enforcement at validation** — currently a no-op (`validateApiKey` doesn't read it). Out of scope for the storage migration but worth flagging.
