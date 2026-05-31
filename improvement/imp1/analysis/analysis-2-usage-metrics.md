# Analysis 2: Usage Metrics Generation & Tagging by API Key

## Overview

Analisis cara usage metrics di-generate dan potensi penambahan grouping/filtering berdasarkan `apiKeyId`.

## Bagaimana Usage Metrics Di-generate

### 1. Ekstraksi Usage dari SSE Stream

**File**: `open-sse/handlers/chatCore/stream.js`

Format yang didukung (priority order):
1. **Claude** — `message_delta` event → `input_tokens` / `output_tokens`
2. **OpenAI Responses API** — `response.completed` → `input_tokens`, `output_tokens`, `cached_tokens`, `reasoning_tokens`
3. **OpenAI standard** — `chunk.usage` → `prompt_tokens` / `completion_tokens`
4. **Gemini/Antigravity** — `usageMetadata` → `promptTokenCount` / `candidatesTokenCount`
5. **Ollama** — `done` + `prompt_eval_count` / `eval_count`

Fallback: `estimateUsage(body, contentLength)` jika provider tidak return usage.

### 2. Penyimpanan Usage

**File**: `src/lib/usageDb.js:243-287`

`saveRequestUsage()` melakukan 3 operasi dalam satu transaction:
1. **INSERT** ke `usageHistory` — row per request
2. **UPSERT** ke `usageDaily` — JSON aggregate per hari
3. **INCREMENT** `_meta.totalRequestsLifetime`

### 3. Agregasi usageDaily

**File**: `src/lib/usageDb.js:44-77`

```
aggregateEntryToDay(dayData, entry):
  dayData.requests += 1
  dayData.promptTokens += entry.promptTokens
  dayData.completionTokens += entry.completionTokens
  dayData.cost += entry.cost
  dayData.byProvider[provider] += ...
  dayData.byModel[model|provider] += ...
  dayData.byAccount[connectionId] += ...
  dayData.byApiKey[apiKey|model|provider] += ...  ← SUDAH ADA
  dayData.byEndpoint[endpoint] += ...
```

### 4. Query untuk Stats

**File**: `src/lib/usageDb.js:319-618`

`getUsageStats(period)`:
- **7d/30d/60d**: baca `usageDaily` JSON blob, parse `byApiKey`
- **24h/today**: aggregate langsung dari `usageHistory` rows

Returns:
```js
{
  totalRequests, totalPromptTokens, totalCompletionTokens, totalCost,
  byProvider: {}, byModel: {}, byAccount: {}, byApiKey: {}, byEndpoint: {}
}
```

`getChartData(period)` — time series buckets (hourly untuk 24h, daily untuk 7d/30d/60d).

## Potensi Penambahan Tagging `apiKeyId`

### Opsi A: Work with Current Schema (Recommended untuk MVP)

**Perubahan yang dibutuhkan:**
1. Tambah index: `CREATE INDEX idx_uh_apiKey ON usageHistory(apiKey);`
2. Tambah filter di `getUsageHistory(filter)` — parameter `filter.apiKey`
3. Query `byApiKey` dari `usageDaily` sudah bisa difilter (sudah ada)

**Keunggulan:**
- Migrasi simpel (1 index + 1 filter tambahan)
- Data sudah tercatat
- Query per-key bisa dilakukan via `usageHistory` atau `usageDaily.byApiKey`

**Kelemahan:**
- `apiKey` disimpan sebagai string full (bukan FK ke `apiKeys.id`)
- Grouping by `apiKey` tanpa index = full scan untuk dataset besar
- `usageDaily.byApiKey` key adalah composite `${apiKey}|${model}|${provider}` — filtering by `apiKey` saja perlu iterasi semua key

### Opsi B: Tambah Kolom `apiKeyId` (FK ke tabel apiKeys)

**Perubahan yang dibutuhkan:**
1. Tambah kolom `apiKeyId INTEGER` di `usageHistory`
2. Tambah kolom `apiKeyId INTEGER` di `usageDaily` (atau pisah tabel aggregate)
3. Populate `apiKeyId` saat `saveRequestUsage()` — lookup dari `apiKeys` table
4. Tambah index: `CREATE INDEX idx_uh_apiKeyId ON usageHistory(apiKeyId);`

**Keunggulan:**
- Query ter-normalisasi, bisa JOIN ke `apiKeys` untuk resolve nama
- Storage lebih efisien (INTEGER vs TEXT)
- Grouping by key ID lebih cepat

**Kelemahan:**
- Perlu DB lookup per request (overhead ~0.1ms per query)
- Perlu migration + backfill data existing
- Lebih kompleks

### Opsi C: Tabel Baru `usageByApiKey`

**Struktur:**
```sql
CREATE TABLE usageByApiKey (
  dateKey TEXT NOT NULL,
  apiKeyId TEXT NOT NULL,
  requests INTEGER DEFAULT 0,
  promptTokens INTEGER DEFAULT 0,
  completionTokens INTEGER DEFAULT 0,
  cost REAL DEFAULT 0,
  PRIMARY KEY (dateKey, apiKeyId)
);
```

**Keunggulan:**
- Query super cepat untuk dashboard per-key
- Terpisah dari `usageHistory` (tidak perlu scan row-by-row)

**Kelemahan:**
- Data duplicated dari `usageHistory`
- Perlu maintain consistency
- Over-engineering untuk fitur yang bisa dilakukan via `usageDaily`

## Rekomendasi

**Gunakan Opsi A** untuk fitur "Usage per API Key":

1. Tambah index pada `usageHistory(apiKey)` — 1 baris migration
2. Tambah parameter `apiKey` di `getUsageHistory()` — 5 baris kode
3. Query `usageDaily.byApiKey` untuk agregat harian — sudah ada

Ini cukup untuk menampilkan halaman usage per-api-key yang performan baik untuk skala menengah (< 1J requests).

Jika nanti butuh performa lebih, bisa migrate ke Opsi B.

### File Terkait

| File | Peran |
|------|-------|
| `src/lib/db/schema.js:105-150` | Schema tabel |
| `src/lib/usageDb.js:44-77` | `aggregateEntryToDay()` — agregasi per apiKey |
| `src/lib/usageDb.js:243-287` | `saveRequestUsage()` — penyimpanan |
| `src/lib/usageDb.js:319-618` | `getUsageStats()` — query agregat |
| `src/lib/usageDb.js:289-307` | `getUsageHistory()` — query raw |
| `src/lib/usageDb.js:620-691` | `getChartData()` — time series |
