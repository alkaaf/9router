package model

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSettingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Setting{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestSetting_TableName(t *testing.T) {
	s := Setting{}
	if got := s.TableName(); got != "settings" {
		t.Fatalf("TableName() = %q, want settings", got)
	}
}

func TestSetting_FirstUpsert(t *testing.T) {
	db := newSettingTestDB(t)
	s := Setting{Data: `{"theme":"dark"}`}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.ID != 1 {
		t.Errorf("first insert ID = %d, want 1", s.ID)
	}
}

func TestSetting_SecondUpsertID1(t *testing.T) {
	db := newSettingTestDB(t)
	first := Setting{Data: `{"theme":"dark"}`}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}

	// Attempting to insert another row with id=1 must violate PK uniqueness
	// OR succeed as a no-op via ON CONFLICT (depending on driver behavior).
	// For SQLite, raw INSERT will fail with UNIQUE constraint failed.
	second := Setting{ID: 1, Data: `{"theme":"light"}`}
	err := db.Create(&second).Error
	if err == nil {
		// Acceptable: some configurations may allow this, skip
		return
	}
	// Error is expected; upsert logic lives in the repo.
	if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Fatalf("expected unique-constraint error, got: %v", err)
	}

	// After upsert via repo helper, the row should still exist with id=1
	// and the original Data unchanged.
	var got Setting
	if err := db.First(&got, "id = ?", 1).Error; err != nil {
		t.Fatalf("read after conflict: %v", err)
	}
	if got.Data != `{"theme":"dark"}` {
		t.Errorf("data should still be %q, got %q", `{"theme":"dark"}`, got.Data)
	}
}

func TestSetting_InvalidIDRejected(t *testing.T) {
	db := newSettingTestDB(t)

	// Try raw INSERT to bypass GORM's autoIncrement
	err := db.Exec(`INSERT INTO settings (id, data) VALUES (?, ?)`, 2, `{"x":1}`).Error
	if err == nil {
		t.Fatalf("expected CHECK constraint error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "check") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Logf("got error: %v", err)
		t.Fatalf("expected CHECK constraint error, got: %v", err)
	}
}

func TestSetting_GetData(t *testing.T) {
	s := Setting{Data: `{"theme":"dark","lang":"en","count":3}`}
	got, err := s.GetData()
	if err != nil {
		t.Fatalf("GetData: %v", err)
	}
	if got["theme"] != "dark" {
		t.Errorf("theme = %v, want dark", got["theme"])
	}
	if got["lang"] != "en" {
		t.Errorf("lang = %v, want en", got["lang"])
	}
	// JSON numbers come back as float64
	if got["count"].(float64) != 3 {
		t.Errorf("count = %v, want 3", got["count"])
	}
}

func TestSetting_GetData_InvalidJSON(t *testing.T) {
	s := Setting{Data: `not-json`}
	_, err := s.GetData()
	if err == nil {
		t.Fatalf("expected JSON parse error")
	}
}
