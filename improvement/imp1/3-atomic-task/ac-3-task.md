# Atomic Task: ac-3 — Document API Contract for `GET /api/usage/per-key/[keyId]/history`

**Domain**: API Contract
**Priority**: Medium
**Estimated effort**: 10 min

---

## Input

- `src/app/api/usage/per-key/[keyId]/history/route.js` — existing implementation
- `src/lib/db/repos/usageRepo.js` — `getUsageHistory(filter)` with pagination

## Output

- API contract specification for the history endpoint
- Covers pagination, request params, response schema

## Process

### Step 1: Document request specification

```
GET /api/usage/per-key/{keyId}/history?limit=50&offset=0

Path Parameters:
  keyId  (string, required) — UUID of the API key

Query Parameters:
  limit  (int, optional) — Max rows to return
             Default: 50
             Max: 200
             Min: 1
  offset (int, optional) — Number of rows to skip (for pagination)
             Default: 0

Headers:
  Content-Type: application/json
```

### Step 2: Document response schema

```typescript
// Success response (200 OK)
{
  keyId: string,      // UUID of the API key
  history: Array<{
    id: number,           // Row ID from usageHistory (auto-increment)
    timestamp: string,    // ISO 8601 timestamp
                          // Example: "2026-05-29T10:00:00Z"
    model: string,        // Model name
                          // Example: "gpt-4", "claude-3-opus"
    provider: string,     // Provider name
                          // Example: "openai", "anthropic"
    connectionId: string, // Connection identifier
                          // Example: "conn-abc123"
    promptTokens: number, // Input/prompt tokens
    completionTokens: number, // Output/completion tokens
    cost: number,         // Cost in USD
                          // Example: 0.05
    tokens: number        // Total tokens (prompt + completion)
  }>,
  limit: number,    // The limit that was applied
                    // Example: 50
  offset: number    // The offset that was applied
                    // Example: 0
}
```

### Step 3: Document pagination behavior

| Parameter | Behavior |
|-----------|----------|
| `limit=50` (default) | Returns up to 50 most recent entries |
| `limit=200` (max) | Returns up to 200 entries |
| `limit=500` | Capped to 200 |
| `offset=0` (default) | Starts from beginning |
| `offset=50` | Skips first 50, returns entries 51-100 |
| `offset=10000` | Returns empty array if beyond data range |

**Ordering**: Results ordered by `id ASC` (oldest first) — chronological order.

### Step 4: Document filtering

- All results filtered by `apiKey = ?` (resolved from keyId UUID)
- Additional filters from `usageHistory` columns not available here
- Returns only entries for the specified API key

### Step 5: Document error responses

```typescript
// 404 Not Found — API key does not exist
{ error: "API key not found" }

// 500 Internal Server Error
{ error: "Failed to fetch per-key history" }
```

### Step 6: Document frontend consumers

| Consumer | File | Usage |
|----------|------|-------|
| `PerKeyUsagePage` initial render | `usage-per-key/[keyId]/page.js` | Receives `history` from main `/per-key/[keyId]` endpoint (first 50) |
| History pagination (future) | Same file | Would call `/history` endpoint with offset for "load more" |

**Note**: The main endpoint (`/api/usage/per-key/[keyId]`) also returns `history` (default 50 entries). The `/history` sub-endpoint exists for pagination — when the UI needs more than 50 entries, it calls `/history?limit=200&offset=50`.

## Dependencies

- be-7: History API route (DONE)

## Success Criteria

- Contract documented with request/response schema
- Pagination rules documented (limit cap, offset behavior)
- Ordering documented (id ASC = chronological)
- Frontend consumers listed
