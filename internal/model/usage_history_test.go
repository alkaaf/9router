package model

import (
	"math"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newUsageHistoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&UsageHistory{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestUsageHistory_TableName(t *testing.T) {
	u := UsageHistory{}
	if got := u.TableName(); got != "usageHistory" {
		t.Fatalf("TableName() = %q, want usageHistory", got)
	}
}

func TestUsageHistory_AllIndexes(t *testing.T) {
	db := newUsageHistoryTestDB(t)

	want := []string{
		"idx_uh_ts",
		"idx_uh_provider_ts",
		"idx_uh_model_ts",
		"idx_uh_conn_ts",
		"idx_uh_key_ts",
		"idx_uh_status_ts",
		"idx_uh_provider_model",
		"idx_uh_cost_ts",
	}
	for _, idx := range want {
		if !db.Migrator().HasIndex(&UsageHistory{}, idx) {
			t.Errorf("missing index %q", idx)
		}
	}
}

func TestUsageHistory_AutoIncrementID(t *testing.T) {
	db := newUsageHistoryTestDB(t)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		u := UsageHistory{Timestamp: now}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if u.ID != uint(i+1) {
			t.Errorf("ID[%d] = %d, want %d", i, u.ID, i+1)
		}
	}
}

func TestUsageHistory_BatchInsert500(t *testing.T) {
	db := newUsageHistoryTestDB(t)

	const n = 500
	now := time.Now().UTC()
	rows := make([]UsageHistory, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, UsageHistory{Timestamp: now})
	}
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatalf("batch insert: %v", err)
	}

	var count int64
	db.Model(&UsageHistory{}).Count(&count)
	if count != n {
		t.Errorf("expected %d rows, got %d", n, count)
	}
}

func TestUsageHistory_CostPrecision(t *testing.T) {
	db := newUsageHistoryTestDB(t)

	cost := 0.030001
	u := UsageHistory{
		Timestamp: time.Now().UTC(),
		Cost:      cost,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}

	var got UsageHistory
	if err := db.First(&got, "id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}

	// SQLite REAL stores 6 decimal digits reliably; difference must be < 1e-6
	if math.Abs(got.Cost-cost) > 1e-6 {
		t.Errorf("cost precision lost: got %.8f, want %.8f", got.Cost, cost)
	}
}

func TestUsageHistory_JSONTokensRoundtrip(t *testing.T) {
	db := newUsageHistoryTestDB(t)

	tokens := `{"prompt":100,"completion":50,"total":150}`
	meta := `{"client":"cli","ip":"127.0.0.1"}`
	u := UsageHistory{
		Timestamp: time.Now().UTC(),
		Tokens:    &tokens,
		Meta:      &meta,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}

	var got UsageHistory
	if err := db.First(&got, "id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Tokens == nil || *got.Tokens != tokens {
		t.Errorf("tokens roundtrip mismatch: %v", got.Tokens)
	}
	if got.Meta == nil || *got.Meta != meta {
		t.Errorf("meta roundtrip mismatch: %v", got.Meta)
	}
}

func TestUsageHistory_NullablePointerFields(t *testing.T) {
	db := newUsageHistoryTestDB(t)

	u := UsageHistory{
		Timestamp:        time.Now().UTC(),
		Provider:         nil,
		Model:            nil,
		ConnectionID:     nil,
		ApiKey:           nil,
		Endpoint:         nil,
		Status:           nil,
		PromptTokens:     0,
		CompletionTokens: 0,
		Cost:             0,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	var got UsageHistory
	if err := db.First(&got, "id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Provider != nil {
		t.Errorf("Provider should be nil, got %v", *got.Provider)
	}
	if got.ConnectionID != nil {
		t.Errorf("ConnectionID should be nil, got %v", *got.ConnectionID)
	}
}

func TestUsageHistory_FilterByProviderTimestamp(t *testing.T) {
	db := newUsageHistoryTestDB(t)

	now := time.Now().UTC()
	provA := "openai"
	provB := "claude"

	rows := []UsageHistory{
		{Timestamp: now.Add(-2 * time.Hour), Provider: &provA},
		{Timestamp: now.Add(-1 * time.Hour), Provider: &provA},
		{Timestamp: now, Provider: &provB},
		{Timestamp: now.Add(1 * time.Hour), Provider: &provA},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	since := now.Add(-90 * time.Minute)
	var got []UsageHistory
	if err := db.Where("provider = ? AND timestamp >= ?", provA, since).Order("timestamp ASC").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 rows for openai since -90m, got %d", len(got))
	}
}
