# Analysis 1: Database Schema — Usage Per API Key

## Overview

Pertanyaan utama: Apakah database saat ini menyimpan informasi usage **per token keseluruhan** atau **per user (api key)**? Apakah perlu tabel baru?

## Temuan

### Tabel usageHistory — SUDAH ada kolom `apiKey`

**File**: `src/lib/db/schema.js:112`

```js
apiKey: "TEXT",  // kolom sudah ada di schema
```

Namun **tidak ada index** pada kolom ini. Index yang ada:
- `idx_uh_ts` — timestamp DESC
- `idx_uh_provider` — provider
- `idx_uh_model` — model
- `idx_uh_conn` — connectionId

### Tabel usageDaily — SUDAH ada agregasi per apiKey

**File**: `src/lib/usageDb.js:70-72`

Aggregate key: `${apiKey}|${model}|${provider}` — per api key + model + provider.

### Tabel apiKeys — struktur yang ada

**File**: `src/lib/db/repos/apiKeysRepo.js:4-13`

| Kolom | Tipe | Catatan |
|-------|------|---------|
| `id` | TEXT PK | UUID v4 |
| `key` | TEXT UNIQUE | Full API key string (misal `sk-a1b2...`) |
| `name` | TEXT | Nama user-friendly |
| `machineId` | TEXT | Binding ke mesin |
| `isActive` | INTEGER | 1=aktif, 0=paused |
| `createdAt` | TEXT | ISO timestamp |

### Data Flow: apiKey ditrack per request

```
Request masuk
  → chat.js:60 extractApiKey(request)         // dari Authorization header
  → chat.js:211 pass ke handleChatCore         // apiKey = string
  → streamingHandler.js:95 saveUsageStats({..., apiKey})
  → requestDetail.js:93-101 entry.apiKey = apiKey
  → usageRepo.js:261 INSERT ... apiKey ...     // simpan ke usageHistory
  → usageRepo.js:70 aggregateEntryToDay()      // agregasi per apiKey ke usageDaily
```

### Kesimpulan

**Database SUDAH menyimpan usage per api key**, tapi ada gap:

| Aspek | Status |
|-------|--------|
| Kolom `apiKey` di `usageHistory` | ✅ Ada |
| Agregasi `byApiKey` di `usageDaily` | ✅ Ada |
| Index pada `usageHistory.apiKey` | ❌ Tidak ada — full table scan |
| Filter `apiKey` di `getUsageHistory()` | ❌ Tidak ada — hanya filter provider/model/date |
| Tabel khusus per-api-key aggregate | ❌ Tidak ada |

### Apakah perlu tabel baru?

**Tidak wajib.** Kolom yang dibutuhkan sudah ada. Solusi yang cukup:

1. **Tambah index** — `CREATE INDEX idx_uh_apiKey ON usageHistory(apiKey);` — membuat query per-key cepat
2. **Tambah filter apiKey di `getUsageHistory()`** — tambah parameter opsional `filter.apiKey`

Tabel baru hanya diperlukan jika:
- Dataset sangat besar (>1J request) dan performa menjadi masalah
- Perlu tracking hal tambahan yang tidak muat di kolom existing (misal: per-api-key quota realtime, per-key billing)

### File Terkait

| File | Peran |
|------|-------|
| `src/lib/db/schema.js:105-150` | Schema semua tabel |
| `src/lib/usageDb.js:243-287` | `saveRequestUsage()` — insert + upsert + aggregate |
| `src/lib/usageDb.js:44-77` | `aggregateEntryToDay()` — agregasi per apiKey |
| `src/lib/usageDb.js:289-307` | `getUsageHistory()` — query tanpa filter apiKey |
| `src/lib/db/repos/apiKeysRepo.js` | CRUD api keys |
| `src/sse/services/auth.js:286-305` | `extractApiKey()` + `isValidApiKey()` |
