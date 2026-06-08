---
id: COMBO-005
domain: combos
status: TODO
estimate: 1h
title: Implement POST /api/combos Handler
---

## Description

Fiber handler `CreateCombo` that:
1. Parses JSON body `{name, models, kind}`.
2. Returns 400 with `"Name is required"` if `name` missing.
3. Validates `name` against `^[a-zA-Z0-9_.\-]+$`; on failure returns 400 with the Node.js message `"Name can only contain letters, numbers, -, _ and ."`.
4. Calls `comboRepo.FindByName(name)`; if found returns 400 with `"Combo name already exists"`.
5. Persists via `Create` and returns 201 with the new combo (not wrapped).
6. On unexpected error returns 500 `"Failed to create combo"`.

## Input

`{"name":"my-combo","models":["gpt-4o","claude-sonnet-4-5"],"kind":null}`

## Output

- **201 Created:** the new combo object
- **400 Bad Request:** `{"error":"<message>"}` (three distinct messages)
- **500 Error:** `{"error":"Failed to create combo"}`

## Logic

1. Parse JSON body
2. If `name` missing/empty: return 400 `"Name is required"`
3. Validate name regex `^[a-zA-Z0-9_.\-]+$`; fail: 400 `"Name can only contain letters, numbers, -, _ and ."`
4. Check `FindByName(name)`; if found: 400 `"Combo name already exists"`
5. Create combo via repo
6. Return 201 with created combo
7. On unexpected error: 500 `"Failed to create combo"`

## Acceptance Criteria
- [ ] Missing name returns 400 `"Name is required"`
- [ ] Invalid name (space, slash, unicode) returns 400 with regex message
- [ ] Duplicate name returns 400 `"Combo name already exists"`
- [ ] Valid request with empty models returns 201, persisted as `[]`
- [ ] Valid request with `kind` persists and returns `kind`
- [ ] DB error returns 500 `"Failed to create combo"`

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Missing name | `{}` | 400 `"Name is required"` |
| Invalid name (space) | `{"name":"my combo"}` | 400 `"Name can only contain..."` |
| Invalid name (slash) | `{"name":"my/combo"}` | 400 `"Name can only contain..."` |
| Invalid name (unicode) | `{"name":"my combo"}` | 400 `"Name can only contain..."` |
| Duplicate name | `{"name":"existing"}` | 400 `"Combo name already exists"` |
| Valid, empty models | `{"name":"valid","models":[]}` | 201, models stored as `[]` |
| Valid with kind | `{"name":"valid","models":["a"],"kind":"test"}` | 201, kind persisted |
| DB error | Valid input, DB fails | 500 `"Failed to create combo"` |