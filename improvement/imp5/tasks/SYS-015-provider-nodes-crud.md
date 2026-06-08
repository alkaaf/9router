---
id: SYS-015
domain: settings
status: DONE
estimate: 4h
title: GET/POST /api/provider-nodes — List and create provider nodes
---

## Description

Two endpoints for managing provider nodes. GET returns all nodes. POST creates a new node with type-specific validation and base URL sanitisation. Supports three node types: `openai-compatible`, `anthropic-compatible`, and `custom-embedding`.

## Input

**GET:** None.

**POST:**
```json
{
  "name": "My Gateway",
  "prefix": "my",
  "type": "anthropic-compatible",
  "baseUrl": "https://api.example.com/v1"
}
```

## Output

**GET:**
```json
{
  "nodes": [
    {
      "id": "anthropic-compatible-abc123",
      "type": "anthropic-compatible",
      "name": "My Gateway",
      "prefix": "my",
      "baseUrl": "https://api.example.com/v1"
    }
  ]
}
```

**POST:**
```json
{
  "node": {
    "id": "anthropic-compatible-abc123",
    "type": "anthropic-compatible",
    "name": "My Gateway",
    "prefix": "my",
    "baseUrl": "https://api.example.com/v1"
  }
}
```

## Logic

### GET
1. Read all provider nodes from the `providerNodes` table.
2. Return `{ "nodes": [...] }` with 200.

### POST
1. Parse request body. Require `name`, `prefix`, `type`, and `baseUrl`.
2. Validate `type` is one of: `openai-compatible`, `anthropic-compatible`, `custom-embedding`. Return 400 if not.
3. Validate `name` and `prefix` are non-empty. Return 400 if missing.
4. Generate a unique node ID with a type-specific prefix:
   - `openai-compatible-chat-<random>` or `openai-compatible-responses-<random>` (depending on `apiType`)
   - `anthropic-compatible-<random>`
   - `custom-embedding-<random>`
5. Sanitise `baseUrl` per type:
   - `anthropic-compatible`: strip trailing slash and `/messages` path.
   - `custom-embedding`: strip trailing slash and `/embeddings` path.
   - `openai-compatible`: use as-is (or strip trailing slash).
6. Persist the node to the `providerNodes` table.
7. Return the created node with 201.

## Acceptance Criteria

- [ ] `GET /api/provider-nodes` returns 200 with all nodes
- [ ] `POST /api/provider-nodes` validates `type` against allowed values
- [ ] Invalid `type` returns 400
- [ ] Missing `name` or `prefix` returns 400
- [ ] Node ID has correct type-specific prefix
- [ ] `anthropic-compatible` base URL strips trailing slash and `/messages`
- [ ] `custom-embedding` base URL strips trailing slash and `/embeddings`
- [ ] `openai-compatible` base URL is stored as-is (trailing slash stripped)
- [ ] Created node is returned in GET response
- [ ] Duplicate `name` or `prefix` handled gracefully (409 or overwrite)

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| List nodes | GET /api/provider-nodes | 200, `{ "nodes": [...] }` |
| Create anthropic | `{ "type": "anthropic-compatible", "baseUrl": "https://api.example.com/v1/messages" }` | 201, baseUrl sanitised to `https://api.example.com/v1` |
| Create embedding | `{ "type": "custom-embedding", "baseUrl": "https://api.example.com/v1/embeddings/" }` | 201, baseUrl sanitised to `https://api.example.com/v1` |
| Invalid type | `{ "type": "unknown" }` | 400 |
| Missing name | `{ "prefix": "x", "type": "o", "baseUrl": "..." }` | 400 |
| Missing prefix | `{ "name": "x", "type": "o", "baseUrl": "..." }` | 400 |
| ID prefix | Created node ID starts with type prefix | Correct prefix verified |
| Trailing slash | `baseUrl` with trailing `/` | Stripped in stored value |
