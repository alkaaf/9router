---
id: DB-002
domain: database
status: DONE
estimate: 45m
title: Combo Model
---

## Description
Create the Combo GORM model — model combo definitions with a unique name constraint and a JSON array column storing the list of model identifiers.

## Input
Existing Node schema: `id` (TEXT PK), `name` (TEXT unique), `kind` (TEXT nullable), `models` (TEXT/JSONB JSON array), `createdAt` (timestamp), `updatedAt` (timestamp).

## Output
`internal/model/combo.go` with the `Combo` struct.

```go
type Combo struct {
    ID        string    `gorm:"primaryKey;type:text;column:id"`
    Name      string    `gorm:"not null;uniqueIndex;type:text;column:name"`
    Kind      *string   `gorm:"column:kind"`
    Models    string    `gorm:"not null;type:text;column:models"`
    CreatedAt time.Time `gorm:"column:createdAt"`
    UpdatedAt time.Time `gorm:"column:updatedAt"`
}
```

## Logic
1. Define struct with `column:` GORM tags matching existing camelCase schema
2. `Name` has `uniqueIndex` — GORM creates a UNIQUE constraint on the `name` column
3. `Models` stores a JSON array (e.g. `["gpt-4","claude-3-opus"]`) as a TEXT/JSONB string — typed access via `ComboModelsData` in JSON helpers (DB-014)
4. Register `BeforeCreate` hook for UUID generation and timestamp population
5. Register `BeforeUpdate` hook to refresh `UpdatedAt`

## Acceptance Criteria
- [x] Struct compiles and can be instantiated
- [x] GORM `AutoMigrate` on in-memory SQLite succeeds
- [x] `uniqueIndex` on `name` is confirmed via `db.Migrator().HasIndex()`
- [x] Duplicate name insert returns a unique constraint violation
- [x] JSON array roundtrip in `Models` column succeeds

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Create combo | Name="my-combo", Models=`["gpt-4"]` | Row inserted with generated ID |
| Duplicate name | Same Name inserted twice | Second insert returns unique constraint error |
| JSON array roundtrip | Models=`["a","b","c"]` | Retrieved `Models` field equals original JSON |
| Nil Kind | Kind=nil | Stored as null, no error |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-db
- AC-001 verified: go build ./internal/model/ passes
- AC-002 verified: gorm.AutoMigrate on :memory: SQLite succeeds (5/5 tests PASS)
- AC-003 verified: db.Migrator().HasIndex(&Combo{}, "idx_combos_name") returns true
- AC-004 verified: inserting duplicate name returns "UNIQUE constraint failed: combos.name" — TestCombo_DuplicateNameFails PASS
- AC-005 verified: TestCombo_JSONArrayRoundtrip PASS — ["a","b","c"] round-trips exactly

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/model/combo.go + internal/model/combo_test.go
