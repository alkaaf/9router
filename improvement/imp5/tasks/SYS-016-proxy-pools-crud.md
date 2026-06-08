---
id: SYS-016
domain: settings
status: DONE
estimate: 4h
title: GET/POST /api/proxy-pools — List and create proxy pools
---

## Description

Two endpoints for managing proxy pools. GET lists all pools with optional `isActive` filter and `includeUsage` flag (enriches with `boundConnectionCount`). POST creates a new pool with defaults (`type: http`, `isActive: true`).

## Input

**GET:** Query params (optional):
- `?isActive=true` — filter to only active pools
- `?includeUsage=true` — add `boundConnectionCount` to each pool

**POST:**
```json
{
  "name": "us-east-proxy",
  "proxyUrl": "http://user:pass@proxy.example.com:8080",
  "type": "http",
  "noProxy": "localhost,127.0.0.1",
  "isActive": true,
  "strictProxy": false
}
```

## Output

**GET:**
```json
{
  "proxyPools": [
    {
      "id": "...",
      "name": "us-east-proxy",
      "proxyUrl": "http://user:pass@proxy.example.com:8080",
      "type": "http",
      "noProxy": "localhost,127.0.0.1",
      "isActive": true,
      "strictProxy": false,
      "boundConnectionCount": 0
    }
  ]
}
```

**POST:**
```json
{
  "proxyPool": {
    "id": "...",
    "name": "us-east-proxy",
    "proxyUrl": "...",
    "type": "http",
    "noProxy": "",
    "isActive": true,
    "strictProxy": false
  }
}
```

## Logic

### GET
1. Read all proxy pools from the `proxyPools` table.
2. If `isActive` query param is present, filter to pools where `isActive` matches.
3. If `includeUsage=true` is present, for each pool count provider connections bound to it (via `proxyPoolId` foreign key) and add `boundConnectionCount`.
4. Return `{ "proxyPools": [...] }` with 200.

### POST
1. Parse request body. Require `name` and `proxyUrl`.
2. Apply defaults: `type: "http"`, `isActive: true`, `strictProxy: false`, `noProxy: ""`.
3. Validate `type` is one of allowed values (e.g., `http`, `socks5`, `cloudflare`, `vercel`, `deno`).
4. Generate a unique ID for the pool.
5. Persist to `proxyPools` table.
6. Return the created pool with 201.

## Acceptance Criteria

- [ ] `GET /api/proxy-pools` returns 200 with all pools
- [ ] `?isActive=true` filters to active pools only
- [ ] `?isActive=false` filters to inactive pools only
- [ ] `?includeUsage=true` adds `boundConnectionCount` to each pool
- [ ] `POST /api/proxy-pools` requires `name` and `proxyUrl`
- [ ] Missing `name` returns 400
- [ ] Missing `proxyUrl` returns 400
- [ ] Defaults: `type: "http"`, `isActive: true` when not provided
- [ ] Created pool is returned with 201
- [ ] Invalid `type` returns 400

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| List all | GET /api/proxy-pools | 200, all pools |
| Filter active | `?isActive=true` | Only active pools |
| Filter inactive | `?isActive=false` | Only inactive pools |
| Include usage | `?includeUsage=true` | Each pool has `boundConnectionCount` |
| Create pool | Valid POST body | 201, pool created with defaults applied |
| Missing name | POST without `name` | 400 |
| Missing proxyUrl | POST without `proxyUrl` | 400 |
| Invalid type | `{ "type": "invalid" }` | 400 |
| Defaults applied | POST with minimal body | `type: "http"`, `isActive: true` |
