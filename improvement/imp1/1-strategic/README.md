# Usage per User — Strategic Brief & Project Manifest

**Project**: `imp1` — Per-Key Usage Tracking for 9Router
**Status**: Pre-implementation — analysis complete, implementation pending
**Date**: 2026-05-30
**Approach**: Parallel Track (isolation-first, intentional duplication)

---

## 1. Executive Summary

9Router adalah AI proxy/router yang menghubungkan CLI tools dan aplikasi ke 40+ LLM providers. Saat ini, sistem tracking usage hanya menampilkan **agregat global** — tidak ada cara untuk melihat berapa usage per API key (per "user" atau per service).

Project ini menambahkan **halaman "Usage per User"** di dashboard — serupa dengan halaman Usage yang sudah ada, tapi difilter per API key. Setiap API key bisa mewakili: tim, environment (dev/staging/prod), aplikasi, atau client individual.

**Strategic decision: duplication over refactoring.** Sistem baru berjalan paralel dengan sistem existing, tidak mengubah kode yang sudah ada. Ini menghasilkan sedikit redundansi, tapi menghilangkan 100% risiko regresi pada fitur Usage yang sudah berjalan di production.

---

## 2. Problem Statement

### Pain Points

1. **No per-key visibility** — Admin tidak bisa tahu API key mana yang paling banyak digunakan, mana yang idle, mana yang over budget.
2. **No cost allocation** — Tidak ada cara untuk mengalokasikan biaya LLM ke tim atau service yang bersangkutan.
3. **No per-key debugging** — Ketika ada error di API key tertentu, tidak ada cara untuk isolate history-nya tanpa filter manual di tabel.
4. **Existing "Usage by API Key" table is insufficient** — `UsageTable` dengan view mode "by API Key" menampilkan data agregat, tapi bukan halaman dedicated dengan chart, detail history, dan real-time stream.

### Goals

| Goal | Metric |
|------|--------|
| Per-key usage visibility | Admin bisa lihat stats per API key dari sidebar |
| Cost allocation | Setiap key punya breakdown cost + model usage |
| Real-time awareness | SSE stream menampilkan active requests per key |
| Zero regression | Halaman Usage yang ada tetap berfungsi tanpa perubahan |

### Non-Goals

- Mengubah halaman Usage yang ada (tidak ada refactor)
- Mengubah API endpoint yang ada (endpoint baru, bukan modifikasi)
- Menambahkan auth baru (reuse existing JWT/CLI token auth)
- Multi-tenant isolation di level data (shared DB, filtered by key)

---

## 3. Strategic Approach: Parallel Track Isolation

### Philosophy

**"Build a new road next to the old one. Don't repave."**

Alih-alih modify sistem usage yang ada (yang sudah 500+ baris, dioptimasi, dan berjalan di production), kita **membangun sistem baru secara paralel** yang:

- Memiliki endpoint API sendiri (`/api/usage/per-key/[keyId]`)
- Memiliki halaman sendiri (`/dashboard/usage-per-key/[keyId]`)
- Memiliki fungsi data layer sendiri (`getPerKeyStats`, `getPerKeyChartData`, dll)
- Memiliki SSE stream sendiri (opsional)

Sistem existing (Usage dashboard) **tidak disentuh**.

### Why Duplication Is The Right Call Here

| Concern | Refactor Approach | Duplication Approach |
|---------|------------------|---------------------|
| Risk to existing system | High — 300 baris aggregation logic diubah | **Zero** — existing code untouched |
| Testing complexity | High — perlu regression test seluruh usage system | **Low** — test baru hanya test yang baru |
| Development speed | Slower — perlu pahami + refactor existing code | **Faster** — copy pattern, implement isolated |
| Maintenance burden | Lower — satu source of truth | Sedikit higher — tapi bounded |
| Debugging | Complex — perubahan berdampak ke banyak tempat | **Simple** — bug di sistem baru, sistem lama aman |

**Rationale**: Dalam konteks 9Router yang adalah production tool dengan pengguna aktif, **stability > elegance**. Duplikasi 50-80 baris kode adalah harga yang kecil dibandingkan risiko regresi pada fitur yang sudah stabil.

### The Bounded Duplication Principle

Duplikasi tidak berarti copy-paste without thinking. Ada batasan:

| Aspect | Strategy |
|--------|----------|
| UI Components | **REUSE** — `Card`, `UsageChart`, `UsageTable`, `OverviewCards` dipakai ulang via props |
| Data Logic | **DUPLICATE** — fungsi `getPerKeyStats()` baru, tidak modify `getUsageStats()` |
| API Routes | **DUPLICATE** — endpoint baru `/api/usage/per-key/[keyId]`, tidak modify `/api/usage/stats` |
| SSE Streams | **REUSE + FILTER** — endpoint existing ditambah query param `?apiKey=`, tidak perlu endpoint baru untuk MVP |
| Database | **SHARED** — tabel yang sama, query yang sama, hanya filter beda |
| Styling | **REUSE** — globals.css + shared components |

**Hanya yang di-duplicate**: route handler, data aggregation function, dan page component. Yang direuse: UI shell, CSS, design system, shared logic.

---

## 4. System Architecture

### High-Level Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                       9Router Dashboard                          │
├──────────────┬──────────────────────────────────────────────────┤
│              │                                                  │
│   SIDEBAR    │   MAIN CONTENT                                   │
│              │                                                  │
│  ┌────────┐  │   ┌──────────────────────────────────────────┐  │
│  │ Usage  │  │   │  Usage (existing)                        │  │
│  │ >      │──┼──▶│  - /api/usage/stats (existing)           │  │
│  └────────┘  │   │  - UsageStats.js (unchanged)              │  │
│              │   │  - Full aggregate view                    │  │
│  ┌────────┐  │   └──────────────────────────────────────────┘  │
│  │ Usage  │  │                                                  │
│  │ per    │──┼──▶  ┌────────────────────────────────────────┐  │
│  │ User   │  │   │  Usage per Key (NEW)                     │  │
│  │ > Key1 │  │   │  - /api/usage/per-key/[keyId] (new)      │  │
│  │ > Key2 │  │   │  - PerKeyUsagePage.js (new)              │  │
│  │ > Key3 │  │   │  - Filtered per-key view                 │  │
│  └────────┘  │   │  - Reuses Card, Chart, Table components   │  │
│              │   └──────────────────────────────────────────┘  │
└──────────────┴──────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                      API Layer                                  │
├─────────────────────────────────────────────────────────────────┤
│  Existing (untouched):         New (this project):             │
│  GET /api/usage/stats          GET /api/usage/per-key/[keyId]   │
│  GET /api/usage/chart          GET /api/usage/history (extended)│
│  GET /api/usage/stream         GET /api/usage/per-key/[keyId]/  │
│  GET /api/usage/history        stream (optional)                │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    Data Layer                                   │
├─────────────────────────────────────────────────────────────────┤
│  Shared Database (SQLite):                                     │
│  usageHistory (apiKey column, index added)                     │
│  usageDaily (byApiKey already in JSON)                         │
│  apiKeys (source of truth for key metadata)                    │
│                                                                │
│  New functions (isolated):                                     │
│  getPerKeyStats(keyId, period)  → stats filtered by key        │
│  getPerKeyChartData(keyId, period) → chart data per key        │
│  getPerKeyHistory(keyId, filter) → raw rows per key            │
│                                                                │
│  Existing functions (unchanged):                               │
│  getUsageStats(period)         → full aggregate (no filter)    │
│  getChartData(period)          → full chart (no filter)        │
│  getUsageHistory(filter)       → raw rows (no apiKey filter)   │
└─────────────────────────────────────────────────────────────────┘
```

### Component Tree (New Page)

```
PerKeyUsagePage (new page component)
├── Page Header (key name + masked key)
├── OverviewCards (reused)          ← 4 KPI cards
│   └── props: stats.totalRequests, stats.totalPromptTokens, ...
├── UsageChart (reused)             ← Area chart
│   └── props: period, apiKey filter
├── UsageTable (reused)             ← Grouped table
│   └── props: groupedData by model only
└── SSE Stream (optional)           ← Real-time updates
    └── EventSource /api/usage/stream?apiKey=sk-...
```

### URL Structure

```
/dashboard/usage-per-key/[keyId]?period=7d

Where:
  keyId = UUID dari tabel apiKeys (e.g. "a1b2c3d4-...")
  period = today | 24h | 7d | 30d | 60d (default: 7d)

API:
  GET /api/usage/per-key/[keyId]?period=7d
  → { keyId, keyName, period, stats, byModel, chartData, history }

SSE (optional):
  GET /api/usage/stream?apiKey=sk-{machine}-{id}-{crc}
  → filtered real-time delta
```

---

## 5. Data Flow

### End-to-End Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                        REQUEST FLOW                               │
└──────────────────────────────────────────────────────────────────┘

1. User klik "Usage per User" di sidebar
   → Sidebar fetch GET /api/keys (existing endpoint)
   → Render accordion dengan daftar API keys

2. User klik salah satu API key
   → Navigasi ke /dashboard/usage-per-key/[keyId]

3. Page component mount
   → Call getApiKeyById(keyId) → resolve UUID → { key: "sk-...", name: "..." }
   → Parallel fetch:
     a. GET /api/usage/per-key/[keyId]?period=7d  → getPerKeyStats()
        → Internal: getUsageStats(period, { apiKey: "sk-..." })
        → Filter aggregation result → return per-key stats only
     b. GET /api/usage/per-key/[keyId]/chart?period=7d  → getPerKeyChartData()
        → Internal: query usageHistory WHERE apiKey = ? (for today/24h)
        → Or parse usageDaily JSON, extract byApiKey (for 7d+)

4. Components render
   → OverviewCards: stats.totalRequests, stats.totalPromptTokens, ...
   → UsageChart: chartData (tokens + cost per day/hour)
   → UsageTable: stats.byModel (grouped, sortable)

5. Optional: SSE connection
   → EventSource("/api/usage/stream?apiKey=sk-...")
   → On each "pending"/"update" event: filter delta → update local state

┌──────────────────────────────────────────────────────────────────┐
│                        WRITE FLOW                                │
│                    (unchanged, shared)                            │
└──────────────────────────────────────────────────────────────────┘

Request masuk → open-sse handler → logUsage() → saveRequestUsage()
  → INSERT INTO usageHistory (apiKey = "sk-...")
  → UPSERT usageDaily JSON (byApiKey["sk-...|model|provider"] += ...)
  → statsEmitter.emit("update")
```

### Key Data Points

| Entity | Storage | Per-Key Tracked? |
|--------|---------|-------------------|
| `usageHistory.apiKey` | Column di setiap row | ✅ — sudah ada |
| `usageDaily.data.byApiKey` | JSON blob per day | ✅ — sudah ada |
| `usageDaily.data.byApiKey[].apiKey` | Full key string di JSON | ✅ — sudah ada |
| `apiKeys.id` | UUID primary key | — |
| `apiKeys.key` | Full key string (UNIQUE) | — |

**Critical insight**: Data untuk per-key tracking **sudah ada** di database sejak awal. Tidak perlu migrasi schema, tidak perlu tambah tabel. Yang dibutuhkan adalah **view layer** yang bisa filter dan menampilkan data tersebut.

---

## 6. Implementation Phases

### Phase 0: Foundation (prerequisite untuk semua)
**Estimate: 30 menit**

| Item | Action | File |
|------|--------|------|
| Tambah index `idx_uh_apiKey` | Edit `schema.js` indexes array | `src/lib/db/schema.js` |
| Ottomatis jalan di boot berikutnya via `syncSchemaFromTables()` | — | — |

### Phase 1: Data Layer (backend core)
**Estimate: 1-2 jam**

| Item | Action | File |
|------|--------|------|
| `getPerKeyStats(keyId, period)` | New function — resolve keyId → apiKey → call `getUsageStats()` + post-filter | `src/lib/db/repos/usageRepo.js` |
| `getPerKeyChartData(keyId, period)` | New function — parse daily JSON or query history by apiKey | `src/lib/db/repos/usageRepo.js` |
| `getPerKeyHistory(keyId, filter)` | New function — `getUsageHistory({ ...filter, apiKey })` | `src/lib/db/repos/usageRepo.js` |
| Barrel export | Tambah 3 exports baru | `src/lib/usageDb.js` |

### Phase 2: API Layer
**Estimate: 1 jam**

| Item | Action | File |
|------|--------|------|
| `GET /api/usage/per-key/[keyId]` | New route — resolve keyId → call getPerKeyStats | `src/app/api/usage/per-key/[keyId]/route.js` |
| `GET /api/usage/per-key/[keyId]/chart` | New route — call getPerKeyChartData | `src/app/api/usage/per-key/[keyId]/chart/route.js` |
| `GET /api/usage/per-key/[keyId]/history` | New route — call getPerKeyHistory + pagination | `src/app/api/usage/per-key/[keyId]/history/route.js` |

### Phase 3: UI Layer
**Estimate: 2-3 jam**

| Item | Action | File |
|------|--------|------|
| Sidebar accordion | Tambah "Usage per User" accordion + fetch API keys | `src/shared/components/Sidebar.js` |
| Per-key page | New page component — reuse OverviewCards, UsageChart, UsageTable | `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` |
| Period selector | Reuse SegmentedControl pattern | — |

### Phase 4: Real-time (Optional)
**Estimate: 30 menit**

| Item | Action | File |
|------|--------|------|
| SSE filter | Tambah query param `?apiKey=` di stream endpoint | `src/app/api/usage/stream/route.js` |
| Client SSE | Tambah EventSource di per-key page | `page.js` |

### Phase 5: Polish & Testing
**Estimate: 1-2 jam**

| Item | Action |
|------|--------|
| Unit tests | Test getPerKeyStats, getPerKeyChartData, getPerKeyHistory (isolated) |
| Integration tests | Test full flow: keyId → API → UI render |
| Edge cases | Key tidak ditemukan, period tanpa data, empty history |
| Styling polish | Match existing design system, test dark/light mode |

**Total Estimate: 5-8 jam** (design + implementation + testing)

---

## 7. Scope Boundaries

### IN Scope

- Halaman `/dashboard/usage-per-key/[keyId]` dengan KPI + chart + table
- API endpoints untuk stats, chart, history per key
- Sidebar navigation (accordion dengan daftar API keys)
- SSE real-time stream per key (optional, Phase 4)
- Tambah index `idx_uh_apiKey` di database
- Unit + integration test untuk fitur baru

### OUT of Scope

- Modifikasi halaman Usage yang ada (tanpa rename/remove/refactor)
- Modifikasi endpoint usage yang ada (`/api/usage/stats`, `/api/usage/chart`, dll)
- Perubahan pada `getUsageStats()`, `getChartData()`, `getUsageHistory()`
- Multi-tenant data isolation (shared DB tetap)
- Auth/permission per key (semua user yang bisa akses dashboard bisa lihat semua keys)
- Historical data migration (data existing sudah ada `apiKey` column)
- Cost prediction / budgeting alerts
- Usage alerts / quota enforcement

### Future Possibilities (post-launch)

- Merge approach: setelah stabil, refactor untuk reduce duplication
- Per-key quota limits
- Usage comparison antar keys
- Export CSV per key
- Time-range picker (custom dates)

---

## 8. Risk Assessment & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| API key resolution gagal (UUID tidak ketemu) | Medium | Medium | Return 404 dengan error message yang jelas |
| Performance lags pada key dengan ribuan requests | Low | Medium | `usageHistory` belum ada index `apiKey` — **tambahkan di Phase 0** |
| Data mismatch antara usageDaily dan usageHistory | Low | Low | Kedua source of truth — daily JSON untuk agregasi, history untuk detail |
| SSE bandwidth waste (kirim semua data lalu filter client-side) | Low | Low | Optional Phase 4 — bisa skip untuk MVP |
| Sidebar accordion fetch API keys gagal | Low | Low | Silent fail (`.catch(() => {})`) — accordion tampil kosong |
| Key di-delete saat user masih di halaman per-key | Low | Low | Show "Key not found" state di page |
| Period mismatch (chart vs stats) | Low | Low | Gunakan same period parameter di semua request |

---

## 9. Key Decisions & Rationale

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Approach | **Duplication over refactoring** | Zero regression risk. Existing system untouched. |
| New endpoint | `/api/usage/per-key/[keyId]` | UUID di URL lebih secure dari full API key string |
| API key in URL | **Tidak** — pakai keyId (UUID) | Full key string tidak boleh muncul di URL/browser history |
| Data source | `usageDaily` JSON untuk agregasi | Data sudah teraggregate — query lebih cepat dari scan usageHistory |
| Chart data for 7d+ | Parse `usageDaily.byApiKey` | Reuse existing daily aggregation, no re-query |
| Chart data for today/24h | Query `usageHistory WHERE apiKey = ?` | Data terlalu granular untuk JSON blob |
| UI reuse | `OverviewCards`, `UsageChart`, `UsageTable` | Tidak ada alasan rebuild component yang sudah bagus |
| SSE | Optional Phase 4 | Core functionality tidak bergantung pada real-time |
| Sidebar pattern | Accordion (mirip Media Providers) | Pattern yang sudah ada, minimal disruption |
| Auth | Reuse existing `PROTECTED_API_PATHS` | Tidak perlu auth tambahan |
| Testing | Isolated unit tests per new function | Existing test tidak perlu diubah |

---

## 10. Success Criteria

Project ini dianggap **complete** ketika:

1. Admin bisa klik API key di sidebar dan melihat halaman usage-nya
2. Halaman menampilkan 4 KPI cards (requests, input tokens, output tokens, cost)
3. Chart menampilkan tren usage over time untuk key tersebut
4. Table menampilkan breakdown per model (grouped, sortable)
5. Period selector works (today, 24h, 7d, 30d, 60d)
6. Halaman Usage yang ada tetap berfungsi normal (tanpa perubahan)
7. Semua endpoint baru return data yang akurat (match existing usage data)
8. Codebase tetap maintainable — duplication bounded, documented, tidak menyebar

### Definition of Done

- [ ] Phase 0: Index `idx_uh_apiKey` ditambahkan
- [ ] Phase 1: 3 fungsi data layer baru + export
- [ ] Phase 2: 3 API endpoint baru + response schema
- [ ] Phase 3: Sidebar accordion + halaman per-key
- [ ] Phase 4: SSE per-key (optional, nice-to-have)
- [ ] Phase 5: Unit tests pass, manual QA pass
- [ ] Existing usage dashboard: verified no changes, no regressions

---

## 11. Reference: Analysis Artifacts

Semua analisis mendetail tersimpan di `improvement/imp1/analysis/`:

| File | Isi |
|------|-----|
| `analysis-1-db-schema.md` | Schema database — apiKey column exists, index needed |
| `analysis-2-usage-metrics.md` | Usage flow — saveRequestUsage → usageDaily → byApiKey |
| `analysis-3-usage-menu.md` | Struktur halaman Usage yang ada |
| `analysis-4-usage-css.md` | Design system, Card, SegmentedControl, layout |
| `analysis-5-media-providers-menu.md` | Sidebar accordion pattern |
| `analysis-6-endpoint-middleware.md` | API endpoint pattern + auth |
| `analysis-7-sse-realtime.md` | SSE pattern — statsEmitter + EventSource |
| `analysis-8-db-migrations.md` | Migration system — additive sync otomatis |
| `analysis-9-data-layer-filtering.md` | Per-key filtering strategy + implementation plan |

---

*Document version: 1.0 — Created 2026-05-30 — For implementation reference only*
