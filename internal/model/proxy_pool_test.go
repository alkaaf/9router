package model

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newProxyPoolTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ProxyPool{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestProxyPool_TableName(t *testing.T) {
	p := ProxyPool{}
	if got := p.TableName(); got != "proxyPools" {
		t.Fatalf("TableName() = %q, want proxyPools", got)
	}
}

func TestProxyPool_Indexes(t *testing.T) {
	db := newProxyPoolTestDB(t)
	for _, idx := range []string{"idx_pp_active", "idx_pp_status"} {
		if !db.Migrator().HasIndex(&ProxyPool{}, idx) {
			t.Errorf("missing index %q", idx)
		}
	}
}

func TestProxyPool_CreateActive(t *testing.T) {
	db := newProxyPoolTestDB(t)

	active := true
	status := "pass"
	now := time.Now().UTC()
	p := ProxyPool{
		ID:         "pp-1",
		IsActive:   &active,
		TestStatus: &status,
		Data:       `{"host":"1.2.3.4","port":8080}`,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProxyPool
	if err := db.First(&got, "id = ?", "pp-1").Error; err != nil {
		t.Fatal(err)
	}
	if got.IsActive == nil || *got.IsActive != true {
		t.Errorf("isActive = %v, want true", got.IsActive)
	}
}

// FindActive returns all pools with IsActive = true (excluding nil).
// Implemented here for test coverage; the proper repo function lives in
// internal/repository/proxy_pool.go (SYS-016).
func FindActivePools(db *gorm.DB) ([]ProxyPool, error) {
	var out []ProxyPool
	if err := db.Where("isActive = ?", true).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func TestProxyPool_FindActive(t *testing.T) {
	db := newProxyPoolTestDB(t)

	now := time.Now().UTC()
	active := true
	inactive := false
	statusPass := "pass"
	statusFail := "fail"

	rows := []ProxyPool{
		{ID: "pp-1", IsActive: &active, TestStatus: &statusPass, Data: "{}", CreatedAt: now, UpdatedAt: now},
		{ID: "pp-2", IsActive: &active, TestStatus: &statusPass, Data: "{}", CreatedAt: now, UpdatedAt: now},
		{ID: "pp-3", IsActive: &active, TestStatus: &statusFail, Data: "{}", CreatedAt: now, UpdatedAt: now},
		{ID: "pp-4", IsActive: &inactive, TestStatus: &statusPass, Data: "{}", CreatedAt: now, UpdatedAt: now},
		{ID: "pp-5", IsActive: &inactive, TestStatus: &statusFail, Data: "{}", CreatedAt: now, UpdatedAt: now},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	got, err := FindActivePools(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 active pools, got %d", len(got))
	}
}

// FindByStatus returns all pools with the given testStatus.
func FindByTestStatus(db *gorm.DB, status string) ([]ProxyPool, error) {
	var out []ProxyPool
	if err := db.Where("testStatus = ?", status).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func TestProxyPool_FindByStatus(t *testing.T) {
	db := newProxyPoolTestDB(t)

	now := time.Now().UTC()
	active := true
	pass := "pass"
	fail := "fail"
	pending := "pending"

	rows := []ProxyPool{
		{ID: "pp-1", IsActive: &active, TestStatus: &pass, Data: "{}", CreatedAt: now, UpdatedAt: now},
		{ID: "pp-2", IsActive: &active, TestStatus: &pass, Data: "{}", CreatedAt: now, UpdatedAt: now},
		{ID: "pp-3", IsActive: &active, TestStatus: &fail, Data: "{}", CreatedAt: now, UpdatedAt: now},
		{ID: "pp-4", IsActive: nil, TestStatus: &pending, Data: "{}", CreatedAt: now, UpdatedAt: now},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	failed, err := FindByTestStatus(db, "fail")
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed pool, got %d", len(failed))
	}

	passed, _ := FindByTestStatus(db, "pass")
	if len(passed) != 2 {
		t.Errorf("expected 2 passed pools, got %d", len(passed))
	}
}

func TestProxyPool_ToggleActive(t *testing.T) {
	db := newProxyPoolTestDB(t)

	active := true
	now := time.Now().UTC()
	p := ProxyPool{
		ID:        "pp-toggle",
		IsActive:  &active,
		Data:      "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}

	// Toggle to false
	inactive := false
	if err := db.Model(&p).Update("isActive", inactive).Error; err != nil {
		t.Fatal(err)
	}

	got, _ := FindActivePools(db)
	for _, g := range got {
		if g.ID == "pp-toggle" {
			t.Errorf("pp-toggle should not be active after toggle")
		}
	}
}

func TestProxyPool_JSONRoundtrip(t *testing.T) {
	db := newProxyPoolTestDB(t)

	active := true
	now := time.Now().UTC()
	data := `{"host":"1.2.3.4","port":8080,"auth":{"user":"u","pass":"p"},"rotateEvery":300}`
	p := ProxyPool{
		ID:        "pp-json",
		IsActive:  &active,
		Data:      data,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}

	var got ProxyPool
	if err := db.First(&got, "id = ?", "pp-json").Error; err != nil {
		t.Fatal(err)
	}
	if got.Data != data {
		t.Errorf("data roundtrip mismatch:\n got:  %q\n want: %q", got.Data, data)
	}
}
