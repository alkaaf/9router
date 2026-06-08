---
id: USAGE-005
domain: usage
status: TODO
estimate: 1h
title: RequestDetails Repository
---

## Description

Implement `RequestDetailsRepository` for observability logging. Handles request/response body storage with sanitization, truncation, and auto-cleanup.

## Input

- RequestDetailInput struct with full request/response data
- RequestDetailsFilter for paginated queries
- Observability config (enabled flag, maxRecords, maxJsonSize)

## Output

- RequestDetailsRepository with save, get, and cleanup methods
- Sanitized JSON data stored in requestDetails table

## Key methods

### 5a. SaveRequestDetail(detail RequestDetailInput)

```
- Input: {id, provider, model, connectionId, timestamp, status, latency, tokens,
          request, providerRequest, providerResponse, response}
- Sanitizes sensitive headers before storing
- Truncates JSON if > maxJsonSize (default 5KB)
- Uses write buffer + periodic flush (FLUSH_INTERVAL_MS=5000, BATCH_SIZE=20)
- Auto-deletes oldest records when total > maxRecords (default 200)
```

### 5b. GetRequestDetails(filter RequestDetailsFilter)

```
- Input: filter {provider, model, connectionId, status, startDate, endDate, page, pageSize}
- pageSize clamped 1-100
- Output: { details: []ParsedDetail, pagination: { page, pageSize, totalItems, totalPages, hasNext, hasPrev } }
- Reads from: requestDetails, ordered by timestamp DESC
```

### 5c. GetRequestDetailById(id string)

```
- Input: detail ID
- Output: full JSON data blob or null
```

## Sensitive header keys to sanitize

- authorization
- x-api-key
- cookie
- token
- api-key

## Logic

1. Implement `sanitizeHeaders()` function that recursively walks the headers object and removes sensitive keys
2. Implement `truncateIfNeeded()` function that checks JSON size and truncates with `_truncated: true` marker
3. Implement auto-cleanup: count total records, delete oldest if count > maxRecords
4. Implement buffered write: collect up to BATCH_SIZE entries, flush every FLUSH_INTERVAL_MS
5. Implement pagination calculation: totalPages = ceil(totalItems / pageSize), hasNext = page < totalPages, hasPrev = page > 1
6. page minimum is 1, pageSize clamped 1-100

## Acceptance Criteria

- [ ] SaveRequestDetail sanitizes all sensitive headers before storage
- [ ] JSON data truncated when exceeding maxJsonSize (5KB)
- [ ] Auto-cleanup removes oldest records when count > maxRecords (200)
- [ ] GetRequestDetails returns paginated results with correct metadata
- [ ] Pagination clamps pageSize to 1-100 range
- [ ] Write buffer batches up to 20 entries per flush
- [ ] GetRequestDetailById returns full data blob for valid ID

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Sanitize authorization | request.headers.authorization = "Bearer sk-xxx" | Field absent from stored data |
| Sanitize multiple keys | All 5 sensitive keys present | All removed from stored data |
| JSON truncation | JSON data = 10KB | Truncated to 5KB with marker |
| Auto-cleanup trigger | Insert 300 records (max=200) | Oldest 100 deleted |
| Auto-cleanup no-op | Insert 100 records (max=200) | All 100 remain |
| Pagination page 1 | 150 total items, pageSize=20, page=1 | hasNext=true, hasPrev=false, totalPages=8 |
| Pagination page 5 | 150 total items, pageSize=20, page=5 | hasNext=true, hasPrev=true, totalPages=8 |
| Pagination last page | 150 total items, pageSize=20, page=8 | hasNext=false, hasPrev=true, totalPages=8 |
| Filter by provider | provider="openai" filter | Only openai records returned |
| GetRequestDetailById exists | Valid ID | Full JSON data returned |
| GetRequestDetailById not found | Invalid ID | nil, no error |
| Buffer flush | 20 entries accumulated | Flushed to DB automatically |
