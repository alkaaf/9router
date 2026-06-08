package repository

import (
	"testing"

	"github.com/9router/9router/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestGormDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestNewRepositories_AllFieldsNonNil(t *testing.T) {
	db := newTestGormDB(t)
	repos := NewRepositories(db)

	if repos.Provider == nil {
		t.Error("Provider nil")
	}
	if repos.ProviderNode == nil {
		t.Error("ProviderNode nil")
	}
	if repos.ProxyPool == nil {
		t.Error("ProxyPool nil")
	}
	if repos.ApiKey == nil {
		t.Error("ApiKey nil")
	}
	if repos.Combo == nil {
		t.Error("Combo nil")
	}
	if repos.Setting == nil {
		t.Error("Setting nil")
	}
	if repos.Meta == nil {
		t.Error("Meta nil")
	}
	if repos.KV == nil {
		t.Error("KV nil")
	}
	if repos.Usage == nil {
		t.Error("Usage nil")
	}
	if repos.UsageDaily == nil {
		t.Error("UsageDaily nil")
	}
	if repos.RequestDetail == nil {
		t.Error("RequestDetail nil")
	}
}

func TestNewRepositories_SharedDB(t *testing.T) {
	db := newTestGormDB(t)
	repos := NewRepositories(db)

	if repos.Provider.DB() != repos.ApiKey.DB() {
		t.Error("Provider.DB != ApiKey.DB")
	}
	if repos.Combo.DB() != repos.RequestDetail.DB() {
		t.Error("Combo.DB != RequestDetail.DB")
	}
	if repos.DB() != db {
		t.Error("Repositories.DB() != db")
	}
}

func TestNewRepositories_NilDBPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewRepositories(nil)
}

func TestRepositories_Close(t *testing.T) {
	db := newTestGormDB(t)
	repos := NewRepositories(db)

	if err := repos.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRepositories_EndToEnd(t *testing.T) {
	db := newTestGormDB(t)
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("AutoMigrateAll: %v", err)
	}
	repos := NewRepositories(db)

	got, err := repos.Provider.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d", len(got))
	}
}

func TestBaseRepository_Count(t *testing.T) {
	db := newTestGormDB(t)
	if err := AutoMigrateAll(db); err != nil {
		t.Fatal(err)
	}
	repos := NewRepositories(db)

	n, err := repos.Provider.Count(&model.ProviderConnection{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}
