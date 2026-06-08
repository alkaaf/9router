---
id: SYS-007
domain: settings
status: DONE
estimate: 2h
title: GET /api/version — Get current and latest version
---

## Agent Log

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (all PASS)
- Code location: internal/system/007-version.go + internal/system/007-version_test.go
- Started: 2026-06-04
- Agent: agent-sys

## Description

Fetch the latest version from the npm registry (`https://registry.npmjs.org/9router/latest`) with a 4-second timeout, compare it against the current version from `package.json`. Returns `hasUpdate: true` when a newer version is available.

## Input

None.

## Output

```json
{
  "currentVersion": "1.2.3",
  "latestVersion": "1.3.0",
  "hasUpdate": true
}
```

On npm fetch failure:
```json
{
  "currentVersion": "1.2.3",
  "latestVersion": null,
  "hasUpdate": false
}
```

## Logic

1. Read the current version from the project's `package.json` (or a compiled-in version constant).
2. Make an HTTP GET to `https://registry.npmjs.org/9router/latest` with a 4-second timeout.
3. Parse the response to extract the `version` field.
4. Compare `latestVersion` with `currentVersion`:
   - If `latestVersion` is successfully fetched and differs from `currentVersion`, set `hasUpdate: true`.
   - If `latestVersion` is `null` (fetch failed/timed out) or equals `currentVersion`, set `hasUpdate: false`.
5. Return the version info object.
6. On any network error or timeout, return `latestVersion: null` and `hasUpdate: false` — do not return 500.

## Acceptance Criteria

- [ ] `GET /api/version` returns 200 with `currentVersion`, `latestVersion`, and `hasUpdate`
- [ ] `currentVersion` matches the value in `package.json`
- [ ] When npm registry returns a newer version, `hasUpdate` is `true`
- [ ] When npm registry is same version, `hasUpdate` is `false`
- [ ] On npm fetch timeout (4s), `latestVersion` is `null` and `hasUpdate` is `false`
- [ ] On npm fetch failure (network error), `latestVersion` is `null` and `hasUpdate` is `false`
- [ ] No external call failure causes a 500 response
- [ ] npm registry URL is configurable or hardcoded to `https://registry.npmjs.org/9router/latest`

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Update available | npm returns newer version | `hasUpdate: true` |
| Up to date | npm returns same version | `hasUpdate: false` |
| npm timeout | Registry unreachable (>4s) | `latestVersion: null`, `hasUpdate: false`, 200 |
| npm error | Registry returns error | `latestVersion: null`, `hasUpdate: false`, 200 |
| Current version | Reads from package.json | `currentVersion` matches package.json |
