---
id: EXEC-007
domain: executors
status: DONE
estimate: 3d
title: Google Vertex AI Executor
---

## Description

Port the `VertexExecutor`. Handles Google Cloud Vertex AI with two sub-modes: (1) Vertex Gemini: SA JSON → JWT assertion → Bearer token, (2) Vertex Partner: OpenAI-compatible endpoint for Llama, Mistral, DeepSeek, Qwen etc.

## Input

- Mode 1 (Gemini): HTTP POST to `https://aiplatform.googleapis.com/v1/projects/{project}/locations/{location}/publishers/google/models/{model}:streamGenerateContent?alt=sse`
- Mode 2 (Partner): HTTP POST to `https://aiplatform.googleapis.com/v1/projects/{project}/locations/global/endpoints/openapi/chat/completions`
- Auth: Bearer token (from SA JWT) or API key in URL query
- Body: Gemini format (vertex) or OpenAI format (vertex-partner)

## Output

- `internal/executor/vertex.go` — VertexExecutor struct
- Override: `BuildUrl`, `BuildHeaders`, `Execute` (full override for SA JSON token minting), `RefreshCredentials`
- Project ID resolution function, SA JSON → JWT → Bearer token flow

## Logic

1. Define `VertexExecutor` struct with SA JSON parsing capability
2. Implement `parseVertexSaJson` — extract project_id, client_email, private_key from SA JSON
3. Implement `refreshVertexToken` — JWT creation from SA + exchange for Bearer token
4. Override `BuildUrl` — mode-specific URL construction (Gemini vs Partner)
5. Override `BuildHeaders` — Bearer header from SA JWT or API key in URL
6. Implement project ID resolution — probe request, parse `projects/{id}` from error
7. Implement token cache with TTL for SA JWT tokens

## Acceptance Criteria
- [ ] SA JSON correctly parsed (project_id, client_email, private_key)
- [ ] JWT correctly created and exchanged for Bearer token
- [ ] Vertex Gemini URL format correct
- [ ] Vertex Partner URL format correct
- [ ] Project ID resolution works via probe request
- [ ] Token caching prevents unnecessary refreshes

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| SA JSON parsing | Valid service account JSON | Parsed struct with all fields |
| JWT creation | SA credentials | JWT string |
| Vertex Gemini URL | project="proj", location="us-central1", model="gemini-1.5-pro" | Correct vertex URL |
| Vertex Partner URL | project="proj", endpoint=global | Correct partner URL |
| Project resolution | 404 error with projects/{id} | Extracted project ID |
| Token refresh | Expired JWT | New valid Bearer token |
