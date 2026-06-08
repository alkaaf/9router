---
id: COMBO-003
domain: combos
status: TODO
estimate: 1h
title: Add Settings Schema Fields for Combos
---

## Description

Extend `model.Setting.Data` JSON schema (settings lives in `settings` table as a single JSON blob) with combo-related fields and document them: `comboStrategy string` (default `"fallback"`), `comboStickyRoundRobinLimit int` (default `1`), `comboStrategies map[string]ComboStrategyOverride` where `ComboStrategyOverride{FallbackStrategy string}`. Validate these fields inside the existing `internal/handler/api/settings.go` PATCH handler — but the **combo handler** also reads them, so expose a typed `ComboSettings` accessor in the repository.

## Input

Raw settings JSON from DB.

## Output

`settings.GetComboSettings()` returns a fully-typed `ComboSettings` struct, filling defaults for missing fields.

## Logic

- Add to settings schema: `comboStrategy`, `comboStickyRoundRobinLimit`, `comboStrategies`
- `ComboSettings` struct with typed accessors
- Default values applied when fields are missing: `comboStrategy = "fallback"`, `comboStickyRoundRobinLimit = 1`
- Validation in PATCH handler for settings

## Acceptance Criteria
- [ ] Settings JSON schema includes combo fields
- [ ] `GetComboSettings()` returns defaults when fields missing
- [ ] Partial settings (e.g. `comboStrategy: "round-robin"`) other fields default
- [ ] Malformed `comboStrategies` map does not panic
- [ ] PATCH handler validates combo settings fields

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Empty settings | No combo fields | Defaults: strategy="fallback", sticky=1, strategies={} |
| Partial settings | `{"comboStrategy":"round-robin"}` | Strategy set, sticky defaults to 1 |
| Full settings | All combo fields | All values returned as set |
| Malformed map | `{"comboStrategies":"invalid"}` | No panic, uses defaults |
| Invalid strategy value | `{"comboStrategy":"invalid"}` | Validation error or accepted as-is |