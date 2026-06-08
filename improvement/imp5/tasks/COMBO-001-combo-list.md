---
id: COMBO-001
domain: combos
status: TODO
estimate: 1h
title: Define Combo GORM Model
---

## Description

Create `internal/model/combo.go` with the `Combo` struct mirroring the SQLite `combos` table (id TEXT PK, name TEXT UNIQUE NOT NULL, kind *string, models TEXT NOT NULL storing JSON array, createdAt/updatedAt time.Time). Provide typed accessors `GetModels() ([]string, error)` and `SetModels([]string) error` that marshal/unmarshal the `models` JSON column. Use `gorm:"type:text"` and an `uniqueIndex` on Name to match the existing constraint.

## Input

None (compile-time definition)

## Output

`model.Combo` struct with GORM tags, satisfies `BeforeCreate` hook generating a UUID v4 ID when empty.

## Logic

- Combo struct mirrors: `id TEXT PRIMARY KEY`, `name TEXT UNIQUE NOT NULL`, `kind *string`, `models TEXT NOT NULL` (JSON array), `createdAt/updatedAt time.Time`
- `GetModels()` unmarshals `models` JSON column into `[]string`
- `SetModels()` marshals `[]string` into `models` JSON column
- `BeforeCreate` hook generates UUID v4 for ID when empty

## Acceptance Criteria
- [ ] Combo struct has all required fields with correct GORM tags
- [ ] `GetModels()` returns empty slice for empty string, original slice for round-trip
- [ ] `SetModels()` produces valid JSON; rejects nil/empty without panic
- [ ] `BeforeCreate` sets a non-empty UUID v4 ID
- [ ] `AutoMigrate(&Combo{})` against in-memory SQLite creates table with unique index

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| GetModels with empty string | `models = ""` | `[]string{}` |
| GetModels round-trip | `models = "[\"gpt-4o\",\"claude\"]"` | `["gpt-4o", "claude"]` |
| SetModels produces valid JSON | `["a", "b"]` | `"[\"a\",\"b\"]"` |
| SetModels with nil | `nil` | Error returned, no panic |
| BeforeCreate sets ID | Empty ID | UUID v4 ID set |
| AutoMigrate creates table | `AutoMigrate(&Combo{})` | Table created with unique index |