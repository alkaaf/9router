# Analysis 6: API Endpoint & Middleware Patterns

## Overview

Analisis standar aplikasi untuk membuat endpoint API baru, agar fitur "Usage per User" mengikuti pattern yang sudah ada.

## Middleware: `dashboardGuard`

**File**: `src/dashboardGuard.js` (re-export via `src/proxy.js`)

### Route Classification

| Kategori | Paths | Auth Requirement |
|----------|-------|-----------------|
| **PUBLIC_API_PATHS** | `/api/health`, `/api/init`, `/api/locale`, `/api/auth/*`, `/api/version`, `/api/settings/require-login` | Tidak perlu auth |
| **PUBLIC_PREFIXES** | `/v1`, `/v1beta`, `/api/v1`, `/api/v1beta` | Auth dicek di dalam handler (API key) |
| **LOCAL_ONLY_PATHS** | `/api/mcp/*`, `/api/tunnel/*`, `/api/cli-tools/cowork-settings`, OAuth auto-imports | CLI token ATAU loopback + JWT |
| **ALWAYS_PROTECTED** | `/api/shutdown`, `/api/settings/database`, `/api/version/update` | JWT ATAU CLI token |
| **PROTECTED_API_PATHS** | `/api/settings`, `/api/keys`, `/api/providers`, `/api/usage`, `/api/combos`, dll | JWT ATAU `requireLogin=false` ATAU CLI token |

### Auth Decision Flow

```
Request
  → Is LOCAL_ONLY? → CLI token OR (loopback + JWT)
  → Is ALWAYS_PROTECTED? → CLI token OR JWT
  → Is /v1 or /v1beta? → CLI token OR loopback OR valid API key
  → Is /api/* (general)? → Public allow-list OR CLI token OR authenticated
  → Is /dashboard/*? → check requireLogin → redirect if needed
  → Allow through
```

### Three Auth Mechanisms

| Mechanism | Header | Validasi | Dipakai Untuk |
|-----------|--------|----------|---------------|
| **API Key** | `Authorization: Bearer sk-...` atau `x-api-key: sk-...` | DB lookup `validateApiKey()` | LLM API calls |
| **CLI Token** | `x-9r-cli-token` | `token === getCliToken()` (machineId-based) | Local process calls |
| **JWT Session** | Cookie `auth_token` | jose HS256, 24h expiry | Dashboard browser |

## Standard Pattern untuk API Endpoint Baru

### File Location

```
src/app/api/<resource>/<sub-resource>/route.js
```

Contoh yang ada:
- `src/app/api/usage/stats/route.js`
- `src/app/api/usage/chart/route.js`
- `src/app/api/keys/[id]/route.js`

### Required Exports

```js
export const dynamic = "force-dynamic";  // Opt out of static rendering

export async function GET(request) { ... }
export async function POST(request) { ... }
export async function PUT(request) { ... }
export async function DELETE(request) { ... }
```

### Response Format Convention

| Jenis | Format | Status Code |
|-------|--------|-------------|
| Success (list) | `NextResponse.json({ keys: [...] })` | 200 |
| Success (single) | `NextResponse.json(resultObject)` | 200 |
| Created | `NextResponse.json({ resource: result }, { status: 201 })` | 201 |
| Validation error | `NextResponse.json({ error: "..." }, { status: 400 })` | 400 |
| Auth error | `NextResponse.json({ error: "..." }, { status: 401 })` | 401 |
| Not found | `NextResponse.json({ error: "..." }, { status: 404 })` | 404 |
| Server error | `NextResponse.json({ error: "..." }, { status: 500 })` | 500 |

### Minimal GET Example

```js
// src/app/api/usage/stats/route.js
import { NextResponse } from "next/server";
import { getUsageStats } from "@/lib/usageDb";

const VALID_PERIODS = new Set(["today", "24h", "7d", "30d", "60d", "all"]);
export const dynamic = "force-dynamic";

export async function GET(request) {
  try {
    const { searchParams } = new URL(request.url);
    const period = searchParams.get("period") || "7d";
    if (!VALID_PERIODS.has(period)) {
      return NextResponse.json({ error: "Invalid period" }, { status: 400 });
    }
    const stats = await getUsageStats(period);
    return NextResponse.json(stats);
  } catch (error) {
    console.error("[API] Failed:", error);
    return NextResponse.json({ error: "Failed to fetch" }, { status: 500 });
  }
}
```

## Database Access Pattern (Repo Pattern)

### Location

Semua repos di `src/lib/db/repos/`:

| File | Fungsi |
|------|--------|
| `usageRepo.js` | Usage CRUD + stats + chart + pending tracking |
| `apiKeysRepo.js` | API key CRUD + validate |
| `settingsRepo.js` | Settings read/write |
| `connectionsRepo.js` | Provider connections CRUD |
| `pricingRepo.js` | Pricing lookup + update |

### Standard Repo Pattern

```js
import { getAdapter } from "../driver.js";

// READ
export async function getXxx(filter = {}) {
  const db = await getAdapter();
  const rows = db.all(`SELECT * FROM table WHERE ...`, [...params]);
  return rows.map(rowToXxx);
}

// CREATE
export async function createXxx(data) {
  const db = await getAdapter();
  db.run(`INSERT INTO table(...) VALUES(?, ?, ...)`, [...values]);
  return created;
}

// UPDATE (atomic)
export async function updateXxx(id, data) {
  const db = await getAdapter();
  db.transaction(() => {
    const row = db.get(`SELECT * FROM table WHERE id = ?`, [id]);
    const merged = { ...rowToXxx(row), ...data };
    db.run(`UPDATE table SET ... WHERE id = ?`, [...]);
  });
  return result;
}

// DELETE
export async function deleteXxx(id) {
  const db = await getAdapter();
  db.run(`DELETE FROM table WHERE id = ?`, [id]);
}
```

### Key Details

- `getAdapter()` — returns SQLite driver (lazy loaded via dynamic import)
- Mutating operations — always wrap in `db.transaction()` untuk atomicity
- JSON columns — handled via `parseJson` / `stringifyJson` helpers
- Sensitive data — stripped before returning (API keys, tokens)

## Endpoint Baru untuk "Usage per User"

### Route: `GET /api/usage/per-key/[keyId]?period=7d`

**Auth**: Route ini masuk kategori `PROTECTED_API_PATHS` (line 48-66) → requires JWT atau `requireLogin=false` atau CLI token. Tidak perlu auth tambahan di route handler.

**Query params:**
- `period` — `today | 24h | 7d | 30d | 60d` (default: `7d`)
- Optional: `provider`, `model` — untuk further filtering

**Response:**
```json
{
  "keyId": "uuid",
  "keyName": "My API Key",
  "period": "7d",
  "stats": {
    "totalRequests": 1234,
    "totalPromptTokens": 56789,
    "totalCompletionTokens": 23456,
    "totalCost": 45.67
  },
  "byModel": { "claude-sonnet-4-6": { requests: 500, tokens: 20000, cost: 18.50 }, ... },
  "chartData": [
    { "label": "Mon", "tokens": 5000, "cost": 2.50 },
    ...
  ]
}
```

### Atau: Tambah Filter ke Endpoint Existing (Lebih Simpel)

Tambah parameter `apiKey` di endpoint yang sudah ada:

```
GET /api/usage/stats?period=7d&apiKey=sk-abc...
GET /api/usage/chart?period=7d&apiKey=sk-abc...
GET /api/usage/history?apiKey=sk-abc...
```

Keuntungan:
- Tidak perlu endpoint baru
- Reuse logic yang sudah ada
- Perubahan kecil di `usageRepo.js`

Kerugian:
- Semua client yang panggil endpoint ini juga bisa pass `apiKey` param (tapi tidak masalah karena data apinya sendiri)

### File Terkait

| File | Peran |
|------|-------|
| `src/dashboardGuard.js:165-242` | Proxy middleware — auth enforcement |
| `src/dashboardGuard.js:48-66` | `PROTECTED_API_PATHS` — auth requirements per route |
| `src/dashboardGuard.js:22-35` | `PUBLIC_API_PATHS` / `PUBLIC_PREFIXES` |
| `src/app/api/usage/stats/route.js` | Pattern untuk GET endpoint |
| `src/app/api/keys/route.js` | Pattern untuk POST endpoint |
| `src/app/api/keys/[id]/route.js` | Pattern untuk DELETE/PUT by ID |
| `src/lib/usageDb.js` | Barrel export ke usageRepo |
| `src/lib/db/repos/usageRepo.js` | Semua usage query + write operations |
| `src/lib/db/repos/apiKeysRepo.js` | API key lookup — untuk resolve keyId → key name |
| `src/lib/db/driver.js` | `getAdapter()` — SQLite driver access |
