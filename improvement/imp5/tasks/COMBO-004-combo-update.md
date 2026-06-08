---
id: COMBO-004
domain: combos
status: TODO
estimate: 1h
title: Implement GET /api/combos Handler
---

## Description

Fiber handler `ListCombos` in `internal/handler/api/combos.go` that calls `comboRepo.FindAll()` and returns `{"combos": [...]}` with status 200. Use the shared `response.JSON` helper. Errors return `500 { "error": "Failed to fetch combos" }` matching the Node.js message.

## Input

HTTP GET request to `/api/combos`, no body.

## Output

- **200 OK:** `{"combos":[{"id":"...","name":"...","kind":...,"models":[],"createdAt":"...","updatedAt":"..."},...]}`
- **500 Error:** `{"error":"Failed to fetch combos"}`

## Logic

1. Call `comboRepo.FindAll()`
2. On success: return 200 with `{"combos": [...]}`
3. On error: return 500 with `"Failed to fetch combos"`

## Acceptance Criteria
- [ ] Returns 200 with empty `combos: []` when DB is empty
- [ ] Returns 200 with all combos in `createdAt ASC` order
- [ ] Returns 500 with exact error message on DB error
- [ ] Uses shared `response.JSON` helper
- [ ] Response format matches Node.js exactly

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Empty DB | GET /api/combos | 200 `{"combos":[]}` |
| 3 seeded combos | GET /api/combos | 200 with 3 combos in order |
| DB error | GET /api/combos | 500 `{"error":"Failed to fetch combos"}` |
| Single combo | GET /api/combos | 200 with single combo |