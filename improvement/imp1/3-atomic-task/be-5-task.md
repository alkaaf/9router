# Atomic Task: be-5 — Create API Route `GET /api/usage/per-key/[keyId]`

**Domain**: Backend (API Layer)
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- `src/lib/usageDb.js` — barrel exports (`getUsageStats`, `getChartData`, `getUsageHistory`)
- `src/lib/db/repos/apiKeysRepo.js` — `getApiKeyById(id)` function
- File path: `src/app/api/usage/per-key/[keyId]/route.js` (create new)

## Output

- New API route file at `src/app/api/usage/per-key/[keyId]/route.js`
- Endpoint returns combined stats + chartData + history for a single API key
- UUID → full key resolution, error handling for invalid keyId

## Process

### Step 1: Create directory structure

```bash
mkdir -p src/app/api/usage/per-key/\[keyId\]
```

### Step 2: Create route file

**File**: `src/app/api/usage/per-key/[keyId]/route.js`

```js
import { NextResponse } from "next/server";
import { getUsageStats, getChartData, getUsageHistory } from "@/lib/usageDb";
import { getApiKeyById } from "@/lib/db/repos/apiKeysRepo";

const VALID_PERIODS = new Set(["today", "24h", "7d", "30d", "60d", "all"]);

export const dynamic = "force-dynamic";

export async function GET(request, { params }) {
  try {
    const { keyId } = await params;
    const { searchParams } = new URL(request.url);
    const period = searchParams.get("period") || "7d";

    if (!VALID_PERIODS.has(period)) {
      return NextResponse.json({ error: "Invalid period" }, { status: 400 });
    }

    const key = await getApiKeyById(keyId);
    if (!key) {
      return NextResponse.json({ error: "API key not found" }, { status: 404 });
    }

    const [stats, chartData, history] = await Promise.all([
      getUsageStats(period, { apiKey: key.key }),
      getChartData(period, { apiKey: key.key }),
      getUsageHistory({ apiKey: key.key }),
    ]);

    const byModel = Object.entries(stats.byModel || {}).map(([name, data]) => ({
      name,
      ...data,
    }));

    return NextResponse.json({
      keyId: key.id,
      keyName: key.name,
      keyMasked: key.key.slice(0, 8) + "..." + key.key.slice(-4),
      period,
      stats: {
        totalRequests: stats.totalRequests,
        totalPromptTokens: stats.totalPromptTokens,
        totalCompletionTokens: stats.totalCompletionTokens,
        totalCost: stats.totalCost,
      },
      byModel,
      chartData,
      history,
    });
  } catch (error) {
    console.error("[API] Failed to get per-key usage:", error);
    return NextResponse.json({ error: "Failed to fetch per-key usage" }, { status: 500 });
  }
}
```

### Step 3: Verify route is accessible

- Start dev server
- `GET /api/usage/per-key/{valid-uuid}` → 200 or 404 (if key doesn't exist)
- `GET /api/usage/per-key/invalid` → 404

## Dependencies

- be-2: `getUsageStats()` with filter (DONE)
- be-3: `getChartData()` with filter (DONE)
- be-4: `getUsageHistory()` with filter (DONE)

## Success Criteria

- Valid keyId returns 200 with correct JSON structure
- Invalid keyId returns 404
- Invalid period returns 400
- `keyMasked` format: first 8 chars + "..." + last 4 chars of full key
- Response includes: `keyId`, `keyName`, `keyMasked`, `period`, `stats`, `byModel`, `chartData`, `history`
