---
id: EXEC-005
domain: executors
status: DONE
estimate: 1.5d
title: Gemini CLI Executor
---

## Description

Port the `GeminiCLIExecutor`. Similar to Antigravity but specifically for Gemini CLI (Codey Assist). Handles Gemini format with Cloud Code Assist wrapping, Google OAuth refresh, and custom error parsing for Google API RetryInfo.

## Input

- HTTP POST to Gemini API endpoint
- Auth: Bearer token (Google OAuth access token)
- Body: wrapped in `{project, model, request: <body>}` envelope
- Response: Gemini SSE format (`data: {"candidates":[...]}`)

## Output

- `internal/executor/geminicli.go` — GeminiCLIExecutor struct
- Overrides: `BuildUrl`, `BuildHeaders`, `TransformRequest`, `ParseError`, `RefreshCredentials`

## Logic

1. Define `GeminiCLIExecutor` struct with `_currentModel` instance variable
2. Override `BuildUrl` — stream vs non-stream URL path (`streamGenerateContent?alt=sse`)
3. Override `BuildHeaders` — Bearer auth, User-Agent with model tracking
4. Override `TransformRequest` — wrap body in Code Assist envelope `{project, model, request}`
5. Override `ParseError` — extract RetryInfo from Google RPC error details
6. Override `RefreshCredentials` — Google OAuth token refresh flow
7. Store `_currentModel` during TransformRequest for use in BuildHeaders

## Acceptance Criteria
- [ ] Body correctly wrapped in Code Assist envelope
- [ ] Streaming URL path correctly constructed
- [ ] RetryInfo error parsing extracts retryDelay from Google RPC error
- [ ] Google OAuth refresh works correctly
- [ ] User-Agent includes model information

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Request wrapping | OpenAI body | Code Assist envelope |
| Stream URL | stream=true | streamGenerateContent?alt=sse |
| Non-stream URL | stream=false | generateContent |
| RetryInfo parsing | Google error with retryDelay | ExecutorError with RetryAfter |
| OAuth refresh | Expired access token | New valid access token |
