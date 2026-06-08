---
id: SYS-012
domain: settings
status: DONE
estimate: 2h
title: GET/PUT /api/models — List models with aliases (legacy)
---

## Description

Two endpoints that manage the model list with legacy alias support. GET returns the AI_MODELS list filtered to exclude disabled models, with aliases from the kv table applied. PUT sets a model alias, checking for duplicates across different models.

## Input

**GET:** None.

**PUT:**
```json
{ "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

## Output

**GET:**
```json
{
  "models": [
    { "provider": "openai", "model": "gpt-4o", "fullModel": "openai/gpt-4o", "alias": "GPT-4o" }
  ]
}
```

**PUT:**
```json
{ "success": true, "model": "openai/gpt-4o", "alias": "GPT-4o" }
```

## Logic

### GET
1. Load the base model list from config (`AI_MODELS` or equivalent).
2. Load disabled models from the `kv` table (`disabledModels` key, per-provider map).
3. Filter out any model whose `provider/model` appears in the disabled list.
4. Load model aliases from the `kv` table (`modelAliases` key, map of `provider/model` → alias).
5. For each remaining model, enrich with:
   - `fullModel`: `"<provider>/<model>"`
   - `alias`: the alias from the map, or the model name itself if no alias is set.
6. Return the enriched list.

### PUT
1. Parse request body — require `model` (`provider/model` format) and `alias`.
2. Check if the alias already maps to a *different* model in the aliases map.
   - If alias maps to the same model, treat as idempotent (return success).
   - If alias maps to a different model, return 400 with duplicate error.
3. Set the alias in the `kv` table under `modelAliases`.
4. Return success response.

## Acceptance Criteria

- [ ] `GET /api/models` returns 200 with enriched model list
- [ ] Disabled models are excluded from the list
- [ ] Each model has `provider`, `model`, `fullModel`, and `alias` fields
- [ ] Missing alias falls back to the model name as alias value
- [ ] `PUT /api/models` sets an alias for a model
- [ ] Duplicate alias for a different model returns 400
- [ ] Setting the same alias for the same model is idempotent (returns 200)
- [ ] Aliases are persisted in the `kv` table under `modelAliases`
- [ ] GET reflects the newly set alias immediately

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| List models | GET /api/models | 200, all non-disabled models with aliases |
| Disabled excluded | Model in disabledModels list | Not present in GET response |
| No alias set | Model has no alias entry | `alias` = model name |
| Alias set | PUT `{ "model": "o/gpt-4o", "alias": "GPT-4o" }` | 200, alias persisted |
| Duplicate alias | PUT alias already used by different model | 400, duplicate error |
| Same model same alias | PUT same alias for same model | 200, idempotent |
| Missing fields | PUT without `model` or `alias` | 400 |
