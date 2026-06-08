---
id: COMBO-002
domain: combos
status: TODO
estimate: 1h
title: Create Combo Repository
---

## Description

Create `internal/repository/combo.go` exposing `ComboRepository` with `FindAll() ([]model.Combo, error)`, `FindByID(id string) (*model.Combo, error)`, `FindByName(name string) (*model.Combo, error)`, `Create(*model.Combo) error`, `Update(id string, fields map[string]any) (*model.Combo, error)`, `Delete(id string) (bool, error)`. Match the Node.js behavior in `combosRepo.js`: list is `ORDER BY createdAt ASC`; updates merge existing row with supplied fields and bump `updatedAt`; deletes return whether a row was affected.

## Input

Repository methods receive primitive args; `Update` accepts a `map[string]any` of partial fields.

## Output

GORM result + domain error, with `gorm.ErrRecordNotFound` translated to `nil` entity (no sentinel — the handler decides 404).

## Logic

- `FindAll()`: SELECT all combos ORDER BY createdAt ASC
- `FindByID(id)`: SELECT combo WHERE id = ?
- `FindByName(name)`: SELECT combo WHERE name = ?
- `Create(combo)`: INSERT new combo, set createdAt/updatedAt
- `Update(id, fields)`: UPDATE existing row with merged fields, bump updatedAt
- `Delete(id)`: DELETE row, return whether row was affected

## Acceptance Criteria
- [ ] `FindAll` returns combos in `createdAt ASC` order
- [ ] `FindByName` returns the right row; case sensitivity preserved
- [ ] `Update` with partial fields preserves untouched columns
- [ ] `Delete` returns `(true, nil)` for existing, `(false, nil)` for missing
- [ ] `gorm.ErrRecordNotFound` translated to nil entity

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| FindAll empty DB | No combos | `[]model.Combo{}` |
| FindAll with 3 combos | 3 seeded combos | 3 combos in createdAt ASC order |
| FindByName exists | "my-combo" | Matching combo returned |
| FindByName not exists | "nonexistent" | `nil, nil` |
| Update partial fields | ID + `{"models": [...]}` | Other fields preserved |
| Delete exists | Valid ID | `(true, nil)` |
| Delete not exists | Invalid ID | `(false, nil)` |