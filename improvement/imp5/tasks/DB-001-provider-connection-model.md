---
id: DB-001
domain: database
status: DONE
estimate: 1h
title: ProviderConnection Model
---

## Description
Create the ProviderConnection GORM model — the most complex table with boolean fields, a JSON data column, and three composite indexes.

## Input
Existing Node schema from `schema.js` / `schema.postgres.js`: columns `id` (TEXT PK), `provider` (TEXT), `authType` (TEXT), `name` (TEXT nullable), `email` (TEXT nullable), `priority` (INTEGER nullable), `isActive` (BOOLEAN default true), `data` (TEXT/JSONB), timestamps.

## Output
`internal/model/provider_connection.go` with the `ProviderConnection` struct.

```go
type ProviderConnection struct {
    ID         string    `gorm:"primaryKey;type:text;column:id"`
    Provider   string    `gorm:"not null;column:provider;index:idx_pc_provider;index:idx_pc_provider_active,priority:1;index:idx_pc_priority,priority:1"`
    AuthType   string    `gorm:"not null;column:authType"`
    Name       *string   `gorm:"column:name"`
    Email      *string   `gorm:"column:email"`
    Priority   *int     `gorm:"column:priority"`
    IsActive   *bool    `gorm:"default:true;column:isActive"`
    Data       string   `gorm:"not null;type:text;column:data"`
    CreatedAt  time.Time `gorm:"column:createdAt"`
    UpdatedAt  time.Time `gorm:"column:updatedAt"`
}
```

## Logic
1. Define struct with `column:` GORM tags matching the existing camelCase schema (`authType`, `isActive`, `createdAt`, etc.)
2. Three composite indexes: `idx_pc_provider(provider)`, `idx_pc_provider_active(provider, isActive)`, `idx_pc_priority(provider, priority)`
3. Use `*bool` for `IsActive` so GORM handles SQLite INTEGER 0/1 and PostgreSQL BOOLEAN transparently
4. `Data` is stored as a TEXT/JSONB string — typed access via JSON helpers (DB-014)
5. Register `BeforeCreate` hook (DB-011) for UUID generation and timestamp population

## Acceptance Criteria
- [x] Struct compiles and can be instantiated
- [x] GORM `AutoMigrate` on in-memory SQLite succeeds without errors
- [x] `db.Migrator().HasIndex("providerConnections", "idx_pc_provider")` returns true for all three indexes
- [x] `db.Migrator().HasIndex("providerConnections", "idx_pc_provider_active")` returns true
- [x] `db.Migrator().HasIndex("providerConnections", "idx_pc_priority")` returns true
- [x] `*bool` nil vs false vs true persist correctly and are read back correctly
- [x] Table name resolves to `providerConnections` (not snake_case override needed)

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Create minimal record | provider="openai", authType="apikey" | Row inserted, UUID generated for ID |
| Create full record | all fields set | All fields persisted and retrieved |
| Nil boolean | IsActive=nil | Stored as DB null, read back as nil |
| True boolean | IsActive=&trueVal | Stored as true/1, read back correctly |
| Index existence | AutoMigrate complete | All 3 composite indexes confirmed |
| JSON column roundtrip | Data=`{"accessToken":"abc"}` | Stored and retrieved as identical JSON string |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-db
- Notes:
  - Bootstrapped missing Go rewrite scaffolding (`go.mod` was present; `go.sum`, `internal/model/`, `internal/repository/` did not exist) by running `go get gorm.io/gorm gorm.io/driver/sqlite gorm.io/driver/postgres` and creating the directories.
  - `IsActive` was specified as `*bool` with `default:true` tag. SQLite + GORM coerces NULL → 1 on read, breaking nil roundtrip. Removed `default:true` from the struct tag; the application-level default is handled by the DB-011 BeforeCreate hook.
  - All 8 test cases pass on `:memory:` SQLite (PASS, 0.43s).
  - `go build ./internal/model/` and `go vet ./internal/model/` both clean.
- AC-001 verified: `go build ./internal/model/` exits 0
- AC-002 verified: `db.AutoMigrate(&ProviderConnection{})` on `:memory:` SQLite returns nil — see TestProviderConnection_CreateMinimal
- AC-003 verified: `TestProviderConnection_IndexesCreated` checks all 3 indexes via `db.Migrator().HasIndex` — all PASS
- AC-004 verified: same
- AC-005 verified: same
- AC-006 verified: `TestProviderConnection_NilBoolean` + `TestProviderConnection_TrueBoolean` + `TestProviderConnection_FalseBoolean` all PASS — nil persists as NULL, true/false round-trip exactly
- AC-007 verified: `TestProviderConnection_TableName` returns "providerConnections" — explicit `TableName()` method also pins it

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (8/8 PASS)
- Code location: `internal/model/provider_connection.go` (model) + `internal/model/provider_connection_test.go` (tests)
- Deviation: removed `default:true` from `IsActive` GORM tag to keep nil roundtrip working — `*bool` + `nil` is the source of truth, application default handled in DB-011 BeforeCreate.
