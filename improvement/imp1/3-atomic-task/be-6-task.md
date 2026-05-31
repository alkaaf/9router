# Atomic Task: be-6 — Create API Route `GET /api/usage/per-key/[keyId]/chart`

**Domain**: Backend (API Layer)
**Priority**: Medium
**Estimated effort**: 20 min

---

## Input

- `src/lib/usageDb.js` — `getChartData(period, filter)`
- `src/lib/db/repos/apiKeysRepo.js` — `getApiKeyById(id)`
- File path: `src/app/api/usage/per-key/[keyId]/chart/route.js` (create new)

## Output

- New API route file for chart data endpoint
- Returns `{ keyId, period, chartData }` for a single API key

## Process

### Step 1: Create directory

```bash
mkdir -p src/app/api/usage/per-key/\[keyId\]/chart
```

### Step 2: Create route file

**File**: `src/app/api/usage/per-key/[keyId]/chart/route.js`

```js
import { NextResponse } from "next/server";
import { getChartData } from "@/lib/usageDb";
import { getApiKeyById } from "@/lib/db/repos/apiKeysRepo";

const VALID_PERIODS = new Set(["today", "24h", "7d", "30d", "60d"]);

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

    const chartData = await getChartData(period, { apiKey: key.key });
    return NextResponse.json({ keyId: key.id, period, chartData });
  } catch (error) {
    console.error("[API] Failed to get per-key chart:", error);
    return NextResponse.json({ error: "Failed to fetch per-key chart" }, { status: 500 });
  }
}
```

## Dependencies

- be-3: `getChartData()` with filter (DONE)

## Success Criteria

- Valid keyId + period → 200 with `{ keyId, period, chartData }`
- Invalid keyId → 404
- Invalid period → 400
- `chartData` is array of `{ label, tokens, cost }` objects
- Supports all 5 periods: today, 24h, 7d, 30d, 60d
