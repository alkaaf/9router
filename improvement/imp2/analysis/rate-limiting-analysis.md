# Rate Limiting by API Key — Strategic Brief

**Project**: `imp2` — Rate Limiting berbasis API Key untuk 9Router
**Status**: Pre-implementation — analysis complete
**Date**: 2026-05-31
**Referensi**: imp1 (Per-Key Usage Tracking) — selesai fase 0-3

---

## 1. Executive Summary

9Router adalah AI proxy/router yang menghubungkan CLI tools dan aplikasi ke 40+ LLM providers. Saat ini, setiap API key yang valid bisa melakukan **unlimited requests** selama provider account-nya tersedia. Tidak ada mekanisme rate limiting di level API key.

Project ini menambahkan **rate limiting berbasis API key** — setiap key bisa diatur limit request dan/atau token per window (1h, 6h, 12h, 24h, 7d). Rule ditempelkan per API key dan bisa dikelola dari dashboard.

**Strategic decision: duplication over refactoring** — mengikuti pola imp1. Sistem rate limiting berjalan paralel dengan sistem existing, tidak mengubah kode yang sudah bekerja di production.

---

## 2. Problem Statement

### Pain Points

1. **No API key throttling** — Admin tidak bisa membatasi usage per API key. Satu key yang abused bisa menghabiskan semua quota provider.
2. **No per-key budget control** — Tidak ada cara untuk allocate budget (requests/tokens) ke tim atau service tertentu.
3. **No visibility into key-level limits** — Tidak ada cara untuk monitor seberapa dekat sebuah key dengan limit-nya.
4. **Provider fallback ≠ usage control** — Fallback mechanism saat account rate-limited adalah milik provider, bukan dari API key user. User tetap bisa memakan unlimited quota.

### Goals

| Goal | Metric |
|------|--------|
| Request throttling per key | Admin bisa set max requests per window |
| Token throttling per key | Admin bisa set max tokens per window |
| Dashboard visibility | Admin bisa lihat remaining + reset time per key |
| Zero regression | Sistem existing (provider fallback, usage tracking) tetap berfungsi |

### Non-Goals

- Mengubah sistem provider account locking (tetap menggunakan exponential backoff)
- Rate limiting untuk local mode (tanpa API key)
- Distributed rate limiting (multi-instance cluster) — MVP fokus single process
- Per-user (end-user application) rate limiting — ini hanya untuk API key level

---

## 3. Strategic Approach: Parallel Track Isolation

### Philosophy

**"Build a new gate next to the old road. Don't repave."**

Alih-alih modify sistem usage tracking yang sudah ada, kita **membangun rate limiting secara paralel** yang:

- Memiliki tabel database sendiri (`apiKeyRateLimits`)
- Memiliki logic enforcement sendiri (`checkRateLimit()`)
- Memiliki dashboard UI sendiri (manage rules + remaining display)
- **Tidak mengubah** `usageRepo.js`, `usageHistory` write path, atau SSE handler logic yang sudah ada (kecuali menambahkan 1-2 baris check)

### Bounded Duplication Principle

| Aspect | Strategy |
|--------|----------|
| Database | **SHARED** — query `usageHistory` yang sudah ada |
| Data Layer (usage) | **REUSE** — `usageRepo.js` tidak diubah |
| Rate Limit Logic | **NEW** — `checker.js` baru, isolated |
| Enforcement | **INJECT** — 1-2 baris di setiap handler |
| Dashboard UI | **REUSE + EXTEND** — existing per-key page + new rule manager |
| Styling | **REUSE** — design system yang ada |

---

## 4. System Architecture

### High-Level Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     Request Flow (with Rate Limit)               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Client Request                                                 │
│    │                                                            │
│    ▼                                                            │
│  SSE Handler (chat.js, embeddings.js, etc.)                     │
│    │                                                            │
│    ├─ 1. Extract API key                                        │
│    ├─ 2. Validate API key (existing)                           │
│    ├─ 3. ★ checkRateLimit(apiKey) ★  ← NEW                     │
│    │       │                                                    │
│    │       ├─ Query apiKeyRateLimits table                      │
│    │       ├─ For each active rule:                             │
│    │       │     ├─ Calculate current window (jam bulat)        │
│    │       │     ├─ COUNT/SUM usageHistory in window            │
│    │       │     └─ Compare with limit                          │
│    │       │                                                    │
│    │       ├─ If exceeded → return 429                         │
│    │       └─ If pass → continue                                │
│    │                                                            │
│    ├─ 4. Select provider credentials (existing)                │
│    └─ 5. Forward to provider                                    │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Dashboard                                                      │
│    │                                                            │
│    ├─ /dashboard/keys/[id]                                     │
│    │   ├─ Rule list (type, limit, window, status)              │
│    │   ├─ Add/edit rule form                                   │
│    │   └─ Remaining display (progress bar + reset timer)       │
│    │                                                            │
│    └─ /dashboard/usage-per-key/[keyId]  (imp1, existing)       │
│        └─ Shows usage data — no change needed                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                      Data Layer                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  NEW TABLE:                                                     │
│  apiKeyRateLimits                                                │
│    ├─ id (TEXT PK)                                              │
│    ├─ apiKeyId (FK → apiKeys.id)                               │
│    ├─ type ("requests" | "tokens")                             │
│    ├─ limit (INTEGER)                                           │
│    ├─ windowHours (INTEGER: 1, 6, 12, 24, 168)                 │
│    ├─ enabled (INTEGER DEFAULT 1)                              │
│    └─ createdAt (TEXT)                                          │
│                                                                 │
│  SHARED TABLE (untouched):                                      │
│  usageHistory (apiKey, timestamp, promptTokens, completionTokens)│
│  apiKeys (id, key, name, machineId, isActive)                  │
│                                                                 │
│  NEW MODULE:                                                    │
│  src/lib/rateLimit/checker.js                                   │
│    └─ checkRateLimit(apiKey) → { exceeded, rule, remaining }   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### URL Structure

```
Dashboard:
  /dashboard/keys/[id]                    ← Edit rules + remaining (existing page, extended)
  /dashboard/usage-per-key/[keyId]        ← Usage display (imp1, unchanged)

API:
  GET    /api/keys/[id]/rate-limit        ← List rules
  POST   /api/keys/[id]/rate-limit        ← Create rule
  PUT    /api/keys/[id]/rate-limit/[rid]  ← Update rule
  DELETE /api/keys/[id]/rate-limit/[rid]  ← Delete rule
```

---

## 5. Data Flow

### Enforcement Flow

```
1. Client sends request with API key
   → Authorization: Bearer sk-{machine}-{id}-{crc}

2. SSE handler extracts + validates API key
   → isValidApiKey(apiKey) → true

3. ★ checkRateLimit(apiKey) — NEW ★
   → Get all active rules for this API key
   → For each rule:
     a. Calculate current window (jam bulat based on windowHours)
     b. Query usageHistory:
        - requests type: COUNT(*) WHERE apiKey=? AND timestamp in window
        - tokens type: SUM(promptTokens + completionTokens) WHERE apiKey=? AND timestamp in window
     c. Compare usage with limit
   → If any rule exceeded → return 429 with Retry-After header
   → If all pass → continue to credential selection

4. Existing flow continues unchanged
   → getProviderCredentials() → handleChatCore() → response
```

### Rule Management Flow

```
Admin → /dashboard/keys/[id]
  → See list of rules for this key
  → Add new rule: type=requests, limit=100, window=24h
  → POST /api/keys/[id]/rate-limit
    → INSERT INTO apiKeyRateLimits
  → Rule immediately active on next request

Admin → /dashboard/keys/[id]
  → See remaining: "45/100 requests (reset in 6h 23m)"
  → Progress bar showing 45% used
```

### Window Calculation

```
windowHours = 24, now = 2026-05-31 14:30

windowStart = floor(14.5 / 24) * 24h = 0h
           = 2026-05-31 00:00:00

windowEnd = 2026-06-01 00:00:00

windowKey = "2026-05-31T00:00:00.000Z|24h"
```

---

## 6. Implementation Phases

### Phase 1: Database + Core Logic (estimasi 2 jam)

| Item | Action | File |
|------|--------|------|
| Tambah tabel `apiKeyRateLimits` | Edit `schema.js` indexes/tables | `src/lib/db/schema.js` |
| Tambah index `idx_uh_apiKey_ts` | Composite index untuk query performance | `src/lib/db/schema.js` |
| Buat `rateLimitRepo.js` | CRUD operations untuk rules | `src/lib/db/repos/rateLimitRepo.js` |
| Buat `checker.js` | Core rate limit check logic | `src/lib/rateLimit/checker.js` |

### Phase 2: Enforcement (estimasi 1-2 jam)

| Item | Action | File |
|------|--------|------|
| Inject di `chat.js` | Tambah check setelah `isValidApiKey()` | `src/sse/handlers/chat.js` |
| Inject di `embeddings.js` | Sama pattern | `src/sse/handlers/embeddings.js` |
| Inject di `fetch.js` | Sama pattern | `src/sse/handlers/fetch.js` |
| Inject di `search.js` | Sama pattern | `src/sse/handlers/search.js` |
| Inject di `imageGeneration.js` | Sama pattern | `src/sse/handlers/imageGeneration.js` |
| Inject di `tts.js` | Sama pattern | `src/sse/handlers/tts.js` |
| Inject di `stt.js` | Sama pattern | `src/sse/handlers/stt.js` |

### Phase 3: API + Dashboard (estimasi 2-3 jam)

| Item | Action | File |
|------|--------|------|
| API: GET rules list | Tambah route handler | `src/app/api/keys/[id]/rate-limit/route.js` |
| API: POST create rule | Tambah route handler | `src/app/api/keys/[id]/rate-limit/route.js` |
| API: PUT update rule | Tambah route handler | `src/app/api/keys/[id]/rate-limit/route.js` |
| API: DELETE rule | Tambah route handler | `src/app/api/keys/[id]/rate-limit/route.js` |
| Dashboard: Rule list + form | UI component | `src/app/(dashboard)/dashboard/keys/[id]/page.js` |
| Dashboard: Remaining display | Progress bar + timer | reuse `QuotaProgressBar` pattern |

### Phase 4: Testing (estimasi 1-2 jam)

| Item | Action |
|------|--------|
| Unit test checker | Test various window types, edge cases |
| Integration test | Test enforcement returns 429, passes when under limit |
| Edge case testing | Window boundary, multiple rules, concurrent requests |
| Dashboard test | CRUD rules, remaining display accuracy |

**Total Estimate: 6-8 jam**

---

## 7. Pertimbangan Teknis

### 7.1 Race Condition

- Multiple request bisa datang bersamaan untuk API key yang sama
- `better-sqlite3` sync + single Node.js process = `usageHistory` write sudah atomic (inside `db.transaction()`)
- Rate limit check (SELECT COUNT/SUM) dan enforcement (INSERT new row) terjadi di handler yang sama — tidak ada gap untuk race condition dalam single process
- Jika cluster mode (multi-process), butuh row-level lock atau pre-check + post-check

### 7.2 Refresh Window: Jam Bulat vs Rolling

| Approach | Deskripsi | Cocok untuk |
|----------|-----------|-------------|
| **Jam bulat** | Reset semua user di waktu yang sama (00:00, 06:00, 12:00, 18:00) | Usage harian/mingguan, lebih fair |
| **Rolling** | 24h dari setiap request individual | Per-user personalized limit |

**Rekomendasi: jam bulat** — konsisten dengan pola `usageDaily` yang sudah pakai `dateKey` (tanggal lokal).

### 7.3 Token Counting Strategy

| Approach | Deskripsi | Kelebihan | Kekurangan |
|----------|-----------|-----------|------------|
| **Pre-request estimate** | Hitung dari body size / message count | Block sebelum kirim ke provider | Tidak akurat, bisa false positive |
| **Post-request actual** | Hitung dari `prompt_tokens` + `completion_tokens` setelah response | Akurat | Tidak bisa block sebelum request |
| **Hybrid** | Block by requests count (pre), track tokens (post) | Best of both | Tokens tidak bisa enforcement secara hard |

**Rekomendasi**: Hybrid — `requests` type untuk hard enforcement (block sebelum request), `tokens` type untuk post-hoc tracking + warning. Ini menghindari false-positive dari token estimation yang tidak akurat.

### 7.4 Local Mode (No API Key)

- Request tanpa API key (`local-no-key`) tidak perlu di-rate-limit
- Enforcement hanya berlaku jika `apiKey` ada dan teridentifikasi
- Handler sudah punya logika: `const apiKey = extractApiKey(request)` — bisa langsung reuse

### 7.5 Kombinasi dengan imp1

| imp1 Feature | Penggunaan untuk Rate Limiting |
|-------------|-------------------------------|
| `usageHistory.apiKey` | Primary data source untuk query COUNT/SUM |
| `usageHistory` index `idx_uh_apiKey` | Sudah ada dari imp1 Phase 0 |
| `getUsageHistory({ apiKey })` | Bisa dipakai untuk audit/log |
| `/api/usage/per-key/[keyId]` | Dashboard bisa menampilkan remaining + reset time |
| `usageDaily.byApiKey` | Pre-aggregated data untuk display (tidak untuk enforcement) |

---

## 8. Scope Boundaries

### IN Scope

- Rate limiting per API key (requests + tokens)
- Configurable rules per key (CRUD via dashboard + API)
- Window-based refresh (jam bulat: 1h, 6h, 12h, 24h, 7d)
- 429 response with Retry-After header
- Dashboard UI untuk manage rules + display remaining
- Per-key usage page menampilkan rate limit status

### OUT of Scope

- Distributed rate limiting (multi-instance cluster)
- Per-user (end-user application) rate limiting
- Per-model atau per-provider rate limiting (sudah ada di account lock)
- Rate limiting untuk MITM proxy traffic
- Token estimation pre-request (hanya request count untuk enforcement)

---

## 9. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Race condition pada concurrent requests | Medium | Medium | `better-sqlite3` sync — single process, atomic transaction |
| False positive token estimation | Medium | Low | Gunakan request count untuk hard enforcement, tokens untuk tracking |
| Admin accidentally locks all keys | Low | High | Tambah "emergency bypass" — admin key yang tidak di-rate-limit |
| Window boundary edge case | Low | Low | Jam bulat calculation — `Math.floor(now / windowMs) * windowMs` |
| Performance impact pada high-traffic | Low | Medium | Index `idx_uh_apiKey_ts` (composite) untuk fast COUNT/SUM query |
| MITM proxy bypass | Low | Low | Rate limit berlaku di handler level, bukan di edge |

---

## 10. File Reference

| Domain | Key Files |
|--------|-----------|
| Database | `src/lib/db/schema.js`, `src/lib/db/driver.js` |
| Data Layer | `src/lib/db/repos/rateLimitRepo.js` (new), `src/lib/db/repos/usageRepo.js` (read-only) |
| Rate Limit Logic | `src/lib/rateLimit/checker.js` (new) |
| Enforcement | `src/sse/handlers/chat.js` (+ 6 other handlers) |
| API Routes | `src/app/api/keys/[id]/rate-limit/route.js` (new) |
| Dashboard | `src/app/(dashboard)/dashboard/keys/[id]/page.js` |
| Auth Service | `src/sse/services/auth.js` |
| Referensi | `improvement/imp1/` (per-key usage tracking) |

---

## 11. Referensi imp1

| imp1 File | Kegunaan untuk imp2 |
|-----------|---------------------|
| `improvement/imp1/analysis/analysis-1-db-schema.md` | Pattern untuk schema + migration |
| `improvement/imp1/analysis/analysis-9-data-layer-filtering.md` | Pattern untuk query filtering per apiKey |
| `improvement/imp1/2-domain-plan/README.md` | Template untuk domain breakdown + task tables |
| `improvement/imp1/1-strategic/README.md` | Template untuk strategic brief + architecture diagram |
| `src/lib/db/repos/apiKeysRepo.js` | Reuse `getApiKeyById()` untuk resolve UUID → full key |
| `src/app/api/usage/per-key/[keyId]/route.js` | Pattern untuk API endpoint baru |

---

*Document version: 1.0 — Created 2026-05-31 — For implementation reference only*
