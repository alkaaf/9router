---
id: EXEC-011
domain: executors
status: DONE
estimate: 0.5d
title: Ollama Local Executor
---

## Description

Port the `OllamaLocalExecutor` (extends `DefaultExecutor`). Local-only executor for Ollama with custom URL format (`/api/chat` instead of `/v1/chat/completions`), no auth required, and Ollama-specific API format.

## Input

- HTTP POST to `http://localhost:11434/api/chat`
- No auth required
- Body: Ollama-specific format (different from OpenAI)
- Response: Ollama-specific NDJSON

## Output

- `internal/executor/ollama.go` — OllamaLocalExecutor struct extending base
- Override: `BuildUrl`
- Response translation via Ollama → OpenAI translator

## Logic

1. Define `OllamaLocalExecutor` struct extending `BaseExecutor`
2. Override `BuildUrl` — constructs `{ollamaHost}/api/chat` with stream param
3. Implement host resolution from credentials or environment (default `localhost:11434`)
4. Override `TransformRequest` — convert OpenAI format to Ollama format
5. Response translation: Ollama NDJSON → OpenAI SSE (via translator or inline)

## Acceptance Criteria
- [x] Correct Ollama URL format (/api/chat)
- [x] No auth headers set
- [x] Default host resolved from environment or defaults to localhost:11434
- [x] Request body correctly transformed to Ollama format
- [x] Response correctly transformed to OpenAI SSE format

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Default host | No host in credentials | http://localhost:11434 |
| Custom host | host="192.168.1.100:11434" | Correct URL |
| URL construction | BuildUrl("llama3", true) | /api/chat with stream param |
| No auth | BuildHeaders | No Authorization header |
| Request transform | OpenAI body | Ollama format body |

## Agent Log
- 2026-06-04 17:08 — Verified implementation: `internal/executor/ollama.go` (OllamaLocalExecutor extends BaseExecutor; BuildUrl constructs `{host}/api/chat` with stream=true; no auth headers).
- 2026-06-04 17:08 — Tests: `internal/executor/ollama_test.go` covers host resolution, URL construction, auth absence, request/response transformation.
- 2026-06-04 17:08 — Verification: `go build ./internal/executor/...` passes; `go test ./internal/executor/...` PASS.
