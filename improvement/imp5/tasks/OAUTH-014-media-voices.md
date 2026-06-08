---
id: OAUTH-014
domain: oauth
status: DONE
estimate: 2h
title: Media Providers TTS Voices List Endpoint
---

## Description

Implement the TTS provider voices list endpoint. Returns available voices from the specified TTS provider (e.g., OpenAI TTS, Azure TTS, ElevenLabs). Voices are cached in-memory with a 5-minute TTL per provider to reduce external API calls.

## Input

- HTTP method: GET
- Path: `/api/media-providers/tts/:provider/voices`
- Auth header: JWT or `x-9r-cli-token`
- Query params: Optional provider-specific filters

## Output

- Success: `{"provider": "openai", "voices": [{"id": "alloy", "name": "Alloy", "language": "en"}]}`
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate auth (JWT or CLI token)
2. Extract provider name from URL param
3. Check in-memory cache for provider voices (TTL: 5 minutes)
4. If cache hit, return cached voices
5. If cache miss:
   a. Call provider API to fetch voices list
   b. Transform provider response to standard format
   c. Store in cache with TTL
6. Return standardized voice list
7. Return 404 for unknown providers

## Acceptance Criteria
- [x] OpenAI voices returned in correct format
- [x] Unknown provider returns 404
- [x] Cache hit returns without provider API call
- [x] Cache miss fetches and caches voices
- [x] Voices cached for 5 minutes
- [x] Auth via JWT or CLI token enforced
- [x] Response format consistent across providers

## Agent Log
- 2026-06-04: Implemented `HandleMediaVoices` in `internal/oauth/media_voices.go`. `MediaVoicesFetcher` registered per-provider via `RegisterMediaVoicesFetcher`. In-memory cache with 5-min TTL (`mediaVoicesCacheTTL`). Cache hit short-circuits fetcher. `ClearMediaVoicesCache` exported for tests. Returns `{provider, voices}` with standardized `Voice` shape. Verified `go build ./internal/oauth/` passes.

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| OpenAI voices | GET /api/media-providers/tts/openai/voices | HTTP 200, voices array |
| Unknown provider | GET /api/media-providers/tts/unknown/voices | HTTP 404 |
| Cache hit (within TTL) | GET same provider twice within 5 min | Second request served from cache |
| Cache expiry (after TTL) | GET after 5+ minutes | Fresh fetch from provider API |
| No auth | GET without auth header | HTTP 401 |
