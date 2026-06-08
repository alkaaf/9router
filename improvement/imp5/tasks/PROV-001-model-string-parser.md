---
id: PROV-001
domain: providers
status: DONE
estimate: 1h
title: Model String Parser
---

## Description

Parse a model string into `{ provider, model }`. Support three formats: bare model (uses default provider from settings), `provider/model` (explicit), and `provider:model` (OpenAI-compatible alias style). Validate that resolved provider exists in the registry.

## Agent Log
- Started: 2026-06-04 09:20
- Agent: agent-prov
- AC-001 verified: TestParseModelString_SlashFormat passes
- AC-002 verified: TestParseModelString_ColonFormat passes
- AC-003 verified: TestParseModelString_BareWithDefault passes
- AC-004 verified: TestParseModelString_UnknownProvider returns Err
- AC-005 verified: TestParseModelString_BareWithNoDefault returns Err

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/providers/parser.go, internal/providers/registry.go, internal/providers/parser_test.go

## Input

- Model string: e.g. `"claude-3-haiku-20240307"`, `"anthropic/claude-sonnet-4-20250514"`, `"openai/gpt-4o-mini"`
- Provider registry map
- Default provider from settings (fallback for bare models)

## Output

- `modelInfo = { provider: "anthropic", model: "claude-sonnet-4-20250514" }`
- `null` if unknown provider or model-only input with no default provider

## Logic

1. If string contains `/`, split on first `/`: `provider = parts[0]`, `model = parts[1]`.
2. Else if string contains `:`, split on first `:`: `provider = parts[0]`, `model = parts[1]`.
3. Else (bare model): use `defaultProvider` from settings as provider.
4. Resolve provider via PROV-002 (alias resolution).
5. Validate resolved provider exists in registry (PROV-003).
6. Return `modelInfo` or `null` on failure.

## Acceptance Criteria
- [x] `"provider/model"` correctly splits into provider and model
- [x] `"provider:model"` correctly splits into provider and model
- [x] Bare model uses default provider from settings
- [x] Unknown provider returns null with error
- [x] Model-only input with no default provider returns error

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Slash format | `"anthropic/claude-sonnet-4-20250514"` | `{ provider: "anthropic", model: "claude-sonnet-4-20250514" }` |
| Colon format | `"openai:gpt-4o-mini"` | `{ provider: "openai", model: "gpt-4o-mini" }` |
| Bare model with default | `"claude-3-haiku-20240307"` + default `"anthropic"` | `{ provider: "anthropic", model: "claude-3-haiku-20240307" }` |
| Unknown provider | `"unknown/model"` | `null` with error |
| Bare with no default | `"some-model"` + no default | `null` with error |
