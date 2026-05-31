# Atomic Task: fe-2 — Create PerKeyUsagePage Shell

**Domain**: Frontend
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- Next.js 16 App Router, dynamic route `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js`
- API endpoint: `GET /api/usage/per-key/[keyId]?period=7d`
- Shared components: `CardSkeleton`, `SegmentedControl` from `@/shared/components`

## Output

- New page component `PerKeyUsagePage` with:
  - Period selector (SegmentedControl)
  - Loading state (CardSkeleton)
  - Error state
  - Header with key name + masked key

## Process

### Step 1: Create directory

```bash
mkdir -p src/app/\(dashboard\)/dashboard/usage-per-key/\[keyId\]
```

### Step 2: Create page.js

**File**: `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js`

```jsx
"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useSearchParams } from "next/navigation";
import { CardSkeleton, SegmentedControl, Card } from "@/shared/components";

const PERIODS = [
  { value: "today", label: "Today" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7D" },
  { value: "30d", label: "30D" },
  { value: "60d", label: "60D" },
];

export default function PerKeyUsagePage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const keyId = params.keyId;

  const urlPeriod = searchParams.get("period") || "7d";
  const [period, setPeriod] = useState(urlPeriod);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => { setPeriod(urlPeriod); }, [urlPeriod]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`/api/usage/per-key/${keyId}?period=${period}`);
      if (res.ok) setData(await res.json());
    } catch {}
    finally { setLoading(false); }
  }, [keyId, period]);

  useEffect(() => { fetchData(); }, [fetchData]);

  if (loading) return <CardSkeleton />;
  if (!data) return <div className="text-text-muted text-sm">Failed to load usage data.</div>;

  return (
    <div className="flex min-w-0 flex-col gap-6 px-1 sm:px-0">
      {/* Header */}
      <div className="flex flex-col gap-1">
        <h1 className="text-xl font-bold">Usage per User</h1>
        <p className="text-sm text-text-muted">
          {data.keyName || "Unnamed Key"} &middot; <code className="text-xs bg-surface-2 px-1.5 py-0.5 rounded">{data.keyMasked}</code>
        </p>
      </div>

      {/* Period selector */}
      <SegmentedControl options={PERIODS} value={period} onChange={setPeriod} className="w-full sm:w-auto" />

      {/* Content slots for KPI cards, chart, table — added in fe-3, fe-4, fe-5 */}
    </div>
  );
}
```

## Dependencies

- fe-1: Sidebar accordion (DONE) — provides navigation to this page

## Success Criteria

- Page loads at `/dashboard/usage-per-key/[keyId]`
- Loading state shows CardSkeleton
- Error state shows error message
- Header displays key name and masked key
- Period selector works with 5 options
- Period syncs with URL query param `?period=`
