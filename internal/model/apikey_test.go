package model

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newApiKeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ApiKey{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestApiKey_TableName(t *testing.T) {
	a := ApiKey{}
	if got := a.TableName(); got != "apiKeys" {
		t.Fatalf("TableName() = %q, want %q", got, "apiKeys")
	}
}

func TestApiKey_UniqueIndexCreated(t *testing.T) {
	db := newApiKeyTestDB(t)
	// GORM auto-names uniqueIndex `key` as `idx_apiKeys_key`
	if !db.Migrator().HasIndex(&ApiKey{}, "idx_apiKeys_key") {
		t.Errorf("missing unique index idx_apiKeys_key on key")
	}
}

func TestApiKey_Create(t *testing.T) {
	db := newApiKeyTestDB(t)

	isActive := true
	a := ApiKey{
		ID:        "ak-1",
		Key:       "sk-abc123",
		Name:      stringPtr("test"),
		IsActive:  &isActive,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ApiKey
	if err := db.First(&got, "id = ?", "ak-1").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Key != "sk-abc123" {
		t.Errorf("key mismatch: %q", got.Key)
	}
}

func TestApiKey_DuplicateKeyFails(t *testing.T) {
	db := newApiKeyTestDB(t)

	isActive := true
	now := time.Now()
	first := ApiKey{ID: "ak-1", Key: "sk-dup", IsActive: &isActive, CreatedAt: now}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("first create: %v", err)
	}

	second := ApiKey{ID: "ak-2", Key: "sk-dup", IsActive: &isActive, CreatedAt: now}
	err := db.Create(&second).Error
	if err == nil {
		t.Fatalf("expected unique constraint error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Logf("got error: %v", err)
		t.Fatalf("expected unique-constraint error, got: %v", err)
	}
}

func TestApiKey_NilBool(t *testing.T) {
	db := newApiKeyTestDB(t)
	a := ApiKey{
		ID:        "ak-nil",
		Key:       "sk-nil",
		IsActive:  nil,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got ApiKey
	if err := db.First(&got, "id = ?", "ak-nil").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.IsActive != nil {
		t.Errorf("IsActive should round-trip as nil, got %v", *got.IsActive)
	}
}

func TestApiKey_FalseBool(t *testing.T) {
	db := newApiKeyTestDB(t)
	isActive := false
	a := ApiKey{
		ID:        "ak-false",
		Key:       "sk-false",
		IsActive:  &isActive,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got ApiKey
	if err := db.First(&got, "id = ?", "ak-false").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.IsActive == nil || *got.IsActive != false {
		t.Errorf("IsActive should be false, got %v", got.IsActive)
	}
}

func TestApiKey_TrueBool(t *testing.T) {
	db := newApiKeyTestDB(t)
	isActive := true
	a := ApiKey{
		ID:        "ak-true",
		Key:       "sk-true",
		IsActive:  &isActive,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got ApiKey
	if err := db.First(&got, "id = ?", "ak-true").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.IsActive == nil || *got.IsActive != true {
		t.Errorf("IsActive should be true, got %v", got.IsActive)
	}
}

// FindValidKey is the model-level repo helper used by the auth middleware
// to look up a key by value AND confirm it is active. A key with IsActive
// set to false OR nil must be filtered out — the caller treats that as a
// not-found (deny).
func FindValidKey(db *gorm.DB, key string) (*ApiKey, error) {
	var a ApiKey
	active := true
	err := db.Where("`key` = ? AND isActive = ?", key, active).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &a, nil
}

func TestApiKey_FindValidKey_Active(t *testing.T) {
	db := newApiKeyTestDB(t)
	isActive := true
	a := ApiKey{ID: "ak-1", Key: "sk-valid", IsActive: &isActive, CreatedAt: time.Now()}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}

	got, err := FindValidKey(db, "sk-valid")
	if err != nil {
		t.Fatalf("expected found, got err: %v", err)
	}
	if got.ID != "ak-1" {
		t.Errorf("got %s, want ak-1", got.ID)
	}
}

func TestApiKey_FindValidKey_Disabled(t *testing.T) {
	db := newApiKeyTestDB(t)
	isActive := false
	a := ApiKey{ID: "ak-dis", Key: "sk-disabled", IsActive: &isActive, CreatedAt: time.Now()}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}

	got, err := FindValidKey(db, "sk-disabled")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got err=%v key=%+v", err, got)
	}
}

func TestApiKey_FindValidKey_NilIsActive(t *testing.T) {
	db := newApiKeyTestDB(t)
	a := ApiKey{ID: "ak-nil", Key: "sk-nil", IsActive: nil, CreatedAt: time.Now()}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}

	got, err := FindValidKey(db, "sk-nil")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound for nil IsActive, got err=%v key=%+v", err, got)
	}
}

func stringPtr(s string) *string { return &s }
