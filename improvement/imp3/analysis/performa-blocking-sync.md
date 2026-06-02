# Analisis Bottleneck Performa 9router — Paralel Agent

**Tanggal:** 2026-06-01  
**Masalah:** 9router melambat dan terasa "blocking" ketika banyak agent menjalankan task secara paralel.

---

## Kesimpulan Utama

**Ya, penyebab utamanya adalah synchronous DB writes.** Setiap request yang selesai menulis ke SQLite secara sinkron memblokir event loop Node.js, sehingga request lain terpantau menunggu. Di bawah paralel load, ini menjadi bottleneck terbesar.

---

## Bukti dari Codebase

### 1. Synchronous Transaction di `saveRequestUsage()` — Bottleneck P0

**File:** `src/lib/db/repos/usageRepo.js:243-287`

```js
export async function saveRequestUsage(entry) {
  const db = await getAdapter();
  // ...
  db.transaction(() => {
    // 3 writes dalam SATU transaksi sinkron:
    db.run(INSERT INTO usageHistory ...);          // Write #1
    db.run(UPSERT INTO usageDaily ...);            // Write #2
    db.run(UPSERT INTO _meta ...);                 // Write #3
  });
}
```

**Yang terjadi:**
- Ketika `better-sqlite3` aktif (Node.js default), `db.run()` adalah **synchronous** — memblokir event loop sampai write selesai (~5-20ms per write).
- WAL mode mengizinkan concurrent reads, tapi **writes tetap serial**. Satu write sedang berlangsung = semua request lain menunggu.
- `busy_timeout = 5000` (schema.js:11) berarti jika ada lock, request bisa menunggu **hingga 5 detik**.

**Dampak di bawah paralel load (100 concurrent agents):**
```
Agent A: tulis DB (blocking 15ms) → selesai
Agent B: tulis DB (blocking 15ms) → selesai
Agent C: tulis DB (blocking 15ms) → selesai
...
Total: 100 × 15ms = 1.5 detik sequential blocking
```

### 2. `getSettings()` Dipanggil 3x Per Request — Membebani Lebih

**File:** `src/sse/handlers/chat.js`

| Baris | Panggilan | Konteks |
|-------|-----------|---------|
| 69 | `const settings = await getSettings()` | `handleChat()` — validasi API key |
| 127 | `const chatSettings = await getSettings()` | `handleSingleModelChat()` — combo check |
| 201 | `const chatSettings = await getSettings()` | `handleSingleModelChat()` — sebelum routing |

Setiap panggilan ke `getSettings()` melakukan `db.get("SELECT data FROM settings WHERE id = 1")` (`src/lib/db/repos/settingsRepo.js:42`). Ini synchronous read yang menambah beban event loop.

### 3. `checkRateLimit()` Tidak Pernah Dipanggil — Dead Code

**File:** `src/lib/rateLimit/checker.js:9`

- Fungsi `checkRateLimit(apiKey)` **tidak pernah dipanggil** dari handler manapun.
- Tidak ada import `checkRateLimit` di `chat.js`, `chatCore`, atau handler lainnya.
- Rate limit repo (`rateLimitRepo.js`) dan API route (`src/app/api/keys/[id]/rate-limit/`) ada untuk **CRUD management**, tapi enforcement-nya tidak terhubung ke pipeline.
- Query-nya (`SELECT COUNT/SUM FROM usageHistory WHERE apiKey = ? AND timestamp >= ?`) juga tidak punya composite index optimal (lihat poin 5).

### 4. Adapter Selection — sql.js Bukan Pencetus (Tapi Tetap Berbahaya)

**File:** `src/lib/db/driver.js:55-74`

```
Order: better-sqlite3 → node:sqlite (≥22.5) → sql.js
```

- Di Node.js production: **better-sqlite3** atau **node:sqlite** yang aktif (keduanya sync API).
- sql.js (`src/lib/db/adapters/sqljsAdapter.js:24-27`) adalah fallback terakhir, tapi kalau aktif, bottleneck-nya lebih parah karena `db.export()` + `fs.writeFileSync()` yang menserialisasikan seluruh database setiap write.
- **Kesimpulan:** sql.js kemungkinan tidak aktif di production, tapi masalah sync blocking tetap ada di semua adapter.

### 5. Index Kurang untuk Query Rate Limit

**File:** `src/lib/db/schema.js:121-127`

Index yang ada di `usageHistory`:
```sql
CREATE INDEX idx_uh_ts ON usageHistory(timestamp DESC)
CREATE INDEX idx_uh_provider ON usageHistory(provider)
CREATE INDEX idx_uh_model ON usageHistory(model)
CREATE INDEX idx_uh_conn ON usageHistory(connectionId)
CREATE INDEX idx_uh_apiKey ON usageHistory(apiKey)
```

**Yang tidak ada:** Composite index `(apiKey, timestamp)`.  
Query rate limit (`checkRateLimit`):
```sql
SELECT COUNT(*) FROM usageHistory WHERE apiKey = ? AND timestamp >= ?
```

Tanpa composite index, SQLite melakukan **full scan** pada filtered rows (menggunakan `idx_uh_apiKey` lalu filter `timestamp` secara manual). Di tabel dengan ribuan baris, ini lambat.

---

## Perbandingan Adapter

| Adapter | API Type | Write Mechanism | Bottleneck |
|---------|----------|-----------------|------------|
| `better-sqlite3` | Sync | Direct `db.run()` | Event loop blocked per write |
| `node:sqlite` | Sync | Direct `db.run()` | Event loop blocked per write |
| `bun:sqlite` | Sync | Direct `db.run()` | Event loop blocked per write |
| `sql.js` | Sync | `db.export()` + `fs.writeFileSync()` | Worst: full DB serialization + file I/O per write |

**Semua adapter synchronous.** Tidak ada yang truly async.

---

## Dampak Akumulasi

```
100 concurrent agents × 3 settings reads + 1 usage write per request
= ~400 synchronous DB operations stacked sequentially
= Event loop blocked ~2-6 detik total
= Semua request lain menunggu (FIFO queue)
= Responsifitas aplikasi turun drastis
```

---

## Rekomendasi Fix (Prioritas)

### P1 — Hindari Blocking Write di Request Path

**Opsi A (Recommended):** Defer `saveRequestUsage()` ke background queue  
- Jangan blocking response — request sudah selesai, baru tulis ke DB.
- Gunakan `queueMicrotask()` atau simple async queue (single writer).
- File target: `src/lib/db/repos/usageRepo.js:243`, `open-sse/handlers/chatCore/requestDetail.js:93`, `open-sse/utils/usageTracking.js:345`

**Opsi B:** Wire up `checkRateLimit()` ke pipeline  
- Panggil `checkRateLimit()` di `chat.js` setelah validasi API key, sebelum routing.
- Tambah composite index `CREATE INDEX idx_uh_apiKey_ts ON usageHistory(apiKey, timestamp)`.

### P2 — Cache `getSettings()` di Memory

- `getSettings()` dibaca 3x per request tapi settings jarang berubah.
- Implementasi TTL cache (contoh: 5 detik) di `settingsRepo.js`.
- Mengurangi 3 sync reads → 0 reads per request (setelah cache warm).

### P3 — Pertimbangkan Async Wrapper untuk DB Writes

- `better-sqlite3` dan `node:sqlite` punya sync API tapi thread-safe (bukan thread-safe untuk concurrent writes dalam single process).
- Alternatif: pakai `worker_threads` untuk isolate DB operations, atau switch ke adapter yang support async (misal `@electric-sql/pglite` untuk PostgreSQL, tapi itu perubahan besar).

---

## File Terkait

| File | Peran |
|------|-------|
| `src/lib/db/repos/usageRepo.js:243` | `saveRequestUsage()` — sync transaction 3 writes |
| `src/lib/db/schema.js:11` | `busy_timeout = 5000` — max wait untuk lock |
| `src/lib/db/driver.js:55` | Adapter selection order |
| `src/lib/db/adapters/sqljsAdapter.js:24` | `db.export()` + `writeFileSync()` — worst case |
| `src/sse/handlers/chat.js:69,127,201` | `getSettings()` 3x per request |
| `src/lib/rateLimit/checker.js:9` | `checkRateLimit()` — dead code, tidak terhubung |
| `src/lib/db/schema.js:121` | Index `usageHistory` — tidak ada composite `(apiKey, timestamp)` |

---

## Langkah Verifikasi

1. Cek driver aktif: lihat log startup `[DB] Driver: ...`
2. Benchmark: jalankan `tests/unit/db-benchmark.test.js` dan `tests/unit/db-concurrent.test.js`
3. Profile event loop: pakai `--inspect` + Chrome DevTools untuk lihat blocking duration
