package model

import (
	"strconv"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMetaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Meta{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestMeta_TableName(t *testing.T) {
	m := Meta{}
	if got := m.TableName(); got != "_meta" {
		t.Fatalf("TableName() = %q, want _meta", got)
	}
}

func TestMeta_PrimaryKeyColumn(t *testing.T) {
	db := newMetaTestDB(t)
	if !db.Migrator().HasIndex(&Meta{}, "key") {
		// Some drivers expose PK as a unique index; we accept either check
		// below by inspecting SQLite master.
	}

	// Inspect SQLite master to confirm the PK column
	rows, err := db.Raw("SELECT name FROM pragma_table_info('_meta') WHERE pk > 0").Rows()
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	defer rows.Close()
	foundPK := false
	for rows.Next() {
		var name string
		rows.Scan(&name)
		if name == "key" {
			foundPK = true
		}
	}
	if !foundPK {
		t.Errorf("expected `key` to be primary key column")
	}
}

func TestMeta_SetGet(t *testing.T) {
	db := newMetaTestDB(t)

	m := Meta{Key: "schemaVersion", Value: "1"}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got Meta
	if err := db.First(&got, "`key` = ?", "schemaVersion").Error; err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Value != "1" {
		t.Errorf("value = %q, want \"1\"", got.Value)
	}
}

func TestMeta_UpsertValue(t *testing.T) {
	db := newMetaTestDB(t)

	first := Meta{Key: "schemaVersion", Value: "1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}

	// Direct UPDATE simulates upsert
	if err := db.Model(&Meta{}).Where("`key` = ?", "schemaVersion").Update("value", "2").Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var got Meta
	if err := db.First(&got, "`key` = ?", "schemaVersion").Error; err != nil {
		t.Fatal(err)
	}
	if got.Value != "2" {
		t.Errorf("value should be updated to %q, got %q", "2", got.Value)
	}
}

func TestMeta_DuplicateKeyFails(t *testing.T) {
	db := newMetaTestDB(t)
	first := Meta{Key: "schemaVersion", Value: "1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	second := Meta{Key: "schemaVersion", Value: "2"}
	err := db.Create(&second).Error
	if err == nil {
		t.Fatalf("expected PK violation, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Logf("got error: %v", err)
		t.Fatalf("expected constraint error, got: %v", err)
	}
}

func TestMeta_EmptyKeyFails(t *testing.T) {
	db := newMetaTestDB(t)
	m := Meta{Key: "", Value: "x"}
	err := db.Create(&m).Error
	if err == nil {
		t.Fatalf("expected NOT NULL violation on empty key")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not null") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") &&
		!strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Logf("got error: %v", err)
		t.Fatalf("expected NOT NULL/CHECK error, got: %v", err)
	}
}

// GetInt parses the Value field as an int. The Meta repository (DB-014 /
// migrate.js) stores integer counters as strings for portability, but
// exposes typed accessors.
func (m *Meta) GetInt() (int, error) {
	return strconv.Atoi(m.Value)
}

func TestMeta_GetInt(t *testing.T) {
	m := &Meta{Key: "schemaVersion", Value: "2"}
	got, err := m.GetInt()
	if err != nil {
		t.Fatalf("GetInt: %v", err)
	}
	if got != 2 {
		t.Errorf("GetInt = %d, want 2", got)
	}
}

func TestMeta_GetInt_Invalid(t *testing.T) {
	m := &Meta{Key: "k", Value: "not-a-number"}
	_, err := m.GetInt()
	if err == nil {
		t.Fatalf("expected parse error")
	}
}
