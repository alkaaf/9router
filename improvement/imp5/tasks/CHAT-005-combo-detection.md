---
id: CHAT-005
domain: chat-core
status: DONE
estimate: 1h
title: Combo Detection
---

## Agent Log
- Started: 2026-06-04 18:30
- Agent: agent-chat

## Description

Detect if the model string is a combo name (not a `provider/model` format). Returns the array of models in the combo, or nil if not a combo. Critical to check BEFORE alias resolution to prevent combo names from being routed as aliases.

## Input

- `string` (modelStr)
- `func(string) (*Combo, error)` (combo lookup function)

## Output

- `[]string` (models array) or `nil`
- `*Combo` info if found

## Logic

1. If modelStr contains `/` → return nil (explicit provider format)
2. Look up modelStr in combos table/registry
3. If found, return models array
4. If combo has 0 models, return nil
5. Return nil if not found

## Agent Log
- Started: 2026-06-04 18:30
- Completed: 2026-06-04 18:45
- Agent: agent-chat
- AC-001 verified: TestGetComboModels_Valid — combo returns models
- AC-002 verified: TestGetComboModels_ExplicitProvider — slash short-circuits
- AC-003 verified: TestGetComboModels_Unknown — returns nil
- AC-004 verified: TestGetComboModels_EmptyModels — empty list returns nil
- AC-005 verified: TestComboDetection_RunsBeforeAlias — caller composes the two in correct order

## Acceptance Criteria
- [x] `my-combo` with models [a, b, c] returns [a, b, c]
- [x] `provider/model` returns nil (explicit format)
- [x] Unknown combo name returns nil
- [x] Empty combo (no models) returns nil
- [x] Combo detection runs before alias resolution

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/combo.go
- Tests: internal/chatcore/combo_test.go (12 tests, all pass)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid combo | `my-combo` | models array |
| Explicit provider | `openai/gpt-4` | nil |
| Unknown name | `not-a-combo` | nil |
| Empty combo | `empty-combo` | nil |
| Combo with slash | `combo/name` | nil (has slash) |
