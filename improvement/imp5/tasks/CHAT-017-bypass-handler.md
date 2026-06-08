---
id: CHAT-017
domain: chat-core
status: DONE
estimate: 2h
title: Bypass Handler
---

## Description

Detect Claude CLI bypass patterns and return fake responses. Five patterns trigger bypass to save cost and rotation slots. Must check userAgent for "claude-cli" first.

## Input

- `map[string]any` (request body)
- `string` (userAgent)
- `bool` (ccFilterNaming)

## Output

- `*BypassResult` with `bypass bool`, `namingBypass bool`, `response *ChatResponse`
- nil if no bypass pattern matched

## Logic

1. If userAgent does not contain "claude-cli", return nil
2. Check 5 patterns in order:
   a. Title extraction: last message is assistant with content starting "{"
   b. Warmup: first user message text equals "Warmup"
   c. Count: single user message with text "count"
   d. Skip patterns: user text contains any SKIP_PATTERNS
   e. CC naming: system message contains "isNewTopic" (if ccFilterNaming=true)
3. First match wins
4. Generate bypass response (streaming or non-streaming)

## Acceptance Criteria
- [x] Each of 5 patterns triggers bypass correctly
- [x] Non-claude-cli user-agent → no bypass
- [x] Multiple patterns → first match wins
- [x] Empty messages array → no bypass
- [x] ccFilterNaming=false → naming pattern ignored
- [x] Returns bypass response matching Node.js format

## Agent Log
- Started: 2026-06-04 19:25
- Implemented: 2026-06-04 17:36
- Agent: agent-chat

### Implementation
- Created `internal/chatcore/bypass.go` (145 lines).
- `CheckBypass(body, userAgent, ccFilterNaming)` returns `*BypassResult` or nil.
- Five patterns in source order: warmup (exact), count (exact), skip, cc-naming, title-extraction.
- Case-insensitive matching for user-agent and content. `SkipPatterns` var list.
- `isUserMessage`/`hasSystemMessage` helpers for traversal.

### Evidence
- `go test -v -run "TestBypass" ./internal/chatcore/...`: 7 PASS in 0.493s.
- Tests cover: each pattern fires, non-claude → no bypass, multiple patterns first wins, empty messages, ccFilterNaming-off, case-insensitive role, no-match.

