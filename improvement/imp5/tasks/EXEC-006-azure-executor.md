---
id: EXEC-006
domain: executors
status: DONE
estimate: 1d
title: Azure OpenAI Executor
---

## Description

Port the `AzureExecutor` (extends `DefaultExecutor`). Azure OpenAI uses custom URL format, API-key header (not Bearer token), and configuration from providerSpecificData.

## Input

- HTTP POST to `{endpoint}/openai/deployments/{deployment}/chat/completions?api-version={version}`
- Auth: `api-key` header (not Bearer token)
- Configuration: azureEndpoint, apiVersion, deployment, organization
- Response: Standard SSE (OpenAI-compatible)

## Output

- `internal/executor/azure.go` — AzureExecutor struct extending base
- Overrides: `BuildUrl`, `BuildHeaders`
- Passthrough `TransformRequest`

## Logic

1. Define `AzureExecutor` struct extending `BaseExecutor`
2. Override `BuildUrl` — constructs `{azureEndpoint}/openai/deployments/{deployment}/chat/completions?api-version={apiVersion}` with stream param
3. Override `BuildHeaders` — `api-key: {apiKey}` header (not Authorization: Bearer)
4. Optional: `OpenAI-Organization` header if organization provided
5. Passthrough `TransformRequest` — same as OpenAI

## Acceptance Criteria
- [ ] Correct Azure URL format with deployment and API version
- [ ] api-key header set instead of Bearer token
- [ ] Optional organization header included when provided
- [ ] Stream parameter correctly added to URL
- [ ] Error mapping for Azure-specific error responses

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Azure URL construction | endpoint="https://foo.openai.azure.com", deployment="gpt-4", version="2024-02-15" | Correct URL with all params |
| API-key header | BuildHeaders with apiKey | api-key: {key} header |
| With organization | BuildHeaders with apiKey and org | api-key + OpenAI-Organization headers |
| Without organization | BuildHeaders with apiKey only | api-key header only |
| Stream URL | stream=true | URL includes stream=true param |
