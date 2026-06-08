package model

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB opens an in-memory SQLite GORM instance and runs AutoMigrate
// for the ProviderConnection model. Each call gets a fresh database so
// tests do not share state.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ProviderConnection{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestProviderConnection_TableName(t *testing.T) {
	pc := ProviderConnection{}
	if got := pc.TableName(); got != "providerConnections" {
		t.Fatalf("TableName() = %q, want %q", got, "providerConnections")
	}
}

func TestProviderConnection_IndexesCreated(t *testing.T) {
	db := newTestDB(t)

	want := []string{
		"idx_pc_provider",
		"idx_pc_provider_active",
		"idx_pc_priority",
	}
	for _, idx := range want {
		if !db.Migrator().HasIndex(&ProviderConnection{}, idx) {
			t.Errorf("missing index %q", idx)
		}
	}
}

func TestProviderConnection_CreateMinimal(t *testing.T) {
	db := newTestDB(t)

	pc := ProviderConnection{
		ID:       "test-id-1",
		Provider: "openai",
		AuthType: "apikey",
		Data:     `{"accessToken":"abc"}`,
	}
	pc.CreatedAt = time.Now()
	pc.UpdatedAt = time.Now()

	if err := db.Create(&pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProviderConnection
	if err := db.First(&got, "id = ?", "test-id-1").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Provider != "openai" {
		t.Errorf("provider = %q, want openai", got.Provider)
	}
	if got.AuthType != "apikey" {
		t.Errorf("authType = %q, want apikey", got.AuthType)
	}
	if got.Data != `{"accessToken":"abc"}` {
		t.Errorf("data roundtrip mismatch: %q", got.Data)
	}
}

func TestProviderConnection_CreateFull(t *testing.T) {
	db := newTestDB(t)

	name := "OpenAI Prod"
	email := "ops@example.com"
	priority := 10
	isActive := true
	now := time.Now().UTC().Truncate(time.Second)

	pc := ProviderConnection{
		ID:        "test-id-2",
		Provider:  "openai",
		AuthType:  "apikey",
		Name:      &name,
		Email:     &email,
		Priority:  &priority,
		IsActive:  &isActive,
		Data:      `{"accessToken":"xyz","model":"gpt-4o"}`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProviderConnection
	if err := db.First(&got, "id = ?", "test-id-2").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}

	if got.Provider != "openai" || got.AuthType != "apikey" {
		t.Errorf("core fields mismatch: %+v", got)
	}
	if got.Name == nil || *got.Name != name {
		t.Errorf("name mismatch: %v", got.Name)
	}
	if got.Email == nil || *got.Email != email {
		t.Errorf("email mismatch: %v", got.Email)
	}
	if got.Priority == nil || *got.Priority != priority {
		t.Errorf("priority mismatch: %v", got.Priority)
	}
	if got.IsActive == nil || *got.IsActive != true {
		t.Errorf("isActive mismatch: %v", got.IsActive)
	}
	if got.Data != `{"accessToken":"xyz","model":"gpt-4o"}` {
		t.Errorf("data mismatch: %q", got.Data)
	}
}

func TestProviderConnection_NilBoolean(t *testing.T) {
	db := newTestDB(t)

	pc := ProviderConnection{
		ID:        "test-id-nil",
		Provider:  "openai",
		AuthType:  "apikey",
		Data:      "{}",
		IsActive:  nil,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProviderConnection
	if err := db.First(&got, "id = ?", "test-id-nil").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.IsActive != nil {
		t.Errorf("IsActive should round-trip as nil, got %v", *got.IsActive)
	}
}

func TestProviderConnection_TrueBoolean(t *testing.T) {
	db := newTestDB(t)

	val := true
	pc := ProviderConnection{
		ID:        "test-id-true",
		Provider:  "openai",
		AuthType:  "apikey",
		Data:      "{}",
		IsActive:  &val,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProviderConnection
	if err := db.First(&got, "id = ?", "test-id-true").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.IsActive == nil || *got.IsActive != true {
		t.Errorf("IsActive should be true, got %v", got.IsActive)
	}
}

func TestProviderConnection_FalseBoolean(t *testing.T) {
	db := newTestDB(t)

	val := false
	pc := ProviderConnection{
		ID:        "test-id-false",
		Provider:  "openai",
		AuthType:  "apikey",
		Data:      "{}",
		IsActive:  &val,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProviderConnection
	if err := db.First(&got, "id = ?", "test-id-false").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.IsActive == nil || *got.IsActive != false {
		t.Errorf("IsActive should be false, got %v", got.IsActive)
	}
}

func TestProviderConnection_JSONColumnRoundtrip(t *testing.T) {
	db := newTestDB(t)

	jsonData := `{"accessToken":"abc","refreshToken":"def","scopes":["read","write"]}`
	pc := ProviderConnection{
		ID:        "test-id-json",
		Provider:  "openai",
		AuthType:  "oauth",
		Data:      jsonData,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProviderConnection
	if err := db.First(&got, "id = ?", "test-id-json").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Data != jsonData {
		t.Errorf("data roundtrip mismatch:\n got:  %q\n want: %q", got.Data, jsonData)
	}
}
