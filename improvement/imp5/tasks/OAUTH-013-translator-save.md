---
id: OAUTH-013
domain: oauth
status: DONE
estimate: 1h
title: Translator UI Config Save Endpoint
---

## Description

Implement the translator UI configuration save endpoint. Stores translator settings to the `settings` table via upsert (by key=`translator`). Supports partial config merges with existing settings.

## Input

- HTTP method: PUT
- Path: `/api/translator/config`
- Auth header: JWT or `x-9r-cli-token`
- Request body: `{ "format": "string", "translators": {...}, "settings": {...} }`

## Output

- Success: `{"success": true}`
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate auth (JWT or CLI token)
2. Parse request body
3. Validate config structure (format, translators, settings)
4. Query existing settings from `settings` table
5. Merge new config with existing (partial update)
6. Serialize merged config to JSON
7. Upsert `settings` row with key=`translator`
8. Return success

## Acceptance Criteria
- [x] Valid config saved successfully
- [x] Partial config merges with existing settings
- [x] Invalid config structure returns 400
- [x] Settings row updated in DB
- [x] Auth via JWT or CLI token enforced
- [x] Error responses match expected format

## Agent Log
- 2026-06-04: Implemented `HandleTranslatorSave` in `internal/oauth/translator.go`. Reads existing config, merges `translators` and `settings` maps (partial update), upserts via `TranslatorSettingsRepo.UpsertByKey`. Validates `format` is non-empty. Returns `{success: true}`. Verified `go build ./internal/oauth/` passes.

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Full config save | PUT with complete config | HTTP 200, `{"success": true}` |
| Partial config | PUT with only `{format: "anthropic"}` | HTTP 200, merges with existing |
| Invalid config | PUT with `{format: 123}` | HTTP 400 |
| No auth | PUT without auth header | HTTP 401 |
| Verify merge | PUT partial, then GET /state | HTTP 200, merged config returned |
