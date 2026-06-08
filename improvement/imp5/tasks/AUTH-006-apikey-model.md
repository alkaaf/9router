---
id: AUTH-006
domain: auth
status: DONE
estimate: 1h
title: ApiKey GORM Model
---

## Description
Define the GORM model for storing API keys in the database. API keys are stored as bcrypt hashes (never raw). The model includes metadata for key identification and lifecycle tracking.

## Input
No direct input parameters; this is a GORM model definition consumed by the repository layer.

## Output
- `ApiKey` struct: `ID uint`, `KeyHash string`, `Name string`, `LastUsedAt *time.Time`, `CreatedAt time.Time`, `UpdatedAt time.Time`, `DeletedAt gorm.DeletedAt`

## Logic
1. Define `ApiKey` struct with GORM tags mapping to `api_keys` table.
2. `KeyHash` stores the bcrypt hash of the raw API key — never store raw.
3. `Name` is a human-readable label (e.g. "CI Pipeline", "CLI Tool").
4. `LastUsedAt` tracks when the key was last validated (nullable).
5. Embed GORM's `DeletedAt` for soft deletes.
6. Export `TableName()` method returning `"api_keys"`.

## Acceptance Criteria
- [ ] `gorm.Model` fields (`ID`, `CreatedAt`, `UpdatedAt`, `DeletedAt`) present
- [ ] `KeyHash` column is `text` type (bcrypt hash is ~60 chars)
- [ ] `Name` column has max length 255
- [ ] `LastUsedAt` is nullable
- [ ] `TableName()` returns `"api_keys"`
- [ ] `go vet` and `golangci-lint` pass

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Model instantiation | `ApiKey{Name:"test",KeyHash:"$2a$12$..."}` | Struct compiles, fields accessible |
| Auto-migration | `db.AutoMigrate(&ApiKey{})` | Creates `api_keys` table with all columns |
| Soft delete | `db.Delete(key)` | `DeletedAt` populated, row hidden by default |
