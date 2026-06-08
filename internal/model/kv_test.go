package model

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newKVTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&KV{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestKV_TableName(t *testing.T) {
	k := KV{}
	if got := k.TableName(); got != "kv" {
		t.Fatalf("TableName() = %q, want kv", got)
	}
}

func TestKV_ScopeIndex(t *testing.T) {
	db := newKVTestDB(t)
	if !db.Migrator().HasIndex(&KV{}, "idx_kv_scope") {
		t.Errorf("missing index idx_kv_scope")
	}
}

func TestKV_SetGet(t *testing.T) {
	db := newKVTestDB(t)

	row := KV{Scope: "pricing", Key: "gpt-4", Value: "0.03"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got KV
	if err := db.First(&got, "scope = ? AND `key` = ?", "pricing", "gpt-4").Error; err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Value != "0.03" {
		t.Errorf("value = %q, want 0.03", got.Value)
	}
}

func TestKV_UpsertSameKey(t *testing.T) {
	db := newKVTestDB(t)

	row := KV{Scope: "pricing", Key: "gpt-4", Value: "0.03"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Model(&KV{}).Where("scope = ? AND `key` = ?", "pricing", "gpt-4").Update("value", "0.05").Error; err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var got KV
	if err := db.First(&got, "scope = ? AND `key` = ?", "pricing", "gpt-4").Error; err != nil {
		t.Fatal(err)
	}
	if got.Value != "0.05" {
		t.Errorf("upsert value = %q, want 0.05", got.Value)
	}
}

func TestKV_DuplicatePKFails(t *testing.T) {
	db := newKVTestDB(t)
	first := KV{Scope: "pricing", Key: "gpt-4", Value: "0.03"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	second := KV{Scope: "pricing", Key: "gpt-4", Value: "0.05"}
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

// GetScope returns all (key, value) pairs for a given scope. The repo
// helper (DB-014) is placed here for the test coverage; the actual KV
// repo in internal/repository is created in the future AUTH/PROV flow.
func GetScope(db *gorm.DB, scope string) (map[string]string, error) {
	var rows []KV
	if err := db.Where("scope = ?", scope).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

func TestKV_GetScope(t *testing.T) {
	db := newKVTestDB(t)

	rows := []KV{
		{Scope: "pricing", Key: "gpt-4", Value: "0.03"},
		{Scope: "pricing", Key: "gpt-4-turbo", Value: "0.06"},
		{Scope: "pricing", Key: "claude-3", Value: "0.015"},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	got, err := GetScope(db, "pricing")
	if err != nil {
		t.Fatalf("GetScope: %v", err)
	}
	want := map[string]string{
		"gpt-4":       "0.03",
		"gpt-4-turbo": "0.06",
		"claude-3":    "0.015",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key=%q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestKV_DeleteScope(t *testing.T) {
	db := newKVTestDB(t)

	rows := []KV{
		{Scope: "pricing", Key: "gpt-4", Value: "0.03"},
		{Scope: "aliases", Key: "gpt-4", Value: "gpt4"},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Where("scope = ?", "pricing").Delete(&KV{}).Error; err != nil {
		t.Fatalf("DeleteScope: %v", err)
	}

	var count int64
	db.Model(&KV{}).Where("scope = ?", "pricing").Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows for pricing scope, got %d", count)
	}
}
