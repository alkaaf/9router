# Atomic Task: ac-1 — Document API Contract for `GET /api/usage/per-key/[keyId]`

**Domain**: API Contract
**Priority**: High
**Estimated effort**: 15 min

---

## Input

- `src/app/api/usage/per-key/[keyId]/route.js` — existing implementation
- `src/lib/db/repos/usageRepo.js` — `getUsageStats()`, `getChartData()`, `getUsageHistory()`
- `src/lib/db/repos/apiKeysRepo.js` — `getApiKeyById()`

## Output

- Complete API contract specification for the main per-key endpoint
- Document covers: request params, response schema, error codes, examples

## Process

### Step 1: Document request specification

```
GET /api/usage/per-key/{keyId}?period=7d

Path Parameters:
  keyId  (string, required) — UUID of the API key from apiKeys table
                           Example: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

Query Parameters:
  period (string, optional) — Time range for data aggregation
              Default: "7d"
              Enum: "today" | "24h" | "7d" | "30d" | "60d" | "all"

Headers:
  Content-Type: application/json
```

### Step 2: Document response schema

```typescript
// Success response (200 OK)
{
  keyId: string,        // UUID of the API key
  keyName: string,      // Display name from apiKeys table
  keyMasked: string,    // Masked key: first 8 chars + "..." + last 4 chars
                        // Example: "sk-abc1234...efgh"
  period: string,       // The period used for aggregation
                        // Example: "7d"

  stats: {
    totalRequests: number,           // Sum of all requests for this key
    totalPromptTokens: number,       // Sum of all input/prompt tokens
    totalCompletionTokens: number,   // Sum of all output/completion tokens
    totalCost: number                // Sum of all costs (USD)
  },

  byModel: Array<{
    name: string,          // Model identifier, format: "modelName|providerName"
    provider: string,      // Provider name (e.g., "openai", "anthropic")
    requests: number,      // Request count for this model+provider combo
    promptTokens: number,  // Input tokens
    completionTokens: number, // Output tokens
    cost: number,          // Cost in USD
    lastUsed: string       // ISO timestamp of most recent usage
                           // Example: "2026-05-29T10:00:00Z"
  }>,

  chartData: Array<{
    label: string,     // Time bucket label
                      // Examples: "May 23", "10:00", "Today"
    tokens: number,   // Total tokens (prompt + completion) for this bucket
    cost: number      // Total cost for this bucket
  }>,

  history: Array<{
    id: number,            // Row ID from usageHistory
    timestamp: string,     // ISO timestamp
                          // Example: "2026-05-29T10:00:00Z"
    model: string,         // Model name
    provider: string,      // Provider name
    connectionId: string,  // Connection identifier
    promptTokens: number,  // Input tokens for this request
    completionTokens: number, // Output tokens for this request
    cost: number,          // Cost for this request
    tokens: number         // Total tokens (prompt + completion)
  }>
}
```

### Step 3: Document error responses

```typescript
// 400 Bad Request — Invalid period
{ error: "Invalid period" }

// 404 Not Found — API key does not exist
{ error: "API key not found" }

// 500 Internal Server Error — Unexpected failure
{ error: "Failed to fetch per-key usage" }
```

### Step 4: Document data sources

| Field | Source | Notes |
|-------|--------|-------|
| `stats.totalRequests` | `usageDaily.byApiKey` (7d+) or `usageHistory` (today/24h) | Filtered by apiKey |
| `stats.totalPromptTokens` | Same as above | Sum of promptTokens |
| `stats.totalCompletionTokens` | Same as above | Sum of completionTokens |
| `stats.totalCost` | Same as above | Sum of cost |
| `byModel` | `stats.byModel` filtered to single key | Derived from byApiKey entries |
| `chartData` | `getChartData(period, { apiKey })` | Separate query |
| `history` | `getUsageHistory({ apiKey })` | Last 50 entries (default) |

### Step 5: Document frontend consumers

| Consumer | File | Usage |
|----------|------|-------|
| `PerKeyUsagePage` | `usage-per-key/[keyId]/page.js` | Fetches all data on mount |
| `PerKeyChart` | Same file | Uses `chartData` from response (may re-fetch separately) |
| `UsageTable` | Same file | Uses `byModel` from response |

## Dependencies

- be-5: API route implementation (DONE)

## Success Criteria

- Contract documented with complete type definitions
- All response fields mapped to data sources
- Error codes documented with triggers
- Frontend consumers listed with their data usage
