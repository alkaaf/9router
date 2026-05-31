# Atomic Task: ac-2 — Document API Contract for `GET /api/usage/per-key/[keyId]/chart`

**Domain**: API Contract
**Priority**: Medium
**Estimated effort**: 10 min

---

## Input

- `src/app/api/usage/per-key/[keyId]/chart/route.js` — existing implementation
- `src/lib/db/repos/usageRepo.js` — `getChartData(period, filter)`

## Output

- API contract specification for the chart endpoint
- Covers request params, response schema, period behavior

## Process

### Step 1: Document request specification

```
GET /api/usage/per-key/{keyId}/chart?period=7d

Path Parameters:
  keyId  (string, required) — UUID of the API key

Query Parameters:
  period (string, optional) — Time range for chart buckets
              Default: "7d"
              Enum: "today" | "24h" | "7d" | "30d" | "60d"

Headers:
  Content-Type: application/json
```

### Step 2: Document response schema

```typescript
// Success response (200 OK)
{
  keyId: string,     // UUID of the API key
  period: string,    // The period used
                     // Example: "7d"
  chartData: Array<{
    label: string,   // Time bucket label
                    // today/24h: "10:00", "11:00", ...
                    // 7d: "May 23", "May 24", ...
                    // 30d: "May 1", "May 2", ...
                    // 60d: "Apr 1", "Apr 2", ...
    tokens: number,  // Total tokens for this bucket (prompt + completion)
    cost: number     // Total cost for this bucket
  }>
}
```

### Step 3: Document period behavior

| Period | Bucket Size | Expected Count | Source |
|--------|-------------|----------------|--------|
| `today` | Hourly | ~24 buckets (since midnight) | `usageHistory` WHERE `timestamp >= today 00:00` |
| `24h` | Hourly | 24 buckets | `usageHistory` WHERE `timestamp >= now - 24h` |
| `7d` | Daily | 7 buckets | `usageDaily` parsed `byApiKey` |
| `30d` | Daily | 30 buckets | `usageDaily` parsed `byApiKey` |
| `60d` | Daily | 60 buckets | `usageDaily` parsed `byApiKey` |

### Step 4: Document filtering

- All queries filter by `apiKey = ?` (resolved from keyId)
- For today/24h: SQL `WHERE timestamp >= ? AND apiKey = ?`
- For 7d/30d/60d: JSON `dayData.byApiKey` entries filtered to matching `apiKey`
- If no data for key in period: returns `chartData` with all zeros

### Step 5: Document error responses

```typescript
// 400 Bad Request — Invalid period
{ error: "Invalid period" }

// 404 Not Found — API key does not exist
{ error: "API key not found" }

// 500 Internal Server Error
{ error: "Failed to fetch per-key chart" }
```

### Step 6: Document frontend consumers

| Consumer | File | Usage |
|----------|------|-------|
| `PerKeyUsagePage` initial render | `usage-per-key/[keyId]/page.js` | Receives `chartData` from main endpoint |
| `PerKeyChart` component | Same file | Fetches separately via `/chart` endpoint on mount + period change |
| `PerKeyChart` view toggle | Same file | Same data, different `viewMode` (tokens vs cost) |

## Dependencies

- be-6: Chart API route (DONE)

## Success Criteria

- Contract documented with request/response schema
- Period-to-bucket mapping documented
- Data source for each period documented
- Filtering behavior documented
