# Analysis 7: SSE / Real-time Communication Patterns

## Overview

Analisis bagaimana aplikasi menggunakan SSE (Server-Sent Events) untuk real-time updates, agar fitur "Usage per User" bisa meniru pattern yang sama.

## SSE Systems yang Ada

Aplikasi punya **2 sistem SSE independen**:

### 1. Usage Stream (statsEmitter)

**Server**: `src/app/api/usage/stream/route.js`
**Client**: `src/shared/components/UsageStats.js:255-278`

### 2. Console Log Stream

**Server**: `src/app/api/translator/console-logs/stream/route.js`
**Client**: `src/app/(dashboard)/dashboard/console-log/ConsoleLogClient.js:36-58`

## 1. Usage Stream — Detail Lengkap

### Server Side

**File**: `src/app/api/usage/stream/route.js`

```js
// Dua payload types:
// 1. Full stats (cachedStats + activeRequests) — via state.send
// 2. Lightweight pending update (activeRequests + recent + errorProvider) — via state.sendPending

export async function GET(request) {
  const stream = new ReadableStream({
    start(controller) {
      state.controller = controller;
      // Send initial full stats immediately
      state.send();

      // Subscribe to events
      statsEmitter.on("update", state.send);      // full refresh
      statsEmitter.on("pending", state.sendPending); // lightweight

      // Keepalive every 25s
      state.keepalive = setInterval(() => state.send(), 25000);
    },
    cancel() {
      statsEmitter.off("update", state.send);
      statsEmitter.off("pending", state.sendPending);
      clearInterval(state.keepalive);
    }
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
}
```

### statsEmitter — Singleton EventEmitter

**File**: `src/lib/db/repos/usageRepo.js:11-18`

```js
// Singleton on global to survive Next.js HMR
if (!global._statsEmitter) {
  global._statsEmitter = new EventEmitter();
  global._statsEmitter.setMaxListeners(50);
}
export const statsEmitter = global._statsEmitter;
```

### Events Emitted

| Event | Kapan Di-emit | Payload |
|-------|---------------|---------|
| `"pending"` | Request starts/ends (trackPendingRequest) | `{ activeRequests, recentRequests, errorProvider }` |
| `"update"` | Request completes + usage saved (saveRequestUsage) | Full stats object |

### Sumber Pemanggil

**File**: `open-sse/handlers/chatCore.js`
- Line 129, 137, 140 — `trackPendingRequest(model, provider, connectionId, started)` → emit `"pending"`

**File**: `src/lib/usageDb.js:283`
- `saveRequestUsage()` → `pushToRing(entry)` → `statsEmitter.emit("update")`

### Client Side

**File**: `src/shared/components/UsageStats.js:255-278`

```js
useEffect(() => {
  const es = new EventSource("/api/usage/stream");

  es.onmessage = (e) => {
    const data = JSON.parse(e.data);
    setStats((prev) => ({
      ...(prev || {}),
      activeRequests: data.activeRequests,
      recentRequests: data.recentRequests,
      errorProvider: data.errorProvider,
      pending: data.pending,
    }));
  };

  es.onerror = () => setLoading(false);
  return () => es.close();
}, []);
```

**Pola penting:** SSE hanya mengirim delta (activeRequests, recentRequests, errorProvider). Full stats tetap di-fetch via REST. SSE merge ke state yang sudah ada — tidak overwrite.

### REST + SSE Hybrid Pattern

```js
// REST fetch — full data, triggered by period change
useEffect(() => {
  async function load() {
    const res = await fetch(`/api/usage/stats?period=${period}`);
    const data = await res.json();
    setStats(data);  // full replacement
  }
  load();
}, [period]);

// SSE stream — delta only, always active
useEffect(() => {
  const es = new EventSource("/api/usage/stream");
  es.onmessage = (e) => {
    setStats((prev) => ({ ...prev, ...deltaFields }));
  };
  return () => es.close();
}, []);
```

## 2. Console Log Stream — Pattern Referensi

**File**: `src/app/api/translator/console-logs/stream/route.js`

Perbedaan penting dari usage stream:

| Aspek | Usage Stream | Console Log Stream |
|-------|-------------|-------------------|
| EventEmitter | `global._statsEmitter` (singleton) | `global._consoleLogBufferState.emitter` (per-resource) |
| Message format | JSON object | `{ type: "init"/"line"/"clear", ... }` — typed messages |
| Cleanup | `cancel()` callback | `request.signal` + `cancel()` — lebih robust |
| Keepalive | 25s interval | Tidak ada (browser handles) |
| Initial data | Full stats immediately | `type: "init"` message with buffer contents |

**Console log pattern yang lebih baik untuk reuse:**

```js
start(controller) {
  state.controller = controller;

  // Initial data
  controller.enqueue(new TextEncoder().encode(`data: ${JSON.stringify({ type: "init", logs })}\n\n`));

  // Subscribe with typed events
  emitter.on("line", (line) => state.send({ type: "line", text: line }));
  emitter.on("clear", () => state.send({ type: "clear" }));

  // Cleanup on client abort
  request.signal.addEventListener("abort", () => {
    emitter.off("line", handler);
    emitter.off("clear", handler);
    clearInterval(keepalive);
  });
}
```

## Bagaimana Cara Reuse untuk "Usage per User" SSE

### Opsi A: Filter Existing Stream (Simple)

Tambah query param `?apiKey=sk-xxx` ke `/api/usage/stream`:

```js
// route.js
const apiKey = searchParams.get("apiKey");

statsEmitter.on("update", (data) => {
  if (apiKey && !data.byApiKey?.[apiKey]) return; // filter
  state.send(data);
});
```

**Keuntungan:** Satu endpoint, tidak perlu infrastructure baru.
**Kerugian:** Semua client terima semua data lalu difilter di client — bandwidth waste.

### Opsi B: New Endpoint per API Key (Recommended)

Buat `/api/usage/per-key/[keyId]/stream` — SSE endpoint terpisah:

```js
// src/app/api/usage/per-key/[keyId]/stream/route.js
export async function GET(request, { params }) {
  const { keyId } = params;
  // Filter statsEmitter events by keyId
  // Kirim hanya data yang relevan untuk key ini
}
```

**Keuntungan:**
- Hanya kirim data yang relevan — bandwidth efisien
- Bisa track per-key active requests terpisah
- Lebih clean untuk isolation

**Kerugian:**
- Perlu endpoint baru
- Perlu manage lifecycle (banyak client bisa connect ke key yang sama)

### Opsi C: Per-Key EventEmitter (Scalable)

Buat `global._perKeyStreamEmitters[keyId]` — EventEmitter terpisah per API key:

```js
// UsageRepo.js
const perKeyEmitters = {};

export function getKeyEmitter(keyId) {
  if (!perKeyEmitters[keyId]) {
    perKeyEmitters[keyId] = new EventEmitter();
  }
  return perKeyEmitters[keyId];
}
```

Kemudian di `saveRequestUsage()`:
```js
const emitter = getKeyEmitter(entry.apiKey);
emitter.emit("update", { request: entry });
```

**Keuntungan:** Perfect isolation, scalable.
**Kerugian:** Perlu manage cleanup (hapus emitter saat tidak ada subscriber).

## Rekomendasi untuk Fitur Ini

**Gunakan Opsi A untuk MVP** — tambah query param filter ke endpoint existing:
- Perubahan minimal di `stream/route.js`
- Client `UsageStats.js` bisa pass `apiKey` sebagai filter
- Cukup tambah `?apiKey=` di EventSource URL

**Jika butuh isolate nanti, migrate ke Opsi B** — endpoint terpisah per key.

## Pattern untuk Client SSE

```js
// Reusable pattern untuk per-key usage page
function usePerKeyStream(keyId) {
  const [data, setData] = useState(null);

  useEffect(() => {
    const es = new EventSource(`/api/usage/stream?apiKey=${encodeURIComponent(keyId)}`);
    es.onmessage = (e) => {
      const delta = JSON.parse(e.data);
      setData((prev) => ({ ...prev, ...delta }));
    };
    es.onerror = () => console.error("SSE error");
    return () => es.close();
  }, [keyId]);

  return data;
}
```

## File Terkait

| File | Peran |
|------|-------|
| `src/app/api/usage/stream/route.js` | SSE endpoint — pattern untuk di-copy/modify |
| `src/lib/db/repos/usageRepo.js:11-18` | `statsEmitter` singleton |
| `src/lib/db/repos/usageRepo.js:153-196` | `trackPendingRequest()` — emit "pending" |
| `src/lib/db/repos/usageRepo.js:243-283` | `saveRequestUsage()` — emit "update" |
| `src/shared/components/UsageStats.js:255-278` | Client SSE consumer — pattern untuk reuse |
| `src/app/api/translator/console-logs/stream/route.js` | Alternative SSE pattern (typed messages, signal cleanup) |
| `src/lib/consoleLogBuffer.js` | Per-resource EventEmitter pattern |
