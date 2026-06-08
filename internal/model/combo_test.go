package model

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newComboTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Combo{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestCombo_TableName(t *testing.T) {
	c := Combo{}
	if got := c.TableName(); got != "combos" {
		t.Fatalf("TableName() = %q, want %q", got, "combos")
	}
}

func TestCombo_UniqueIndexCreated(t *testing.T) {
	db := newComboTestDB(t)
	// GORM auto-names uniqueIndex `name` as `idx_combos_name`
	if !db.Migrator().HasIndex(&Combo{}, "idx_combos_name") {
		t.Errorf("missing unique index idx_combos_name on name")
	}
}

func TestCombo_CreateBasic(t *testing.T) {
	db := newComboTestDB(t)

	c := Combo{
		ID:        "combo-1",
		Name:      "my-combo",
		Kind:      nil,
		Models:    `["gpt-4"]`,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got Combo
	if err := db.First(&got, "id = ?", "combo-1").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Name != "my-combo" {
		t.Errorf("name mismatch: %q", got.Name)
	}
	if got.Models != `["gpt-4"]` {
		t.Errorf("models mismatch: %q", got.Models)
	}
}

func TestCombo_DuplicateNameFails(t *testing.T) {
	db := newComboTestDB(t)

	now := time.Now()
	first := Combo{
		ID:        "combo-a",
		Name:      "duplicate-name",
		Models:    `["gpt-4"]`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("first create: %v", err)
	}

	second := Combo{
		ID:        "combo-b",
		Name:      "duplicate-name",
		Models:    `["claude-3"]`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.Create(&second).Error
	if err == nil {
		t.Fatalf("expected unique constraint error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Logf("got error: %v", err)
		t.Fatalf("expected unique-constraint-style error, got: %v", err)
	}
}

func TestCombo_JSONArrayRoundtrip(t *testing.T) {
	db := newComboTestDB(t)

	modelsJSON := `["a","b","c"]`
	c := Combo{
		ID:        "combo-2",
		Name:      "json-combo",
		Models:    modelsJSON,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got Combo
	if err := db.First(&got, "id = ?", "combo-2").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Models != modelsJSON {
		t.Errorf("models roundtrip mismatch:\n got:  %q\n want: %q", got.Models, modelsJSON)
	}
}

func TestCombo_NilKind(t *testing.T) {
	db := newComboTestDB(t)

	c := Combo{
		ID:        "combo-3",
		Name:      "no-kind",
		Kind:      nil,
		Models:    `["gpt-4"]`,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got Combo
	if err := db.First(&got, "id = ?", "combo-3").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Kind != nil {
		t.Errorf("Kind should be nil, got %v", *got.Kind)
	}
}
