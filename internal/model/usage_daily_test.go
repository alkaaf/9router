package model

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newDailyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&UsageDaily{},
		&UsageDailyByProvider{},
		&UsageDailyByModel{},
		&UsageDailyByApiKey{},
		&UsageDailyByAccount{},
		&UsageDailyByEndpoint{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestUsageDaily_TableName(t *testing.T) {
	u := UsageDaily{}
	if got := u.TableName(); got != "usageDaily" {
		t.Errorf("TableName() = %q, want usageDaily", got)
	}
}

func TestUsageDaily_DateKeyFormat(t *testing.T) {
	tt := time.Date(2025, 6, 4, 0, 0, 0, 0, time.UTC)
	got := DateKeyFrom(tt)
	if got != "2025-06-04" {
		t.Errorf("DateKeyFrom = %q, want 2025-06-04", got)
	}

	// Should also normalize to UTC even if non-UTC passed in
	loc, _ := time.LoadLocation("Asia/Jakarta")
	jkt := tt.In(loc)
	if got := DateKeyFrom(jkt); got != "2025-06-04" {
		t.Errorf("DateKeyFrom non-UTC = %q, want 2025-06-04", got)
	}
}

func TestUsageDaily_Create(t *testing.T) {
	db := newDailyTestDB(t)
	u := UsageDaily{DateKey: "2025-06-04", Data: `{"totalCost":1.50}`}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got UsageDaily
	if err := db.First(&got, "dateKey = ?", "2025-06-04").Error; err != nil {
		t.Fatal(err)
	}
	if got.Data != `{"totalCost":1.50}` {
		t.Errorf("data = %q, want %q", got.Data, `{"totalCost":1.50}`)
	}
}

func TestUsageDailyByProvider_TableName(t *testing.T) {
	u := UsageDailyByProvider{}
	if got := u.TableName(); got != "usageDailyByProvider" {
		t.Errorf("TableName() = %q, want usageDailyByProvider", got)
	}
}

func TestUsageDailyByProvider_CreateAndRollupCols(t *testing.T) {
	db := newDailyTestDB(t)

	d := time.Date(2025, 6, 4, 0, 0, 0, 0, time.UTC)
	u := UsageDailyByProvider{
		Date:         d,
		Provider:     "openai",
		RequestCount: 100,
		InputTokens:  5000,
		OutputTokens: 2500,
		TotalTokens:  7500,
		Cost:         0.50,
		UpdatedAt:    time.Now().UTC(),
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got UsageDailyByProvider
	if err := db.First(&got, "date = ? AND provider = ?", d, "openai").Error; err != nil {
		t.Fatal(err)
	}
	if got.RequestCount != 100 || got.InputTokens != 5000 || got.OutputTokens != 2500 || got.TotalTokens != 7500 {
		t.Errorf("int64 columns mismatch: %+v", got)
	}
	if got.Cost != 0.50 {
		t.Errorf("cost = %f, want 0.50", got.Cost)
	}
}

func TestUsageDailyByProvider_UpsertOnConflict(t *testing.T) {
	db := newDailyTestDB(t)

	d := time.Date(2025, 6, 4, 0, 0, 0, 0, time.UTC)
	first := UsageDailyByProvider{
		Date: d, Provider: "openai", RequestCount: 100, Cost: 0.50,
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}

	// Update via GORM Model.Update — simulates ON CONFLICT DO UPDATE
	if err := db.Model(&UsageDailyByProvider{}).
		Where("date = ? AND provider = ?", d, "openai").
		Updates(map[string]interface{}{
			"requestCount": 200,
			"cost":         1.00,
			"updatedAt":    time.Now().UTC(),
		}).Error; err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	var got UsageDailyByProvider
	db.First(&got, "date = ? AND provider = ?", d, "openai")
	if got.RequestCount != 200 {
		t.Errorf("after upsert, requestCount = %d, want 200", got.RequestCount)
	}
	if got.Cost != 1.00 {
		t.Errorf("after upsert, cost = %f, want 1.00", got.Cost)
	}
}

func TestUsageDailyByProvider_DuplicatePKFails(t *testing.T) {
	db := newDailyTestDB(t)

	d := time.Date(2025, 6, 4, 0, 0, 0, 0, time.UTC)
	first := UsageDailyByProvider{Date: d, Provider: "openai", UpdatedAt: time.Now().UTC()}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	second := UsageDailyByProvider{Date: d, Provider: "openai", UpdatedAt: time.Now().UTC()}
	err := db.Create(&second).Error
	if err == nil {
		t.Fatalf("expected PK violation, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Logf("got error: %v", err)
		t.Fatalf("expected unique constraint error, got: %v", err)
	}
}

func TestUsageDaily_AllTablesCreated(t *testing.T) {
	db := newDailyTestDB(t)

	wantTables := []string{
		"usageDaily",
		"usageDailyByProvider",
		"usageDailyByModel",
		"usageDailyByApiKey",
		"usageDailyByAccount",
		"usageDailyByEndpoint",
	}
	for _, tbl := range wantTables {
		// Use SQLite master to verify
		var count int64
		db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&count)
		if count != 1 {
			t.Errorf("table %q not created", tbl)
		}
	}
}

func TestUsageDaily_AllRollupsDistinct(t *testing.T) {
	// Sanity: each rollup model has a distinct table name
	tables := map[string]string{
		"UsageDailyByProvider": (&UsageDailyByProvider{}).TableName(),
		"UsageDailyByModel":    (&UsageDailyByModel{}).TableName(),
		"UsageDailyByApiKey":   (&UsageDailyByApiKey{}).TableName(),
		"UsageDailyByAccount":  (&UsageDailyByAccount{}).TableName(),
		"UsageDailyByEndpoint": (&UsageDailyByEndpoint{}).TableName(),
	}
	seen := map[string]bool{}
	for k, v := range tables {
		if seen[v] {
			t.Errorf("duplicate table name %q for %s", v, k)
		}
		seen[v] = true
	}
}
