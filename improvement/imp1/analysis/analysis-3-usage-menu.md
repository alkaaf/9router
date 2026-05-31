# Analysis 3: Usage & Analytics Menu — How It Works

## Overview

Menu Usage adalah halaman analytics yang menampilkan 3 tab: **Overview**, **Logs**, dan **Details**.

## Struktur Halaman

```
src/app/(dashboard)/dashboard/usage/
├── page.js                          — Entry point, tab switching
└── components/
    ├── OverviewCards.js             — 4 KPI cards
    ├── ProviderTopology.js          — React Flow graph providers
    ├── RequestDetailsTab.js         — Paginated request log + drawer
    ├── UsageChart.js                — Recharts area chart (tokens/cost over time)
    ├── UsageTable.js                — Sortable grouped table (by model/account/apiKey/endpoint)
    ├── RequestLogger.js             — Live log stream
    └── ProviderLimits/              — Quota display per provider
        ├── index.js
        ├── ProviderLimitCard.js
        ├── QuotaProgressBar.js
        ├── QuotaTable.js
        └── utils.js
```

## Komponen Utama

### 1. `page.js` — Tab Controller

**File**: `src/app/(dashboard)/dashboard/usage/page.js`

- Baca query param `?tab=` → `overview` (default) | `logs` | `details`
- Period selector: `today | 24h | 7d | 30d | 60d`
- Render:
  - `overview` → `<UsageStats>`
  - `logs` → `<RequestLogger>`
  - `details` → `<RequestDetailsTab>`

### 2. `UsageStats.js` — Main Dashboard (Orchestrator)

**File**: `src/shared/components/UsageStats.js` (505 lines)

Fetch data dari 2 sumber secara paralel:
1. **REST**: `GET /api/usage/stats?period=X` — full stats agregat
2. **SSE**: `GET /api/usage/stream` — real-time delta (activeRequests, recentRequests, errorProvider)

Render tree:
```
OverviewCards (4 KPI)
ProviderTopology + RecentRequests (grid: 2/3 + 1/3)
UsageChart (area chart, toggle tokens/cost)
UsageTable (grouped sortable table, 4 view modes)
```

### 3. `UsageTable.js` — Grouped Table

**File**: `src/app/(dashboard)/dashboard/usage/components/UsageTable.js`

4 view modes via dropdown:
1. **Usage by Model** — group by raw model name
2. **Usage by Account** — group by connection name
3. **Usage by API Key** — group by apiKey name ← SUDAH ADA
4. **Usage by Endpoint** — group by endpoint URL

Setiap row bisa di-expand untuk breakdown per-model/per-account.

### 4. `UsageChart.js` — Time Series

**File**: `src/app/(dashboard)/dashboard/usage/components/UsageChart.js`

- Area chart (recharts) — tokens atau cost
- Fetches `GET /api/usage/chart?period=X`

### 5. `RequestDetailsTab.js` — Detail Log

**File**: `src/app/(dashboard)/dashboard/usage/components/RequestDetailsTab.js`

- Filter: provider, startDate, endDate
- Table: Timestamp | Model | Provider | In | Out | Latency | Action
- Drawer: 4 collapsible sections (Client Request, Provider Request, Provider Response, Client Response)

### 6. `RequestLogger.js` — Live Log

**File**: `src/shared/components/RequestLogger.js`

- Auto-refresh 3 detik
- Fetches `GET /api/usage/logs` (getRecentLogs 200)
- Columns: DateTime | Model | Provider | Account | In | Out | Status

## API Routes

| Route | Fungsi |
|-------|--------|
| `GET /api/usage/stats?period=` | Stats agregat penuh (byModel, byAccount, byApiKey, byEndpoint) |
| `GET /api/usage/chart?period=` | Time-series untuk chart |
| `GET /api/usage/stream` | SSE real-time |
| `GET /api/usage/history` | Raw usage history |
| `GET /api/usage/logs` | 200 log terakhir |
| `GET /api/usage/request-details` | Paginated detail records |
| `GET /api/usage/providers` | Distinct providers list |
| `GET /api/usage/[connectionId]` | Quota native dari provider |

## Data Layer

**File**: `src/lib/usageDb.js` (shim) → `src/lib/db/repos/usageRepo.js` (real impl)

Key functions:
- `saveRequestUsage(entry)` — insert + upsert + aggregate
- `getUsageStats(period)` — agregasi lengkap
- `getChartData(period)` — time-series buckets
- `getUsageHistory(filter)` — raw rows (tanpa filter apiKey)
- `getRecentLogs(limit)` — formatted log strings
- `trackPendingRequest()` / `getActiveRequests()` — live tracking

## State Management

Tidak ada store khusus untuk usage — state entirely local via `useState` / `useEffect` di setiap component.

Persistence:
- `localStorage` untuk expanded row state (`usage-stats:expanded-models`, dll)
- `localStorage` untuk quota cache (`quotaCacheData`)

### File Terkait

| File | Peran |
|------|-------|
| `src/app/(dashboard)/dashboard/usage/page.js` | Entry point + tab switching |
| `src/shared/components/UsageStats.js` | Orchestrator utama (fetch + render semua section) |
| `src/app/(dashboard)/dashboard/usage/components/UsageTable.js` | Grouped sortable table |
| `src/app/(dashboard)/dashboard/usage/components/UsageChart.js` | Area chart |
| `src/app/(dashboard)/dashboard/usage/components/OverviewCards.js` | 4 KPI cards |
| `src/app/(dashboard)/dashboard/usage/components/RequestDetailsTab.js` | Detail log + drawer |
| `src/shared/components/RequestLogger.js` | Live log stream |
| `src/lib/usageDb.js` | Barrel export ke usageRepo |
| `src/lib/db/repos/usageRepo.js` | Semua query + write operations |
