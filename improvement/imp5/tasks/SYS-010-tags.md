---
id: SYS-010
domain: settings
status: DONE
estimate: 1h
title: GET /api/tags — List Ollama model tags
---

## Agent Log

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (all PASS)
- Code location: internal/system/010-tags.go + internal/system/010-tags_test.go
- Started: 2026-06-04
- Agent: agent-sys

## Description

Return the static list of Ollama model tags from the config module. CORS-enabled endpoint. No database calls — the list comes from application configuration.

## Input

None.

## Output

JSON array of Ollama model tag strings.

```json
["llama2", "llama2:13b", "codellama:7b", "mistral", ...]
```

## Logic

1. Register a `GET` handler.
2. Load the Ollama model tags list from the application config module (static configuration).
3. Return the list as a JSON array with 200 status.
4. Set CORS headers: `Access-Control-Allow-Origin: *`.

## Acceptance Criteria

- [ ] `GET /api/tags` returns 200
- [ ] Response is a JSON array of strings
- [ ] CORS headers are present (`Access-Control-Allow-Origin: *`)
- [ ] No database calls are made
- [ ] Tags come from config/source of truth (not hardcoded in the handler)
- [ ] Empty list returns `[]` if no tags are configured

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Happy path | GET /api/tags | 200, JSON array of tag strings |
| CORS header | GET /api/tags | `Access-Control-Allow-Origin: *` present |
| No tags configured | Config has no tags | 200, `[]` |
| Content-Type | GET response | `application/json` |
