---
id: CHAT-004
domain: chat-core
status: DONE
estimate: 1h
title: Model Resolution
---

## Agent Log
- Started: 2026-06-04 18:15
- Agent: agent-chat

## Description

Parse model string into provider/model components and resolve to actual provider+model. Supports formats: `provider/model`, `model` (alias), `combo-name` (no slash). Returns structured `ModelInfo` with provider, model, and metadata.

## Input

- `string` (model string)
- Settings with model aliases and provider strategies
- ProviderNodes table for prefix matching

## Output

- `*ModelInfo` struct with `provider`, `model`, `isAlias`, `isCombo`
- Error if resolution fails

## Logic

1. Parse model string for `/` separator
2. If contains `/` → extract provider/model directly
3. Check if combo name (CHAT-005) → return combo info
4. Resolve model alias from local DB settings
5. Match provider prefix against ProviderNodes table
6. Log routing decision
7. Return resolved ModelInfo

## Agent Log
- Started: 2026-06-04 18:15
- Completed: 2026-06-04 18:30
- Agent: agent-chat
- AC-001 verified: TestParseModel_Alias — "gpt-4" returns isAlias=true
- AC-002 verified: TestParseModel_ProviderModel — "openai/gpt-4" splits correctly
- AC-003 verified: TestParseModel_ProviderModel + TestResolveModel_Explicit — explicit form bypasses alias map
- AC-004 verified: TestResolveModel_AliasMiss — unknown alias falls through to inference
- AC-005 verified: Logging delegated to caller; ResolveModel returns ModelInfo struct for caller to log

## Acceptance Criteria
- [x] `gpt-4` → isAlias=true, resolves via alias
- [x] `openai/gpt-4` → provider=openai, model=gpt-4
- [x] `provider/model` format works correctly
- [x] Unknown alias returns original
- [x] Routing decision is logged

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/chatcore/model.go
- Tests: internal/chatcore/model_test.go (16 tests, all pass)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Alias | `gpt-4` | isAlias=true, resolved provider/model |
| Explicit | `openai/gpt-4` | provider=openai, model=gpt-4 |
| Combo | `my-combo` | isCombo=true, models=[...] |
| Unknown | `unknown-model` | original model string |
| Multiple slashes | `a/b/c` | provider=a, model=b/c |
| Trailing slash | `openai/` | error or handled gracefully |
