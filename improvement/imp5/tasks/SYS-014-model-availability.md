---
id: SYS-014
domain: settings
status: DONE
estimate: 2h
title: GET/POST /api/models/availability — Model cooldowns and availability status
---

## Description

Two endpoints for managing model availability. GET returns current model cooldowns and unavailable status from provider connections. POST clears a cooldown for a specific model and resets the connection's `testStatus` to `active`.

## Input

**GET:** None.

**POST:**
```json
{ "action": "clearCooldown", "provider": "openai", "model": "gpt-4o" }
```

## Output

**GET:**
```json
{
  "models": [
    {
      "provider": "openai",
      "model": "gpt-4o",
      "status": "cooldown",
      "until": 1750000000000,
      "connectionId": "conn-abc",
      "connectionName": "My OpenAI",
      "lastError": "rate limited"
    },
    {
      "provider": "openai",
      "model": "__all",
      "status": "unavailable",
      "connectionId": "conn-abc",
      "connectionName": "My OpenAI",
      "lastError": "API key invalid"
    }
  ],
  "unavailableCount": 2
}
```

**POST:**
```json
{ "success": true }
```

## Logic

### GET
1. Read all provider connections from the `providerConnections` table.
2. For each connection, check for active `modelLock_*` fields (cooldown locks with a future `until` timestamp).
3. For connections with `testStatus: unavailable`, include an entry with `model: "__all"` and `status: "unavailable"`.
4. Filter out expired locks (where `until` is in the past).
5. Assemble the response array with `provider`, `model`, `status`, `until`, `connectionId`, `connectionName`, and `lastError`.
6. Count total unavailable connections for `unavailableCount`.
7. Return the assembled object.

### POST
1. Parse request body — require `action: "clearCooldown"`, `provider`, and `model`.
2. If `action` is not `clearCooldown`, return 400.
3. Find the provider connection matching the `provider` + `model` combination that has an active cooldown lock.
4. Clear the `modelLock_*` field (set to null/remove).
5. Reset the connection's `testStatus` to `active`.
6. Persist the connection update.
7. Return `{ "success": true }` with 200.

## Acceptance Criteria

- [ ] `GET /api/models/availability` returns 200 with cooldown/unavailable entries
- [ ] Only active (non-expired) cooldown locks are returned
- [ ] Connections with `testStatus: unavailable` produce `__all` model entries
- [ ] Each entry includes `provider`, `model`, `status`, `until`, `connectionId`, `connectionName`, `lastError`
- [ ] `unavailableCount` reflects total unavailable connections
- [ ] `POST /api/models/availability` with `clearCooldown` action clears the lock
- [ ] POST resets `testStatus` to `active`
- [ ] Invalid action returns 400
- [ ] Missing `provider` or `model` returns 400

## Test Scenarios

| Scenario | Input | Expected Output |
|----------|-------|----------------|
| GET with cooldowns | Active locks exist | 200, cooldown entries in `models` array |
| GET with unavailable | Connection `testStatus: unavailable` | `__all` model entry in response |
| GET no locks | No active cooldowns | 200, empty `models` array, `unavailableCount: 0` |
| GET expired filtered | Lock `until` in the past | Lock not returned |
| POST clear cooldown | `{ "action": "clearCooldown", "provider": "o", "model": "gpt-4o" }` | 200, lock cleared, `testStatus` → active |
| POST invalid action | `{ "action": "unknown" }` | 400 |
| POST missing fields | Missing `provider` | 400 |
