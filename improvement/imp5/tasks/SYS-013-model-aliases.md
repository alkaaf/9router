---
id: SYS-013
domain: settings
status: DONE
estimate: 2h
title: GET/PUT/DELETE /api/models/alias — Model alias CRUD
---

## Description

Three endpoints for managing model aliases via the dedicated alias API. GET returns all aliases, PUT sets a single alias (with duplicate detection), and DELETE removes an alias by its alias value (not by model).

## Input

**GET:** None.

**PUT:**
```json
{ "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

**DELETE:** Query param: `alias=GPT-4o`.

## Output

**GET:**
```json
{ "aliases": { "openai/gpt-4o": "GPT-4o", "anthropic/claude-3-5-sonnet": "Claude Sonnet" } }
```

**PUT:**
```json
{ "success": true, "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

**DELETE:**
```json
{ "success": true }
```

## Logic

### GET
1. Read the `modelAliases` key from the `kv` table.
2. Return `{ "aliases": <map> }` — the map may be an empty object `{}`.

### PUT
1. Parse request body — require `model` (`provider/model` format) and `alias`.
2. Load existing aliases map from `kv`.
3. Check if the alias already maps to a *different* model:
   - If alias exists and maps to the same model → idempotent, return 200.
   - If alias exists and maps to a different model → return 400.
4. Set `aliases[model] = alias` in the map.
5. Persist back to `kv` table under `modelAliases`.
6. Return success.

### DELETE
1. Require `alias` query parameter.
2. Load existing aliases map from `kv`.
3. Find the model key that maps to the given alias value.
4. Remove that entry from the map.
5. Persist back to `kv`.
6. Return `{ "success": true }`.
7. If `alias` query param is missing, return 400.

## Acceptance Criteria

- [ ] `GET /api/models/alias` returns 200 with alias map (may be empty `{}`)
- [ ] `PUT /api/models/alias` sets an alias and returns 200
- [ ] `PUT` with duplicate alias for different model returns 400
- [ ] `PUT` for same model with same alias is idempotent
- [ ] `DELETE /api/models/alias?alias=xxx` removes the alias and returns 200
- [ ] `DELETE` without `alias` query param returns 400
- [ ] `DELETE` for non-existent alias returns 200 (idempotent) or 404
- [ ] Aliases persisted in `kv` table under `modelAliases`
- [ ] GET reflects changes immediately after PUT/DELETE

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| GET empty | No aliases set | 200, `{ "aliases": {} }` |
| GET with aliases | Aliases exist | 200, `{ "aliases": { ... } }` |
| PUT new alias | `{ "model": "o/gpt-4o", "alias": "GPT-4o" }` | 200, alias set |
| PUT duplicate | Alias already maps to different model | 400 |
| PUT same model | Same model, same alias | 200, idempotent |
| DELETE existing | `?alias=GPT-4o` | 200, alias removed |
| DELETE missing param | No `alias` query param | 400 |
| DELETE non-existent | `?alias=NonExistent` | 200 or 404 |
