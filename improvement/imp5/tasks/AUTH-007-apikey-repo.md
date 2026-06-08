---
id: AUTH-007
domain: auth
status: DONE
estimate: 1h
title: ApiKey Repository CRUD
---

## Description
Implement `ApiKeyRepository` with GORM-backed CRUD operations for API keys. All keys are stored and compared as bcrypt hashes. The repository provides `Create`, `FindByHash`, `List`, `UpdateLastUsed`, and `Delete` methods.

## Input
- `CreateApiKey(ctx context.Context, keyHash, name string) (*ApiKey, error)` — store a new bcrypt-hashed key
- `FindValidApiKey(ctx context.Context, rawKey string) (*ApiKey, error)` — bcrypt-compare against all stored hashes
- `ListApiKeys(ctx context.Context) ([]ApiKey, error)` — list all active (non-deleted) keys
- `UpdateLastUsed(ctx context.Context, id uint) error` — update `LastUsedAt` to now
- `DeleteApiKey(ctx context.Context, id uint) error` — soft delete by ID

## Output
- `(*ApiKey, error)` on create/find
- `([]ApiKey, error)` on list
- `error` on update/delete

## Logic
1. `CreateApiKey`: hash the raw key via `HashPassword`, save `KeyHash` and `Name`, return the created record (excluding `KeyHash` from response).
2. `FindValidApiKey`: query all non-deleted `ApiKey` records, iterate and `bcrypt.CompareHashAndPassword` each `KeyHash` against `rawKey`. Return the first match.
3. `ListApiKeys`: `db.Where("deleted_at IS NULL")`, select fields excluding `KeyHash`.
4. `UpdateLastUsed`: `db.Model(&ApiKey{}).Where("id=?", id).Update("last_used_at", time.Now())`.
5. `DeleteApiKey`: `db.Delete(&ApiKey{}, id)` — soft delete via GORM `DeletedAt`.

## Acceptance Criteria
- [ ] `CreateApiKey` returns full record with `KeyHash` populated
- [ ] `FindValidApiKey` returns matching key by raw key comparison
- [ ] `FindValidApiKey` returns `gorm.ErrRecordNotFound` for unknown key
- [ ] `ListApiKeys` returns keys without `KeyHash` field populated
- [ ] `DeleteApiKey` performs soft delete (row still exists in DB)
- [ ] All methods accept `context.Context` for cancellation support

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Create key | name="CI", rawKey="test-key" | `ApiKey{Name:"CI", KeyHash: "$2a$12$..."}` |
| Find valid | rawKey matching stored hash | `*ApiKey`, nil |
| Find invalid | rawKey="wrong" | `ErrRecordNotFound` |
| List keys | 3 existing keys | `[]ApiKey` length 3, no KeyHash |
| Delete key | id=1 | `nil` error, `DeletedAt` set |
