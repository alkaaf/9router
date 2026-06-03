# Write Patterns Analysis — imp4 Analysis

Source: All INSERT, UPDATE, DELETE operations in src/lib/db/repos/

---

## Write Operation Inventory by Table

### _meta — Lifetime Metadata

| File | Line | SQL | Context | Batch? | Transaction? |
|------|------|-----|---------|--------|-------------|
| usageRepo.js | 307 | INSERT INTO _meta ... ON CONFLICT | totalRequestsLifetime | Per flush | Yes |

**Write frequency:** Every _flushWriteQueue() — ~1s or every 50 requests

---

### settings — Application Settings

| File | Line | SQL | Context | Batch? | Transaction? |
|------|------|-----|---------|--------|-------------|
| settingsRepo.js | 92 | INSERT INTO settings ... ON CONFLICT | updateSettings() | No | Yes (read-merge-write) |

**Write frequency:** Rare — only on settings changes via dashboard

---

### providerConnections — Provider Credentials

| File | Line | SQL | Context | Batch? | Transaction? |
|------|------|-----|---------|--------|-------------|
| connectionsRepo.js | 48-56 | UPSERT | create/update | No | Yes |
| connectionsRepo.js | 87 | UPDATE priority | reorderInTx() | Yes (N in loop) | Yes |
| connectionsRepo.js | 178 | DELETE | delete | No | Yes |

**Write frequency:** Rare — admin CRUD operations

---

### usageHistory — Request Usage Records CRITICAL

| File | Line | SQL | Context | Batch? | Transaction? |
|------|------|-----|---------|--------|-------------|
| usageRepo.js | 293-296 | INSERT batch | _flushWriteQueue() | Yes: up to 50 per flush | Yes |

**Write frequency:** Every 1s OR every 50 queued requests
**Volume:** 1-50 INSERTs per flush
**Pattern:**
```js
const insertStmt = db.prepare(`INSERT INTO usageHistory(...) VALUES(...)`);
for (const e of entries) insertStmt.run(...);
```

---

### usageDaily — Daily Aggregated Usage CRITICAL

| File | Line | SQL | Context | Batch? | Transaction? |
|------|------|-----|---------|--------|-------------|
| usageRepo.js | 298-303 | INSERT OR REPLACE | _flushWriteQueue() | Yes: 1 per day | Yes |

**Write frequency:** Every flush — 1 UPSERT per unique day in batch
**JSON content:** Massive blob with byProvider, byModel, byApiKey, byAccount, byEndpoint

---

### requestDetails — Per-Request Detail Logs

| File | Line | SQL | Context | Batch? | Transaction? |
|------|------|-----|---------|--------|-------------|
| requestDetailsRepo.js | 103-106 | UPSERT batch | flushToDatabase() | Yes: 1-20 per flush | Yes |

**Write frequency:** Every 5s or 20 queued requests

---

## Write Hotspots Analysis

### By Frequency (Most to Least)

| Rank | Table | Writes/Request | Trigger | Kubernetes Impact |
|------|-------|----------------|---------|-------------------|
| 1 | usageHistory | 1-50 | Every API request | CRITICAL — concurrent writes |
| 2 | usageDaily | 1-2 | Every API request | CRITICAL |
| 3 | _meta | 1 | Every flush | Low |
| 4 | requestDetails | 1-20 | Every 5s | Medium |
| 5 | All others | 1 | Admin action | Low |

### Write Volume Per Request Path

```
Request Flow:
  saveRequestUsage() → queues to writeQueue (async)
  Every 1s OR 50 queued:
    _flushWriteQueue():
      - INSERT INTO usageHistory (batch up to 50)
      - INSERT OR REPLACE INTO usageDaily (1 per day)
      - UPSERT INTO _meta (1)
    Total: 2-52 writes per flush
```

---

## Synchronous vs Async Analysis

All adapters are SYNCHRONOUS:

- better-sqlite3: sync db.transaction()
- node:sqlite: sync SAVEPOINT
- bun:sqlite: sync
- sql.js: sync SAVEPOINT

Impact: Every flush blocks event loop for all writes in transaction.

---

## PostgreSQL Migration Concerns

| Issue | Impact | Recommendation |
|-------|--------|----------------|
| Batch INSERT pattern | Code change | Use UNNEST() or multi-row INSERT |
| ON CONFLICT syntax | Compatible | Same syntax works |
| JSON blob as TEXT | Major | Convert to JSONB |
| Multi-row transactions | Compatible | Same BEGIN/COMMIT works |
| Concurrent writes from pods | CRITICAL | PostgreSQL handles this natively |
| Lock contention | CRITICAL | PG has row-level locks, better concurrency |
| Connection per pod | Needed | Use pg-pool with connection limit |

---

*Generated for imp4*
