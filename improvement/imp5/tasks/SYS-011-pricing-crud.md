---
id: SYS-011
domain: settings
status: DONE
estimate: 3h
title: GET/PATCH/DELETE /api/pricing — Pricing configuration CRUD
---

## Description

Three endpoints for managing pricing configuration: GET returns the merged pricing (user overrides + defaults), PATCH merge-updates pricing overrides, and DELETE resets overrides at global/provider/model scope. All data is stored in the `kv` table under the `pricing` key.

## Input

**GET:** None.

**PATCH:**
```json
{
  "openai": {
    "gpt-4o": { "input": 2.5, "output": 10.0 }
  }
}
```

**DELETE:** Query params: `provider` (optional), `model` (optional).

## Output

**GET:**
```json
{
  "openai": {
    "gpt-4o": { "input": 2.5, "output": 10.0, "cached": 1.25, "reasoning": null },
    "gpt-4o-mini": { "input": 0.15, "output": 0.6, "cached": 0.075 }
  },
  "anthropic": {
    "claude-3-5-sonnet": { "input": 3.0, "output": 15.0, "cached": 3.75, "reasoning": 3.75 }
  }
}
```

**PATCH:** Same shape as GET (merged result after patch applied).

**DELETE:** Same shape as GET (pricing after reset).

## Logic

### GET
1. Read the `pricing` key from the `kv` table.
2. Merge user overrides with built-in defaults (user values take precedence).
3. Return the merged pricing object with 200.

### PATCH
1. Parse and validate the request body.
2. Valid fields per model entry: `input`, `output`, `cached`, `reasoning`, `cache_creation`.
3. All values must be non-negative numbers (int or float). Null is allowed for `reasoning` and `cached`.
4. Read current overrides from `kv` table.
5. Deep-merge the patch into existing overrides.
6. Persist the merged overrides back to `kv`.
7. Return the full merged result (same shape as GET).

### DELETE
1. Read query params: `provider` (optional), `model` (optional).
2. Read current overrides from `kv` table.
3. Determine scope:
   - No params → reset all overrides to empty (factory defaults only).
   - `?provider=openai` → remove the entire `openai` provider key.
   - `?provider=openai&model=gpt-4o` → remove only the `gpt-4o` model key under `openai`.
4. Persist the updated overrides.
5. Return the full merged result (same shape as GET).

## Acceptance Criteria

- [ ] `GET /api/pricing` returns 200 with merged pricing (defaults + overrides)
- [ ] `PATCH /api/pricing` validates field names — invalid field returns 400
- [ ] `PATCH /api/pricing` rejects negative values with 400
- [ ] `PATCH /api/pricing` rejects non-number values with 400
- [ ] `PATCH /api/pricing` deep-merges into existing overrides (does not replace)
- [ ] `DELETE /api/pricing` with no params resets all overrides
- [ ] `DELETE /api/pricing?provider=openai` resets only that provider
- [ ] `DELETE /api/pricing?provider=openai&model=gpt-4o` resets only that model
- [ ] All three methods return the full merged pricing object
- [ ] Data is persisted in the `kv` table under the `pricing` key

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| GET pricing | GET /api/pricing | 200, merged pricing object |
| PATCH valid | `{ "openai": { "gpt-4o": { "input": 2.5 } } }` | 200, merged result with override applied |
| PATCH invalid field | `{ "openai": { "gpt-4o": { "invalidField": 1 } } }` | 400, field name in error |
| PATCH negative value | `{ "openai": { "gpt-4o": { "input": -1 } } }` | 400 |
| PATCH non-number | `{ "openai": { "gpt-4o": { "input": "free" } } }` | 400 |
| DELETE all | DELETE /api/pricing | 200, pricing = defaults only |
| DELETE provider | DELETE /api/pricing?provider=openai | 200, openai removed |
| DELETE model | DELETE /api/pricing?provider=openai&model=gpt-4o | 200, only gpt-4o removed |
| Empty overrides | PATCH when no overrides exist | 200, new overrides created |
| Null reasoning | `{ "reasoning": null }` | Accepted, stored as null |
