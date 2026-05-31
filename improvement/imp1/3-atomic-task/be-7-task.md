# Atomic Task: be-7 — Create API Route `GET /api/usage/per-key/[keyId]/history`

**Domain**: Backend (API Layer)
**Priority**: Medium
**Estimated effort**: 20 min

---

## Input

- `src/lib/usageDb.js` — `getUsageHistory(filter)` with pagination support
- `src/lib/db/repos/apiKeysRepo.js` — `getApiKeyById(id)`
- File path: `src/app/api/usage/per-key/[keyId]/history/route.js` (create new)

## Output

- New API route file for history endpoint with pagination
- Returns `{ keyId, history, limit, offset }`
- `limit` capped at 200

## Process

### Step 1: Create directory

```bash
mkdir -p src/app/api/usage/per-key/\[keyId\]/history
```

### Step 2: Create route file

**File**: `src/app/api/usage/per-key/[keyId]/history/route.js`

```js
import { NextResponse } from "next/server";
import { getUsageHistory } from "@/lib/usageDb";
import { getApiKeyById } from "@/lib/db/repos/apiKeysRepo";

export const dynamic = "force-dynamic";

export async function GET(request, { params }) {
  try {
    const { keyId } = await params;
    const { searchParams } = new URL(request.url);
    const limit = Math.min(parseInt(searchParams.get("limit") || "50", 10), 200);
    const offset = parseInt(searchParams.get("offset") || "0", 10);

    const key = await getApiKeyById(keyId);
    if (!key) {
      return NextResponse.json({ error: "API key not found" }, { status: 404 });
    }

    const history = await getUsageHistory({ apiKey: key.key, limit, offset });
    return NextResponse.json({ keyId: key.id, history, limit, offset });
  } catch (error) {
    console.error("[API] Failed to get per-key history:", error);
    return NextResponse.json({ error: "Failed to fetch per-key history" }, { status: 500 });
  }
}
```

## Dependencies

- be-4: `getUsageHistory()` with filter (DONE)

## Success Criteria

- Valid keyId → 200 with `{ keyId, history, limit, offset }`
- Invalid keyId → 404
- `limit` capped at 200 (e.g., `?limit=500` → returns max 200)
- `offset` enables pagination
- Default `limit=50`, default `offset=0`
- Each history item has: `id`, `timestamp`, `model`, `provider`, `promptTokens`, `completionTokens`, `cost`, `connectionId`
