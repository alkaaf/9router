---
id: OAUTH-012
domain: oauth
status: DONE
estimate: 1h
title: Translator UI State Fetch Endpoint
---

## Description

Implement the translator UI state fetch endpoint. Returns the current translator configuration and state including active format, enabled translators, and per-translator settings. Reads from the `settings` table (JSON `data` column, key=`translator`).

## Input

- HTTP method: GET
- Path: `/api/translator/state`
- Auth header: JWT or `x-9r-cli-token`

## Output

- Success: `{"format": "openai", "translators": {...}, "settings": {...}}`
- Default (no config): Returns default translator state

## Logic

1. Validate auth (JWT or CLI token)
2. Query `settings` table for row with key=`translator`
3. If no config found, return default state:
   - `format`: "openai"
   - `translators`: empty/default object
   - `settings`: empty/default object
4. Parse JSON `data` column
5. Return parsed translator state

## Acceptance Criteria
- [x] Default state returned when no config saved
- [x] Saved config returned correctly
- [x] Config parsed without error on malformed data
- [x] Response matches frontend translator UI expectations
- [x] Auth via JWT or CLI token enforced

## Agent Log
- 2026-06-04: Implemented `HandleTranslatorState` in `internal/oauth/translator.go`. Reads `translator` key from `TranslatorSettingsRepo`. Returns `defaultTranslatorState` (format=openai, empty maps) on missing/empty/malformed data. Fills nil maps with empty maps. Verified `go build ./internal/oauth/` passes.

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Config exists | GET with saved translator config | HTTP 200, parsed config |
| No config | GET with no translator settings | HTTP 200, default state |
| Malformed config | GET with corrupted settings data | HTTP 200, default state (graceful fallback) |
| No auth | GET without auth header | HTTP 401 |
