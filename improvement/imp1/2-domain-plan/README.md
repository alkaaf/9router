# Domain Plan — Usage per User (imp1)

**Project**: imp1 — Per-Key Usage Tracking for 9Router
**Date**: 2026-05-30
**Approach**: Parallel Track (duplication over refactoring)

---

## Status Overview

| Phase | Status | Notes |
|-------|--------|-------|
| Phase 0: Foundation | ✅ Done | `idx_uh_apiKey` index added to schema.js |
| Phase 1: Data Layer | ✅ Done | `getUsageStats`, `getChartData`, `getUsageHistory` extended with `filter.apiKey` |
| Phase 2: API Layer | ✅ Done | 3 new endpoints: `/per-key/[keyId]`, `/chart`, `/history` |
| Phase 3: UI Layer | ✅ Done | Sidebar accordion + PerKeyUsagePage with chart/table/KPI |
| Phase 4: Real-time (SSE) | ⏳ Pending | Optional — `?apiKey=` query param on stream endpoint |
| Phase 5: Testing | ⏳ Pending | Unit, integration, and API tests |

**Implementation complete. Testing phase pending.**

---

## Domain Breakdown

### 1. Backend (Data Layer + API)

**Responsibility**: Data filtering logic and API endpoints for per-key usage.

#### Tasks

| # | Task | Description | File | Status |
|---|------|-------------|------|--------|
| be-1 | Add `idx_uh_apiKey` index | Add index to `usageHistory` table in `schema.js` for perf when filtering by apiKey | `src/lib/db/schema.js` | ✅ Done |
| be-2 | Extend `getUsageStats()` with `filter.apiKey` | Add optional `filter` parameter. Daily summary: `if (filterApiKey && ak.apiKey !== filterApiKey) continue;`. Live history: `AND apiKey = ?` in SQL + JS skip. Overlay: conditional filter. | `src/lib/db/repos/usageRepo.js` | ✅ Done |
| be-3 | Extend `getChartData()` with `filter.apiKey` | Today/24h: add `AND apiKey = ?` to SQL. 7d/30d/60d: parse `dayData.byApiKey` and sum entries matching `filterApiKey`. | `src/lib/db/repos/usageRepo.js` | ✅ Done |
| be-4 | Extend `getUsageHistory()` with `filter.apiKey` | Add 2 lines: `if (filter.apiKey) { conds.push("apiKey = ?"); params.push(filter.apiKey); }` | `src/lib/db/repos/usageRepo.js` | ✅ Done |
| be-5 | API: GET `/api/usage/per-key/[keyId]` | Resolve `keyId` → `getApiKeyById()` → parallel fetch stats + chartData + history → return `{ keyId, keyName, keyMasked, period, stats, byModel, chartData, history }` | `src/app/api/usage/per-key/[keyId]/route.js` | ✅ Done |
| be-6 | API: GET `/api/usage/per-key/[keyId]/chart` | Resolve `keyId` → validate period → `getChartData(period, { apiKey: key.key })` → return `{ keyId, period, chartData }` | `src/app/api/usage/per-key/[keyId]/chart/route.js` | ✅ Done |
| be-7 | API: GET `/api/usage/per-key/[keyId]/history` | Resolve `keyId` → `getUsageHistory({ apiKey: key.key, limit, offset })` → return `{ keyId, history, limit, offset }` | `src/app/api/usage/per-key/[keyId]/history/route.js` | ✅ Done |

**Dependencies**: be-1 → be-2 → be-3 → be-4 → be-5 → be-6, be-7

#### Input/Output

| Input | Output |
|-------|--------|
| `usageRepo.js` (existing functions) | Extended functions with backward-compatible `filter.apiKey` parameter |
| `apiKeysRepo.js` `getApiKeyById()` | UUID → full key resolution |
| 3 new API route files | JSON responses for per-key stats, chart, and history |

---

### 2. Frontend (Dashboard)

**Responsibility**: UI for per-key usage page and sidebar navigation.

#### Tasks

| # | Task | Description | File | Status |
|---|------|-------------|------|--------|
| fe-1 | Sidebar accordion | Add "Usage per User" accordion with expand/collapse state. Fetch `/api/keys` on open. Render API key list with links to `/dashboard/usage-per-key/[keyId]`. Follow Media Providers accordion pattern. | `src/shared/components/Sidebar.js` | ✅ Done |
| fe-2 | Per-key page shell | Create `PerKeyUsagePage` client component with period selector (SegmentedControl), loading state (CardSkeleton), error state. | `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` | ✅ Done |
| fe-3 | OverviewCards integration | Reuse existing `OverviewCards` component with `stats` prop from API response. | `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` | ✅ Done |
| fe-4 | PerKeyChart component | Recharts AreaChart with tokens/cost toggle. Fetch from `/api/usage/per-key/[keyId]/chart`. ResponsiveContainer, gradient fill, tooltip. | `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` | ✅ Done |
| fe-5 | UsageTable integration | Reuse existing `UsageTable` with grouped by-model data. Sortable columns (requests, cost, tokens). Custom summary/detail cells. `storageKey` for expanded state persistence. | `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js` | ✅ Done |
| fe-6 | Styling & responsive | Match existing design system (Card, SegmentedControl, text colors). Test dark/light mode. Mobile-friendly layout. | CSS/Tailwind classes | ✅ Done |

**Dependencies**: fe-1 (independent) → fe-2 → fe-3, fe-4, fe-5 → fe-6

#### Input/Output

| Input | Output |
|-------|--------|
| `Sidebar.js` (existing accordion pattern) | Extended sidebar with "Usage per User" section |
| `OverviewCards`, `UsageTable` (existing components) | Reused via props — no modification needed |
| API responses from be-5, be-6, be-7 | Rendered stats, chart, table |

---

### 3. Database

**Responsibility**: Schema index for per-key query performance.

#### Tasks

| # | Task | Description | File | Status |
|---|------|-------------|------|--------|
| db-1 | Add `idx_uh_apiKey` index | Add `"CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)"` to indexes array. Auto-runs on next app boot via `syncSchemaFromTables()`. | `src/lib/db/schema.js` | ✅ Done |
| db-2 | Verify additive migration | Confirm `syncSchemaFromTables()` picks up new index without data loss. Existing `byApiKey` JSON data already contains per-key entries — no data migration needed. | `src/lib/db/migrations.js` (existing) | ✅ Done |

**Dependencies**: db-1 → db-2

#### Input/Output

| Input | Output |
|-------|--------|
| `schema.js` indexes array | New index created on next boot |
| Existing `usageDaily.byApiKey` JSON | Already contains per-key data — no migration needed |

---

### 4. API Contract

**Responsibility**: Request/response schemas for per-key endpoints.

#### Endpoints

##### `GET /api/usage/per-key/[keyId]?period=7d`

**Request**:
- Path: `keyId` (UUID string)
- Query: `period` (enum: `today`, `24h`, `7d`, `30d`, `60d`, default: `7d`)

**Response (200)**:
```json
{
  "keyId": "uuid",
  "keyName": "Production API Key",
  "keyMasked": "sk-abc1...xyz4",
  "period": "7d",
  "stats": {
    "totalRequests": 1234,
    "totalPromptTokens": 500000,
    "totalCompletionTokens": 200000,
    "totalCost": 45.67
  },
  "byModel": [
    {
      "name": "gpt-4|openai",
      "provider": "openai",
      "requests": 500,
      "promptTokens": 200000,
      "completionTokens": 80000,
      "cost": 25.00,
      "lastUsed": "2026-05-29T10:00:00Z"
    }
  ],
  "chartData": [
    { "label": "May 23", "tokens": 15000, "cost": 2.50 },
    { "label": "May 24", "tokens": 18000, "cost": 3.00 }
  ],
  "history": [
    {
      "id": 1,
      "timestamp": "2026-05-29T10:00:00Z",
      "model": "gpt-4",
      "provider": "openai",
      "promptTokens": 100,
      "completionTokens": 50,
      "cost": 0.05,
      "connectionId": "conn-123"
    }
  ]
}
```

**Errors**:
- `404`: API key not found
- `400`: Invalid period
- `500`: Server error

---

##### `GET /api/usage/per-key/[keyId]/chart?period=7d`

**Request**:
- Path: `keyId` (UUID string)
- Query: `period` (enum: `today`, `24h`, `7d`, `30d`, `60d`)

**Response (200)**:
```json
{
  "keyId": "uuid",
  "period": "7d",
  "chartData": [
    { "label": "May 23", "tokens": 15000, "cost": 2.50 },
    { "label": "May 24", "tokens": 18000, "cost": 3.00 }
  ]
}
```

**Errors**: Same as above.

---

##### `GET /api/usage/per-key/[keyId]/history?limit=50&offset=0`

**Request**:
- Path: `keyId` (UUID string)
- Query: `limit` (int, max 200, default 50), `offset` (int, default 0)

**Response (200)**:
```json
{
  "keyId": "uuid",
  "history": [...],
  "limit": 50,
  "offset": 0
}
```

**Errors**: Same as above.

---

### 5. Backend Unit Testing

**Responsibility**: Test data layer functions in isolation.

#### Tasks

| # | Task | Description | Target | Status |
|---|------|-------------|--------|--------|
| but-1 | Test `getUsageStats()` with `filter.apiKey` | Verify daily summary path filters correctly. Test with matching key, non-matching key, null filter (all data). Verify totals recalculate correctly. | `getUsageStats` | ⏳ Pending |
| but-2 | Test `getChartData()` with `filter.apiKey` | Verify today/24h path adds `AND apiKey = ?`. Verify 7d/30d/60d path parses `byApiKey` JSON correctly. Test edge cases: no data for key, all keys have data. | `getChartData` | ⏳ Pending |
| but-3 | Test `getUsageHistory()` with `filter.apiKey` | Verify SQL condition `apiKey = ?` is added. Test with limit/offset pagination. Verify empty result for non-existent key. | `getUsageHistory` | ⏳ Pending |
| but-4 | Test `getApiKeyById()` resolution | Verify UUID → full key string resolution. Test with invalid UUID (returns null/undefined). | `apiKeysRepo.js` | ⏳ Pending |
| but-5 | Test schema index creation | Verify `idx_uh_apiKey` is created in `schema.js`. Confirm additive migration doesn't drop existing indexes. | `schema.js` | ⏳ Pending |

**Dependencies**: but-1, but-2, but-3 depend on be-2, be-3, be-4. but-4, but-5 independent.

#### Input/Output

| Input | Output |
|-------|--------|
| Test database (in-memory SQLite) | Isolated test cases with known data |
| `usageRepo.js` functions | Verified output matching expected per-key data |

---

### 6. Frontend Testing

**Responsibility**: Test UI components and page rendering.

#### Tasks

| # | Task | Description | Target | Status |
|---|------|-------------|--------|--------|
| fet-1 | Test PerKeyUsagePage renders | Verify page loads with loading skeleton → data display. Mock API responses. Test error state (404, 500). | `page.js` | ⏳ Pending |
| fet-2 | Test period selector | Verify SegmentedControl changes period and triggers re-fetch. Test all 5 periods. Verify URL sync. | `page.js` | ⏳ Pending |
| fet-3 | Test sort functionality | Verify click on table headers toggles sort. Test asc/desc toggle. Verify data re-sorts correctly. | `page.js` | ⏳ Pending |
| fet-4 | Test Sidebar accordion | Verify accordion expand/collapse. Verify API keys list loads on expand. Verify navigation to per-key page. | `Sidebar.js` | ⏳ Pending |
| fet-5 | Test chart view mode toggle | Verify Tokens/Cost toggle switches chart data. Verify gradient, tooltip, responsive container. | `PerKeyChart` in `page.js` | ⏳ Pending |
| fet-6 | Test responsive layout | Verify mobile layout (stacked), desktop layout (horizontal). Test Card, SegmentedControl responsiveness. | `page.js` + `Sidebar.js` | ⏳ Pending |

**Dependencies**: fet-1 → fet-2, fet-3, fet-5 → fet-6. fet-4 independent.

#### Input/Output

| Input | Output |
|-------|--------|
| Mock API responses | Component renders correctly |
| User interactions (click, toggle) | State updates, re-renders, navigation |

---

### 7. API Testing

**Responsibility**: End-to-end testing of API endpoints.

#### Tasks

| # | Task | Description | Target | Status |
|---|------|-------------|--------|--------|
| api-1 | Test `GET /api/usage/per-key/[keyId]` | Test valid keyId → 200 with correct data structure. Test invalid keyId → 404. Test invalid period → 400. Verify `keyMasked` format (first 8 + last 4 chars). | `route.js` | ⏳ Pending |
| api-2 | Test `GET /api/usage/per-key/[keyId]/chart` | Test all 5 periods return valid chartData. Verify `chartData` array has `label`, `tokens`, `cost` fields. Test invalid period → 400. | `chart/route.js` | ⏳ Pending |
| api-3 | Test `GET /api/usage/per-key/[keyId]/history` | Test pagination (limit/offset). Verify `limit` capped at 200. Test offset pagination. Verify history items have required fields. | `history/route.js` | ⏳ Pending |
| api-4 | Test data accuracy | Compare per-key API response data against direct database query for same key. Verify totals match. Verify `byModel` breakdown sums to total. | All 3 endpoints | ⏳ Pending |
| api-5 | Test edge cases | Key with zero usage → empty arrays but valid structure. Key deleted mid-request → 404. Very large period ("60d") → response within acceptable time. | All 3 endpoints | ⏳ Pending |

**Dependencies**: api-1, api-2, api-3 → api-4 → api-5

#### Input/Output

| Input | Output |
|-------|--------|
| HTTP requests to per-key endpoints | Correct JSON responses with expected schema |
| Database state (known keys, known usage) | API response matching ground truth |

---

## Execution Order

```
Phase 0 (db-1, db-2)     ──→ Done
    ↓
Phase 1 (be-2..be-4)     ──→ Done
    ↓
Phase 2 (be-5..be-7)     ──→ Done
    ↓
Phase 3 (fe-1..fe-6)     ──→ Done
    ↓
Phase 5: Testing
    ├── Backend Unit (but-1..but-5)
    ├── Frontend (fet-1..fet-6)
    └── API (api-1..api-5)
        ↓
  Bug fix loop (if failures)
    ├── Create atomic task in 3-atomic-task/
    ├── Fix
    └── Re-test
        ↓
  All pass → Done
```

---

## Key Decisions (from Strategic Brief)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Approach | **Duplication over refactoring** | Zero regression risk. Existing system untouched. |
| Filtering strategy | Extend existing functions with `filter.apiKey` | Minimal changes (~20 lines), backward compatible |
| API key in URL | **keyId (UUID)** not full key | Security — no keys in browser history |
| Data source | `usageDaily.byApiKey` for aggregations | Already contains per-key data, no migration needed |
| UI reuse | `OverviewCards`, `UsageTable` | Don't rebuild working components |
| SSE | Optional (Phase 4, skipped for MVP) | Core functionality doesn't depend on real-time |

---

## Reference Files

| Domain | Key Files |
|--------|-----------|
| Backend | `src/lib/db/repos/usageRepo.js`, `src/lib/db/repos/apiKeysRepo.js` |
| Frontend | `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js`, `src/shared/components/Sidebar.js` |
| Database | `src/lib/db/schema.js` |
| API | `src/app/api/usage/per-key/[keyId]/*/route.js` |
| Analysis | `improvement/imp1/analysis/analysis-9-data-layer-filtering.md` |
| Strategy | `improvement/imp1/1-strategic/README.md` |
