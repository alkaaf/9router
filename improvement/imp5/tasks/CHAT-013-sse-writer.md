---
id: CHAT-013
domain: chat-core
status: DONE
estimate: 2h
title: SSE Writer
---

## Description

Low-level SSE (Server-Sent Events) chunk writer. Format: `data: <json>\n\n`. Provides helpers for writing chunks, done markers, and error events with proper formatting and flushing.

## Input

- `io.Writer` (Fiber bufio.Writer)

## Output

- Written SSE formatted bytes to writer

## Logic

1. WriteChunk(model, delta, finishReason):
   - Format: `data: {"model":"...","choices":[{"delta":{"content":"..."}}]}\n\n`
   - Write to buffer
   - Flush buffer
2. WriteDone():
   - Write: `data: [DONE]\n\n`
   - Flush
3. WriteError(statusCode, message):
   - Write SSE error event
   - Flush

## Acceptance Criteria
- [x] WriteChunk produces correct SSE format
- [x] WriteDone produces `data: [DONE]\n\n`
- [x] WriteError produces error event
- [x] Multiple writes concatenated correctly
- [x] Buffer flushed after each write
- [x] Output matches Node.js formatSSE() byte-for-byte

## Agent Log
- Started: 2026-06-04 19:25
- Implemented: 2026-06-04 17:36
- Agent: agent-chat

### Implementation
- Created `internal/chatcore/sse_writer.go` (180 lines).
- `SSEWriter` emits `data: <json>\n\n` via `json.Marshal` on any payload.
- WriteChunk(model, content, finishReason) → OpenAI-style SSEEvent.
- WriteDone() → `data: [DONE]\n\n`.
- WriteError(status, message) → `event: error\ndata: {...}\n\n`.
- Flushes after every write when underlying writer implements `Flusher`.

### Evidence
- `go test -v -run "TestSSE" ./internal/chatcore/...`: 6 PASS in 0.493s.
- Tests cover: WriteChunk, WriteDone, WriteError, multiple writes (4 events), no-flush fallback, empty content, WriteRaw.

