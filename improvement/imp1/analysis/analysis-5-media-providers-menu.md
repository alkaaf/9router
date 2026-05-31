# Analysis 5: Media Providers Menu — Submenu Pattern untuk "Usage per User"

## Overview

Analisis struktur menu Media Providers (yang punya accordion submenu) sebagai pola untuk menambahkan submenu baru "Usage per User".

## Current Media Providers Structure

### Sidebar Definition

**File**: `src/shared/components/Sidebar.js`

**Lines 42** — Accordion state:
```js
const [mediaOpen, setMediaOpen] = useState(false);
```

**Lines 203-217** — Accordion trigger:
```jsx
<button onClick={() => setMediaOpen((v) => !v)}>
  <span className="material-symbols-outlined">perm_media</span>
  <span>Media Providers</span>
  <span style={{ transform: mediaOpen ? "rotate(180deg)" : "rotate(0deg)" }}>
    expand_more
  </span>
</button>
```

**Lines 218-250** — Accordion content (children links):
```jsx
{mediaOpen && (
  <div className="pl-4">
    {MEDIA_PROVIDER_KINDS.filter(k => VISIBLE_MEDIA_KINDS.includes(k.id)).map(kind => (
      <Link key={kind.id} href={`/dashboard/media-providers/${kind.id}`}>
        <span className="material-symbols-outlined text-[16px]">{kind.icon}</span>
        <span className="text-sm">{kind.label}</span>
      </Link>
    ))}
    <Link href="/dashboard/media-providers/web">Web Fetch & Search</Link>
  </div>
)}
```

### Constant Definition

**File**: `src/shared/constants/providers.js:179-189`

```js
export const MEDIA_PROVIDER_KINDS = [
  { id: "embedding",   label: "Embedding",      icon: "data_array" },
  { id: "image",       label: "Text to Image",  icon: "brush" },
  { id: "imageToText", label: "Image to Text",  icon: "image_search" },
  { id: "tts",         label: "Text To Speech", icon: "record_voice_over" },
  { id: "stt",         label: "Speech To Text", icon: "mic" },
  { id: "webSearch",   label: "Web Search",     icon: "travel_explore" },
  { id: "webFetch",    label: "Web Fetch",      icon: "language" },
  { id: "video",       label: "Video",          icon: "movie" },
  { id: "music",       label: "Music",          icon: "music_note" },
];
```

**Visibility filter** (Sidebar.js line 14-17):
```js
const VISIBLE_MEDIA_KINDS = ["embedding", "image", "tts", "stt"];
```

## Ide untuk "Usage per User" Submenu

### Opsi A: Submenu baru di bawah Usage (Recommended)

Tambah accordion baru di bagian System section, parallel dengan Media Providers:

```
Sidebar Structure:
├── Main Section
│   ├── Endpoint
│   ├── Providers
│   ├── Combos
│   ├── Usage ────────────────────┐
│   ├── Quota Tracker             │
│   ├── MITM                      │  Existing top-level items
│   └── CLI Tools                 │
│                                  │
├── System Section                 │
│   ├── Proxy Pools               │
│   ├── Skills                    │
│   ├── Media Providers ──────────┤  Accordion (existing)
│   │   ├── Embedding            │
│   │   ├── Text to Image        │
│   │   ├── Text To Speech       │
│   │   └── Speech To Text       │
│   └── Usage per User ──────────┘  Accordion (NEW)
│       ├── API Key 1 (sk-abc...) │
│       ├── API Key 2 (sk-def...) │
│       └── API Key 3 (sk-ghi...) │
```

**Perubahan yang dibutuhkan:**

1. **Fetch API keys** — tambah state `const [keys, setKeys] = useState([])` + `useEffect` fetch dari `/api/keys`
2. **Tambah accordion state** — `const [usagePerKeyOpen, setUsagePerKeyOpen] = useState(false)`
3. **Render links** — map keys menjadi `<Link>` items dengan icon `key` dan label `${key.name} (${key.key.slice(0,8)}...)`

### Opsi B: Submenu di bawah Usage nav item

Ubah `Usage` dari top-level link menjadi accordion:

```
│   ├── Usage ────────────────────┐
│   │   ├── Overview             │  ← existing
│   │   └── Usage per User ──────┤  ← NEW (dynamic list of API keys)
│   │       ├── API Key 1        │
│   │       └── API Key 2        │
```

**Keuntungan:** Lebih logical — "Usage per User" adalah sub-feature dari "Usage".
**Kerugian:** Perlu refactor `Usage` dari simple link menjadi accordion.

### Opsi C: Dropdown (hover-based)

Ganti accordion dengan dropdown yang muncul saat hover/click pada "Usage":
- Lebih compact
- Tidak ada state accordion yang perlu di-maintain
- Akan hilang saat user klik di luar

## Rekomendasi: Opsi A

**Alasan:**
1. Minimal disruption — tidak perlu ubah existing nav items
2. Pattern sudah ada (mirip Media Providers accordion)
3. Mudah diimplementasikan — copy-paste pattern Media Providers, ganti data source dari `MEDIA_PROVIDER_KINDS` ke API keys
4. Bisa tambah badge count (misal jumlah usage per key)

## Struktur Halaman "Usage per User"

Ketika user klik salah satu API key di submenu:

```
Route: /dashboard/usage-per-key/[keyId]
```

Halaman menampilkan:
- Header: API key name + masked key string
- 4 KPI cards (same style OverviewCards)
- Chart: usage over time untuk key ini saja
- Table: breakdown by model + provider untuk key ini
- Real-time SSE: stream filtered by apiKey

**Ini adalah subset dari halaman Usage yang sudah ada** — bisa reuse:
- `UsageStats.js` — pass `apiKey` sebagai filter prop
- `UsageChart.js` — pass `apiKey` filter ke API
- `UsageTable.js` — pass `apiKey` filter
- `OverviewCards.js` — pass `apiKey` filter

## API Endpoint yang Dibutuhkan

```
GET /api/usage/per-key/[keyId]?period=7d
→ { stats: { totalRequests, totalPromptTokens, ... }, chartData: [...], history: [...] }
```

Atau lebih simpel — reuse endpoint yang ada dengan tambahan filter:

```
GET /api/usage/stats?period=7d&apiKey=sk-abc...
GET /api/usage/chart?period=7d&apiKey=sk-abc...
GET /api/usage/history?apiKey=sk-abc...
```

## File yang Akan Diubah/Dibuat

| File | Aksi | Keterangan |
|------|------|-----------|
| `src/shared/components/Sidebar.js` | EDIT | Tambah accordion "Usage per User" + fetch API keys |
| `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` | NEW | Halaman usage per key |
| `src/app/api/usage/per-key/[keyId]/route.js` | NEW | API endpoint (atau tambah filter di endpoint existing) |
| `src/lib/usageDb.js` | EDIT | Tambah `filter.apiKey` di `getUsageHistory()` + index |

### File Terkait

| File | Peran |
|------|-------|
| `src/shared/components/Sidebar.js:42,203-250` | Accordion pattern yang akan di-copy |
| `src/shared/constants/providers.js:179-189` | `MEDIA_PROVIDER_KINDS` — pattern untuk constant list |
| `src/app/(dashboard)/dashboard/usage/page.js` | Tab controller — pattern untuk page structure |
| `src/shared/components/UsageStats.js` | Orchestrator — bisa di-reuse dengan filter prop |
| `src/lib/db/repos/apiKeysRepo.js` | `getApiKeys()` — source data untuk submenu list |
