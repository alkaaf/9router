---
id: SYS-009
domain: settings
status: DONE
estimate: 1h
title: POST /api/locale — Set locale cookie
---

## Agent Log

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (all PASS)
- Code location: internal/system/009-locale.go + internal/system/009-locale_test.go
- Started: 2026-06-04
- Agent: agent-sys

## Description

Accept a locale value, validate it against the supported locales list, normalise it (e.g., `EN` → `en`), and set a `locale` cookie with a 1-year max-age and `path: /`.

## Input

```json
{ "locale": "en" }
```

## Output

```json
{ "success": true, "locale": "en" }
```

Also sets a `Set-Cookie` header: `locale=en; Max-Age=31536000; Path=/; HttpOnly`.

## Logic

1. Parse the request body and extract the `locale` field.
2. Validate against the supported locales list (e.g., `["en", "zh", "ja", "ko", "es", "fr", "de", ...]`).
3. Normalise: lowercase the locale string, then validate.
4. If invalid, return 400 with `{ "error": "Invalid locale" }` (or similar error message).
5. If valid, set a `Set-Cookie` response header:
   - Name: `locale`
   - Value: normalised locale string
   - `Max-Age`: 31536000 (1 year in seconds)
   - `Path`: `/`
   - `HttpOnly`: true
6. Return 200 with `{ "success": true, "locale": "<normalised>" }`.

## Acceptance Criteria

- [ ] `POST /api/locale` accepts `{ "locale": "en" }`
- [ ] Valid locale returns 200 with `{ "success": true, "locale": "en" }`
- [ ] `Set-Cookie: locale=en; Max-Age=31536000; Path=/; HttpOnly` header is set
- [ ] Locale is normalised to lowercase (e.g., `EN` → `en`)
- [ ] Invalid locale returns 400 with error message
- [ ] Missing `locale` field returns 400
- [ ] Cookie `path` is `/` (accessible site-wide)

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Valid locale | `{ "locale": "en" }` | 200, `{ "success": true, "locale": "en" }`, cookie set |
| Uppercase locale | `{ "locale": "EN" }` | 200, normalised to `en`, cookie set |
| Invalid locale | `{ "locale": "xx" }` | 400, `{ "error": "Invalid locale" }` |
| Missing field | `{}` | 400, validation error |
| Cookie attributes | Valid request | `Max-Age=31536000`, `Path=/`, `HttpOnly` |
