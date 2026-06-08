---
id: EXEC-004
domain: executors
status: DONE
estimate: 2d
title: Anthropic Executor (Claude API)
---

## Description

Port the Anthropic executor for direct Claude API access. This executor handles Anthropic's specific API format which differs from OpenAI (different headers, body structure, and SSE format).

## Input

- HTTP POST to `https://api.anthropic.com/v1/messages`
- Auth: `x-api-key` header (not Bearer token)
- Anthropic-specific headers: `anthropic-version`, `anthropic-dangerous-direct-access`
- Body: Anthropic message format `{model, messages, max_tokens, system, tools, ...}`
- Response: Anthropic SSE format (`event:` + `data:` lines)

## Output

- `internal/executor/anthropic.go` — AnthropicExecutor struct
- Custom `Execute` with Anthropic-specific request/response handling

## Logic

1. Define `AnthropicExecutor` struct extending `BaseExecutor`
2. Override `BuildUrl` — `https://api.anthropic.com/v1/messages` with stream param
3. Override `BuildHeaders` — `x-api-key`, `anthropic-version: 2023-06-01`, `anthropic-dangerous-direct-access`
4. Implement `TransformRequest` — convert OpenAI format body to Anthropic format
5. Implement SSE parsing for Anthropic events: `message_start`, `content_block_start`, `content_block_delta`, `message_delta`, `message_stop`
6. Transform Anthropic SSE → OpenAI SSE for unified streaming

## Acceptance Criteria
- [ ] Correct Anthropic headers set (x-api-key, anthropic-version)
- [ ] Request body correctly transformed to Anthropic format
- [ ] Anthropic SSE events correctly parsed
- [ ] Response transformed to OpenAI SSE format
- [ ] Error mapping for Anthropic-specific error codes

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Anthropic headers | BuildHeaders with apiKey | x-api-key and version headers |
| Request transform | OpenAI body | Anthropic format body |
| Event parsing | Anthropic SSE stream | Parsed event objects |
| Response transform | Anthropic events | OpenAI SSE format |
| Error handling | Anthropic 400 error | ExecutorError with message |
