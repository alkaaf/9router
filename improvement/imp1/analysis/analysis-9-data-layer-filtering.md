# Analysis 9: Data Layer — Per-Key Filtering Strategy

## Overview

Analisis mendalam tentang bagaimana data layer (`usageRepo.js`) bisa di-adaptasi untuk "Usage per User" — khususnya: cara agregasi `getUsageStats()` bisa difilter per-apiKey, `getChartData()` bisa di-filter, dan `getUsageHistory()` bisa ditambah filter `apiKey`.

## Current State of Data Layer

### Function Inventory

| Function | Lines | Purpose | Has apiKey Filter? |
|----------|-------|---------|-------------------|
| `getUsageStats(period)` | 319-618 | Full agregasi stats (300 baris) | **NO** |
| `getChartData(period)` | 620-691 | Time-series buckets untuk chart | **NO** |
| `getUsageHistory(filter)` | 289-307 | Raw usage rows | **NO** (filter: provider, model, date) |
| `saveRequestUsage(entry)` | 243-287 | Insert + upsert + lifetime counter | N/A (writer) |
| `aggregateEntryToDay(day, entry)` | 44-77 | Aggregate 1 entry ke daily JSON | N/A (internal) |
| `trackPendingRequest()` | 153-196 | Live pending tracking | N/A (in-memory) |
| `getActiveRequests()` | 198-241 | Read pending state | N/A (in-memory) |

### `getUsageStats()` — Deep Dive (300 baris)

**File**: `src/lib/db/repos/usageRepo.js:319-618`

Dua jalur eksekusi berdasarkan period:

#### Jalur A: Daily Summary (period = "7d", "30d", "60d", "all")

```js
const useDailySummary = period !== "24h" && period !== "today";
```

Baca `usageDaily` JSON blobs → parse → iterate `day.byApiKey`:

```js
for (const [akKey, ak] of Object.entries(day.byApiKey || {})) {
  // akKey = `${apiKeyVal}|${model}|${provider || "unknown"}`
  // ak = { requests, promptTokens, completionTokens, cost, rawModel, provider, apiKey, keyName, ... }
  const apiKeyVal = ak.apiKey;  // <-- SUDAH ADA di JSON
  const keyInfo = apiKeyMap[apiKeyVal];  // resolve dari apiKeys table
  const keyName = keyInfo?.name || (apiKeyVal ? apiKeyVal.slice(0, 8) + "..." : "Local (No API Key)");
  // aggregate ke stats.byApiKey[akKey]
}
```

**Penting**: `usageDaily.data.byApiKey` sudah menyimpan `apiKey` (full string) di setiap entry. Jadi data untuk per-key filtering SUDAH ADA di JSON blob.

Setelah loop daily summary, ada **overlay** dari `usageHistory` untuk `lastUsed` timestamp yang lebih akurat (line 505-530).

#### Jalur B: Live History (period = "24h" atau "today")

```js
const filtered = db.all(`SELECT ... FROM usageHistory WHERE timestamp >= ?`, [cutoff]);
for (const r of filtered) {
  if (r.apiKey && typeof r.apiKey === "string") {
    // aggregate ke stats.byApiKey[...]
  } else {
    // aggregate ke stats.byApiKey["local-no-key"]
  }
}
```

**Penting**: Di sini `r.apiKey` adalah full string (`sk-{machineId}-{uuid}-{crc}`). Filter per-key bisa dilakukan langsung di SQL dengan `AND apiKey = ?`.

### `getChartData()` — Deep Dive

**File**: `src/lib/db/repos/usageRepo.js:620-691`

Tiga jalur:

| Period | Source | Query |
|--------|--------|-------|
| `today` | `usageHistory` | `SELECT timestamp, promptTokens, completionTokens, cost WHERE timestamp >= ?` |
| `24h` | `usageHistory` | Same as today, different cutoff |
| `7d/30d/60d` | `usageDaily` | `SELECT dateKey, data FROM usageDaily WHERE dateKey >= ?` |

Untuk daily periods, data diambil dari JSON blob:
```js
const dayData = dayMap[dateKey];
return {
  tokens: dayData ? (dayData.promptTokens || 0) + (dayData.completionTokens || 0) : 0,
  cost: dayData ? (dayData.cost || 0) : 0,
};
```

**Masalah**: `dayData.promptTokens` adalah **total** (semua API key), bukan per-key. Untuk chart per-key, kita butuh extract dari `dayData.byApiKey`.

### `getUsageHistory()` — Deep Dive

**File**: `src/lib/db/repos/usageRepo.js:289-307`

```js
export async function getUsageHistory(filter = {}) {
  const conds = [];
  const params = [];
  if (filter.provider) { conds.push("provider = ?"); params.push(filter.provider); }
  if (filter.model) { conds.push("model = ?"); params.push(filter.model); }
  if (filter.startDate) { conds.push("timestamp >= ?"); params.push(new Date(filter.startDate).toISOString()); }
  if (filter.endDate) { conds.push("timestamp <= ?"); params.push(new Date(filter.endDate).toISOString()); }
  // TIDAK ADA: filter.apiKey
  const rows = db.all(`SELECT ... FROM usageHistory ${where} ORDER BY id ASC`, params);
}
```

### Key Resolution: UUID → Full API Key String

**File**: `src/lib/db/repos/apiKeysRepo.js`

```js
export async function getApiKeyById(id) {
  const db = await getAdapter();
  const row = db.get(`SELECT * FROM apiKeys WHERE id = ?`, [id]);
  return rowToKey(row);  // → { id, key (FULL string), name, machineId, isActive, createdAt }
}
```

URL pattern: `/dashboard/usage-per-key/[keyId]` dimana `keyId` = UUID.

Flow resolusi:
```
URL: /dashboard/usage-per-key/abc-123-uuid
  → keyId = "abc-123-uuid"
  → getApiKeyById(keyId) → { key: "sk-abc123...def456...xyz", name: "My Key" }
  → filter: apiKey = "sk-abc123...def456...xyz"
```

## Refactoring Strategies

### Strategy A: Tambah Filter Parameter ke `getUsageStats()` (Minimal Change)

```js
export async function getUsageStats(period = "all", filter = {}) {
  const { apiKey, provider, model } = filter;
  // ... existing logic, tapi tambah kondisi filter di setiap aggregation loop
}
```

**Yang berubah:**
- Daily summary path (line 420-503): Tambah `if (apiKey && ak.apiKey !== apiKey) continue;` di loop `byApiKey`
- Live history path (line 546-613): Tambah `if (apiKey && r.apiKey !== apiKey) continue;` di loop rows
- Overlay path (line 506-530): Tambah filter di query

**Keuntungan:**
- Tidak ada code duplication
- Satu function untuk semua use case
- Client bisa panggil `getUsageStats("7d", { apiKey: "sk-..." })`

**Kerugian:**
- 300 baris kode jadi lebih kompleks dengan conditional logic
- Setiap aggregation loop perlu tambah 1-2 baris filter

**Ukuran perubahan: ~15 baris tambahan** (conditional filter di 3-4 tempat)

### Strategy B: Extract Core Aggregator + Wrapper Functions (Clean)

```js
// Internal: pure aggregation tanpa filter
async function _buildStats(period, connectionMap, apiKeyMap, providerNodeNameMap) {
  // ... existing 300 baris, TANPA perubahan
}

// Public: aggregate all (existing behavior)
export async function getUsageStats(period = "all") {
  const [{ getProviderConnections }, { getApiKeys }, { getProviderNodes }] = await Promise.all([...]);
  const [allConnections, allApiKeys, allNodes] = await Promise.all([...]);
  return _buildStats(period, connectionMap, apiKeyMap, providerNodeNameMap);
}

// NEW: aggregate per-key
export async function getPerKeyStats(keyId, period = "7d") {
  const key = await getApiKeyById(keyId);
  if (!key) throw new Error("API key not found");
  const [{ getProviderConnections }, { getProviderNodes }] = await Promise.all([...]);
  const stats = await _buildStats(period, connectionMap, { [key.key]: { name: key.name, id: key.id } }, providerNodeNameMap);
  // Filter hasil: hanya byApiKey entries yang match key ini
  const targetKey = key.key;
  stats.byApiKey = Object.fromEntries(
    Object.entries(stats.byApiKey).filter(([k, v]) => v.apiKey === targetKey || k.startsWith(targetKey + "|"))
  );
  // Recalculate totals dari filtered byApiKey
  stats.totalRequests = Object.values(stats.byApiKey).reduce((s, v) => s + v.requests, 0);
  stats.totalPromptTokens = Object.values(stats.byApiKey).reduce((s, v) => s + v.promptTokens, 0);
  stats.totalCompletionTokens = Object.values(stats.byApiKey).reduce((s, v) => s + v.completionTokens, 0);
  stats.totalCost = Object.values(stats.byApiKey).reduce((s, v) => s + v.cost, 0);
  // Hapus byProvider/byModel/byAccount/byEndpoint (tidak relevan untuk per-key view)
  delete stats.byProvider;
  delete stats.byModel;
  delete stats.byAccount;
  delete stats.byEndpoint;
  return stats;
}
```

**Keuntungan:**
- `getUsageStats()` tidak berubah — backward compatible
- `getPerKeyStats()` bisa di-evolve独立 tanpa impact existing code
- Post-filtering di level stats object lebih clean daripada conditional di setiap loop

**Kerugian:**
- Over-aggregation: build FULL stats lalu filter — sedikit waste tapi tidak signifikan (agregasi sudah cepat)
- Perlu re-calculate totals setelah filter

**Ukuran perubahan: ~80 baris baru + rename function existing**

### Strategy C: Dedicated Per-Key Functions (Isolation)

Buat function baru tanpa modify existing:

```js
export async function getPerKeyUsageHistory(keyId, filter = {}) {
  const key = await getApiKeyById(keyId);
  if (!key) return [];
  return getUsageHistory({ ...filter, apiKey: key.key });
}

export async function getPerKeyChartData(keyId, period = "7d") {
  const key = await getApiKeyById(keyId);
  if (!key) return [];
  // Untuk 7d/30d/60d: parse usageDaily JSON, extract byApiKey
  // Untuk today/24h: query usageHistory WHERE apiKey = ?
}

export async function getPerKeyStats(keyId, period = "7d") {
  const key = await getApiKeyById(keyId);
  const [history, chartData] = await Promise.all([
    getPerKeyUsageHistory(keyId, { startDate: ... }),
    getPerKeyChartData(keyId, period),
  ]);
  // Aggregate dari raw history → build stats object
}
```

**Keuntungan:**
- Existing functions 100% tidak berubah
- Bisa iterasi cepat tanpa worry breaking existing dashboard

**Kerugian:**
- Code duplication — `getPerKeyStats` perlu re-implementasi aggregation logic
- Maintenance burden: 2 versi aggregation yang harus di-sync

### Recommendation: **Strategy A + B Hybrid**

Gunakan **Strategy A** untuk `getUsageStats()` (tambah `filter.apiKey` parameter) dan **Strategy B** untuk `getChartData()` (extract + wrapper).

#### Alasan:

1. **`getUsageStats()` dengan filter apiKey** — perubahan minimal (conditional di loop), backward compatible (filter default = {} = no filter), satu source of truth untuk aggregation.

2. **`getChartData()` dengan filter apiKey** — untuk today/24h, tambah `WHERE apiKey = ?` di SQL. Untuk 7d/30d/60d, parse `day.byApiKey` dan sum hanya entries yang match. Lebih clean daripada extract+filter karena chart data jauh lebih sederhana (hanya tokens + cost per bucket).

3. **`getUsageHistory()` dengan filter apiKey** — tambah 2 baris:
   ```js
   if (filter.apiKey) { conds.push("apiKey = ?"); params.push(filter.apiKey); }
   ```

### Detailed Implementation Plan

#### 1. `getUsageStats()` — Tambah Filter Parameter

**File**: `src/lib/db/repos/usageRepo.js:319`

```js
export async function getUsageStats(period = "all", filter = {}) {
  const filterApiKey = filter.apiKey || null;
  // ... existing setup (connectionMap, apiKeyMap, providerNodeNameMap)
```

**Perubahan di Jalur A (daily summary, line 471-487):**

```js
for (const [akKey, ak] of Object.entries(day.byApiKey || {})) {
  if (filterApiKey && ak.apiKey !== filterApiKey) continue;  // TAMBAH INI
  // ... existing aggregation
}
```

**Perubahan di Jalur B (live history, line 586-603):**

```js
if (r.apiKey && typeof r.apiKey === "string") {
  if (filterApiKey && r.apiKey !== filterApiKey) continue;  // TAMBAH INI
  // ... existing aggregation
} else {
  if (filterApiKey) continue;  // TAMBAH INI — skip "local-no-key" jika filter aktif
  // ... existing aggregation
}
```

**Perubahan di Overlay (line 522-525):**

```js
const apiKeyKey = (e.apiKey && typeof e.apiKey === "string")
  ? `${e.apiKey}|${e.model}|${e.provider || "unknown"}`
  : "local-no-key";
if (filterApiKey && e.apiKey !== filterApiKey) continue;  // TAMBAH INI (sebelum masuk loop aggregation)
```

**Total perubahan: ~6 baris conditional.**

#### 2. `getChartData()` — Tambah Filter Parameter

**File**: `src/lib/db/repos/usageRepo.js:620`

```js
export async function getChartData(period = "7d", filter = {}) {
  const filterApiKey = filter.apiKey || null;
```

**Perubahan untuk today/24h (line 634-646):**

```js
const whereClause = filterApiKey ? " AND apiKey = ?" : "";
const params = filterApiKey ? [new Date(startTime).toISOString(), filterApiKey] : [new Date(startTime).toISOString()];
const rows = db.all(`SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ?${whereClause}`, params);
```

**Perubahan untuk 7d/30d/60d (line 676-690):**

```js
for (const r of dayRows) {
  const dayData = parseJson(r.data, {});
  let dayTokens, dayCost;
  if (filterApiKey) {
    // Sum hanya entries di byApiKey yang match filterApiKey
    dayTokens = 0; dayCost = 0;
    for (const [akKey, ak] of Object.entries(dayData.byApiKey || {})) {
      if (ak.apiKey !== filterApiKey) continue;
      dayTokens += (ak.promptTokens || 0) + (ak.completionTokens || 0);
      dayCost += ak.cost || 0;
    }
  } else {
    dayTokens = (dayData.promptTokens || 0) + (dayData.completionTokens || 0);
    dayCost = dayData.cost || 0;
  }
  // ... return bucket
}
```

**Total perubahan: ~12 baris.**

#### 3. `getUsageHistory()` — Tambah Filter `apiKey`

**File**: `src/lib/db/repos/usageRepo.js:289-307`

```js
if (filter.apiKey) { conds.push("apiKey = ?"); params.push(filter.apiKey); }
```

**Total perubahan: 2 baris.**

#### 4. New Function: `getApiKeyByFullKey(fullKey)`

Untuk resolve full key string → key info (name, id):

```js
export async function getApiKeyByFullKey(fullKey) {
  const db = await getAdapter();
  const row = db.get(`SELECT id, key, name, machineId, isActive, createdAt FROM apiKeys WHERE key = ?`, [fullKey]);
  return rowToKey(row);
}
```

Atau reuse existing `validateApiKey(fullKey)` yang sudah ada (apiKeysRepo.js:70) — tapi return boolean, bukan row. Jadi perlu function baru atau modifikasi `validateApiKey` untuk return row.

#### 5. API Endpoint Shape

```
GET /api/usage/per-key/[keyId]?period=7d

Response:
{
  "keyId": "uuid-here",
  "keyName": "My API Key",
  "keyMasked": "sk-abc1...xyz9",
  "period": "7d",
  "stats": {
    "totalRequests": 1234,
    "totalPromptTokens": 56789,
    "totalCompletionTokens": 23456,
    "totalCost": 45.67
  },
  "byModel": { "claude-sonnet-4-6 (anthropic)": { requests: 500, promptTokens: 20000, ... }, ... },
  "chartData": [
    { "label": "Mon", "tokens": 5000, "cost": 2.50 },
    ...
  ],
  "history": [
    { timestamp, model, provider, promptTokens, completionTokens, cost, status },
    ...
  ]
}
```

Atau lebih simpel — tambah query param `apiKey` ke endpoint existing:

```
GET /api/usage/stats?period=7d&apiKey=sk-abc...
GET /api/usage/chart?period=7d&apiKey=sk-abc...
GET /api/usage/history?apiKey=sk-abc...
```

**Keuntungan query param approach:**
- Tidak perlu endpoint baru
- Reuse existing route + middleware
- Client yang sudah ada tidak affected

**Kerugian:**
- URL mengandung full API key (security concern meskipun authenticated via JWT)
- Tidak ada semantic separation

**Rekomendasi: Gunakan endpoint baru `/api/usage/per-key/[keyId]`** — keyId adalah UUID, bukan full key string. Lebih secure di URL, dan route handler resolve UUID → full key di server side.

## Recommended API Design

### New Endpoint

```
GET /api/usage/per-key/[keyId]?period=7d
```

**Auth**: Sama seperti usage endpoints — `PROTECTED_API_PATHS` → JWT atau CLI token.

**Response** (simplified vs full stats):

```json
{
  "keyId": "uuid",
  "keyName": "Production Key",
  "period": "7d",
  "stats": {
    "totalRequests": 1234,
    "totalPromptTokens": 56789,
    "totalCompletionTokens": 23456,
    "totalCost": 45.67
  },
  "byModel": { ... },
  "chartData": [ ... ],
  "history": [ ... ]
}
```

Tidak perlu kirim `byProvider`, `byAccount`, `byEndpoint` — hanya data yang relevan untuk single key.

### Reuse Existing Components

`UsageStats.js`, `UsageChart.js`, `UsageTable.js`, `OverviewCards.js` bisa di-reuse dengan props:
- `UsageStats` → pass `apiKey` filter (setelah modifikasi)
- `UsageChart` → pass `apiKey` filter (setelah modifikasi)
- `UsageTable` → render hanya `byModel` data (sudah di-filter di API)
- `OverviewCards` → pass `stats.total*` (sudah di-filter di API)

### SSE untuk Per-Key

**Opsi A (Recommended untuk MVP):** Tambah filter di SSE endpoint existing:

```
GET /api/usage/stream?apiKey=sk-abc...
```

Di `route.js`:
```js
const apiKey = searchParams.get("apiKey");
// Di state.send():
if (apiKey) {
  const filtered = stats.filter(e => e.apiKey === apiKey);
  controller.enqueue(encoder.encode(`data: ${JSON.stringify(filtered)}\n\n`));
}
```

**Opsi B:** New endpoint `/api/usage/per-key/[keyId]/stream` untuk isolation.

## Implementation Priority

| Priority | Item | File | Effort | Impact |
|----------|------|------|--------|--------|
| 1 | Tambah `filter.apiKey` di `getUsageStats()` | usageRepo.js | ~6 baris | Core — needed for per-key page |
| 2 | Tambah `filter.apiKey` di `getUsageHistory()` | usageRepo.js | 2 baris | Needed for history table |
| 3 | Tambah `filter.apiKey` di `getChartData()` | usageRepo.js | ~12 baris | Needed for chart |
| 4 | New endpoint `GET /api/usage/per-key/[keyId]` | route.js baru | ~40 baris | API layer |
| 5 | New page `/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` | page.js baru | ~60 baris | UI layer |
| 6 | Sidebar accordion "Usage per User" | Sidebar.js | ~25 baris | Navigation |
| 7 | SSE filter per-key (optional) | stream/route.js | ~10 baris | Nice-to-have |

## File Terkait

| File | Peran |
|------|-------|
| `src/lib/db/repos/usageRepo.js:319-618` | `getUsageStats()` — core aggregation, perlu filter param |
| `src/lib/db/repos/usageRepo.js:620-691` | `getChartData()` — perlu filter param |
| `src/lib/db/repos/usageRepo.js:289-307` | `getUsageHistory()` — perlu filter.apiKey |
| `src/lib/db/repos/usageRepo.js:44-77` | `aggregateEntryToDay()` — byApiKey sudah ada di JSON |
| `src/lib/db/repos/apiKeysRepo.js:22-26` | `getApiKeyById()` — resolve UUID → full key |
| `src/lib/usageDb.js` | Barrel export — tambah exports baru |
| `src/app/api/usage/stats/route.js` | Pattern untuk endpoint (atau buat baru `/api/usage/per-key/[keyId]`) |
| `src/app/api/usage/stream/route.js` | SSE — bisa tambah filter query param |
