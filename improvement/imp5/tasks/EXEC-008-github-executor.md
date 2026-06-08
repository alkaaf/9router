---
id: EXEC-008
domain: executors
status: DONE
estimate: 3d
title: GitHub Copilot Executor
---

## Description

Port the `GithubExecutor`. Handles Copilot-specific headers, model-specific transforms (max_completion_tokens for gpt-5+, temperature for gpt-5.4, thinking for Claude), dual routing (/chat/completions first, fallback to /responses for codex models), message sanitization, and Copilot token refresh.

## Input

- HTTP POST to `https://api.githubcopilot.com/chat/completions` or `/responses`
- Auth: `copilotToken` (refreshed from GitHub OAuth token)
- Model detection: which models need /responses endpoint
- Message sanitization: strip non-text/image_url content types

## Output

- `internal/executor/github.go` — GithubExecutor struct
- Overrides: `BuildHeaders`, `TransformRequest`, `Execute`, `RefreshCredentials`, `NeedsRefresh`
- Responses API SSE → OpenAI Chat SSE transform stream

## Logic

1. Define `GithubExecutor` struct with knownCodexModels set
2. Override `BuildHeaders` — copilotToken, copilot-integration-id, editor-version, editor-plugin-version
3. Implement model detection functions: `requiresMaxCompletionTokens`, `supportsTemperature`, `supportsThinking`, `requiresResponsesEndpoint`
4. Override `TransformRequest` — model-specific transforms, message sanitization
5. Implement dual routing: try /chat/completions → 400 → switch to /responses
6. Implement `RefreshCredentials` — GitHub OAuth → Copilot Token
7. Implement Responses API → Chat Completions SSE transform

## Acceptance Criteria
- [ ] All Copilot-specific headers correctly set
- [ ] Model detection correctly identifies codex models
- [ ] Dual routing: try chat/completions, fallback to responses on 400
- [ ] Claude model messages sanitized (tool content stripped)
- [ ] Token refresh from GitHub OAuth works
- [ ] Responses API SSE correctly transformed to Chat SSE

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Codex model routing | Model="gpt-5-codex-high" | Use /responses endpoint |
| Non-codex model | Model="gpt-4" | Use /chat/completions endpoint |
| Message sanitization | Claude message with tool_use | Tool content stripped |
| OAuth refresh | GitHub OAuth token | Copilot token |
| Dual routing fallback | First endpoint returns 400 | Try second endpoint |
| Responses transform | Responses API events | Chat Completions SSE |
