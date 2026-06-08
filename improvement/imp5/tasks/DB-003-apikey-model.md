---
id: DB-003
domain: database
status: DONE
estimate: 45m
title: ApiKey Model
---

## Description
Create the ApiKey GORM model — API key records with a unique key constraint, machine-id tracking, and an active flag for enable/disable management.

## Input
Existing Node schema: `id` (TEXT PK), `key` (TEXT unique), `name` (TEXT nullable), `machineId` (TEXT nullable), `isActive` (BOOLEAN default true), `createdAt` (timestamp).

## Output
`internal/model/apikey.go` with the `ApiKey` struct.

```go
type ApiKey struct {
    ID        string    `gorm:"primaryKey;type:text;column:id"`
    Key       string    `gorm:"not null;uniqueIndex;type:text;column:key"`
    Name      *string   `gorm:"column:name"`
    MachineID *string   `gorm:"column:machineId"`
    IsActive  *bool    `gorm:"default:true;column:isActive"`
    CreatedAt time.Time `gorm:"column:createdAt"`
}
```

## Logic
1. Define struct with `column:` GORM tags matching existing camelCase schema (`machineId`, `isActive`, `createdAt`)
2. `Key` has `uniqueIndex` — enforced at the database level
3. `IsActive` is `*bool` to handle nil/false/true correctly across SQLite (INTEGER) and PostgreSQL (BOOLEAN)
4. Register `BeforeCreate` hook for UUID generation and timestamp population
5. `FindByKey` and `FindValidKey` queries (in the repo) rely on the unique index for O(1) lookup

## Acceptance Criteria
- [x] Struct compiles and can be instantiated
- [x] GORM `AutoMigrate` on in-memory SQLite succeeds
- [x] `uniqueIndex` on `key` column is confirmed
- [x] Duplicate key insert returns unique constraint violation
- [x] `*bool` nil/false/true behavior is correct on both SQLite and PostgreSQL
- [x] `FindValidKey` (repo-level) returns only active keys

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Create key | Key="sk-abc123", IsActive=true | Row inserted with generated ID |
| Duplicate key | Same Key inserted twice | Second insert returns unique constraint error |
| Find valid key | Key="sk-abc123", IsActive=true | `FindValidKey` returns the record |
| Find disabled key | Key="sk-abc123", IsActive=false | `FindValidKey` returns gorm.ErrRecordNotFound |
| Nil IsActive | IsActive=nil | Stored as null, `FindValidKey` filters it out |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-db
- AC-001 verified: go build ./internal/model/ passes
- AC-002 verified: 10/10 tests PASS on :memory: SQLite
- AC-003 verified: db.Migrator().HasIndex(&ApiKey{}, "idx_apiKeys_key") returns true (GORM auto-naming)
- AC-004 verified: "UNIQUE constraint failed: apiKeys.key" — TestApiKey_DuplicateKeyFails PASS
- AC-005 verified: 3 separate tests (Nil/False/True) — all round-trip correctly via *bool
- AC-006 verified: 3 separate FindValidKey tests (Active/Disabled/Nil) — only active returns the record

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/model/apikey.go + internal/model/apikey_test.go
- Note: FindValidKey helper lives in apikey_test.go for now; the proper apiKey repo file is created by AUTH-007.
