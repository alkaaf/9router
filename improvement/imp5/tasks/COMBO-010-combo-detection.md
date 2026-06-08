---
id: COMBO-010
domain: combos
status: TODO
estimate: 2h
title: Implement Combo Detection in Chat Handler
---

## Description

In `internal/handler/v1/chat.go` (and the streaming variant), before resolving a model, call `comboRepo.FindByName(body.Model)`. If found and `combo.Models` is non-empty, dispatch to the combo chat flow. If `body.Model` contains `/`, **skip** the combo lookup (matches `getComboModelsFromData` behaviour). The dispatch path must:
1. Load `settings.GetComboSettings()`.
2. Call `combo.Resolve(model, settings)` to get a `Selector`.
3. Call `selector.NextOrder(comboName, combo.Models)` to get the ordered model list.
4. Forward to `handleComboChat` (COMBO-070).

## Input

OpenAI-format chat request body with `model` field.

## Output

Delegates to COMBO-070 with the resolved model order, or proceeds with normal single-model flow.

## Logic

1. Check if `body.Model` contains `/`; if yes, skip combo lookup (provider/model form)
2. Call `comboRepo.FindByName(body.Model)`
3. If not found or `combo.Models` empty: proceed with normal single-model handler
4. If found with non-empty models:
   - Load `settings.GetComboSettings()`
   - Call `combo.Resolve(model, settings)` to get `Selector`
   - Call `selector.NextOrder(comboName, combo.Models)` for ordered models
   - Forward to `handleComboChat`

## Acceptance Criteria
- [ ] `model: "gpt-4o"` skips combo lookup, uses existing alias flow
- [ ] `model: "my-combo"` invokes combo path with `["gpt-4o","claude-sonnet-4-5"]`
- [ ] `model: "unknown"` (not a combo) proceeds as invalid model
- [ ] Empty combo models list returns 400 Invalid model format
- [ ] Model containing `/` skips combo lookup (e.g. `openai/gpt-4o`)
- [ ] Combo detection integrated into both sync and streaming handlers

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Regular model | `model: "gpt-4o"` | Normal handler, no combo |
| Combo model exists | `model: "my-combo"` | Combo path with models list |
| Unknown model | `model: "nonexistent"` | 400 Invalid model format |
| Provider/model form | `model: "openai/gpt-4o"` | Skips combo lookup |
| Empty models | Combo with `models: []` | 400 Invalid model format |
| Combo with models | Valid combo | Combo path invoked |