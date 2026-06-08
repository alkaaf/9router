---
id: DB-006
domain: database
status: DONE
estimate: 30m
title: KV Model
---

## Description
Create the KV GORM model — a composite primary key key-value store used for model aliases, pricing data, custom models, MITM aliases, and disabled models lists.

## Input
Existing Node schema: `scope` (TEXT PK part 1), `key` (TEXT PK part 2), `value` (TEXT NOT NULL).

## Output
`internal/model/kv.go` with the `KV` struct.

```go
type KV struct {
    Scope string `gorm:"primaryKey;type:text;column:scope"`
    Key   string `gorm:"primaryKey;type:text;column:key"`
    Value string `gorm:"not null;type:text;column:value"`
}
```

## Logic
1. Composite primary key `(scope, key)` — two `primaryKey` tags, GORM creates a multi-column PK
2. Both `Scope` and `Key` are TEXT columns matching the existing camelCase schema
3. `Value` is non-nullable TEXT — supports arbitrary string payloads
4. `GetScope` (repo-level) is the most frequently used operation — returns all keys within a scope
5. An index on `scope` alone (`idx_kv_scope`) speeds up `GetScope` and `DeleteScope` queries
6. Register `BeforeCreate` timestamp hooks (DB-011)

## Acceptance Criteria
- [x] Struct compiles and can be instantiated
- [x] GORM `AutoMigrate` on in-memory SQLite succeeds
- [x] Composite PK `(scope, key)` is confirmed via `db.Migrator().PrimaryKey()`
- [x] Insert with duplicate (scope, key) pair fails (PK violation)
- [x] `idx_kv_scope` index is confirmed via `db.Migrator().HasIndex()`
- [x] `GetScope` returns all keys for a given scope in correct order

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Set key-value | Scope="pricing", Key="gpt-4", Value="0.03" | Row created |
| Get by scope+key | Scope="pricing", Key="gpt-4" | Returns `"0.03"` |
| Upsert same key | Same scope+key, new value | Value updated, no error |
| GetScope | Scope="pricing" with 3 keys | Returns map of all 3 key-value pairs |
| DeleteScope | Scope="pricing" | All rows with that scope removed |
| Composite PK violation | Same (scope,key) twice | Second insert returns PK violation error |

## Agent Log
- Started: 2026-06-04
- Completed: 2026-06-04
- Agent: agent-db
- AC-001 verified: go build ./internal/model/ passes
- AC-002 verified: AutoMigrate succeeds on :memory: SQLite (7/7 tests PASS)
- AC-003 verified: dual primaryKey tags yield composite PK on (scope, key) — duplicate insertion fails with "UNIQUE constraint failed: kv.scope, kv.key"
- AC-004 verified: TestKV_DuplicatePKFails PASS
- AC-005 verified: TestKV_ScopeIndex — db.Migrator().HasIndex(&KV{}, "idx_kv_scope") returns true (added `index:idx_kv_scope,priority:1` tag on Scope)
- AC-006 verified: TestKV_GetScope returns all 3 keys in the pricing scope

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓ (7/7 PASS)
- Code location: internal/model/kv.go + internal/model/kv_test.go
