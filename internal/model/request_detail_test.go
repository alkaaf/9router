package model

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRequestDetailTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&RequestDetail{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestRequestDetail_TableName(t *testing.T) {
	r := RequestDetail{}
	if got := r.TableName(); got != "requestDetails" {
		t.Fatalf("TableName() = %q, want requestDetails", got)
	}
}

func TestRequestDetail_AllIndexes(t *testing.T) {
	db := newRequestDetailTestDB(t)
	want := []string{
		"idx_rd_ts",
		"idx_rd_provider",
		"idx_rd_model",
		"idx_rd_conn",
	}
	for _, idx := range want {
		if !db.Migrator().HasIndex(&RequestDetail{}, idx) {
			t.Errorf("missing index %q", idx)
		}
	}
}

func TestRequestDetail_Create(t *testing.T) {
	db := newRequestDetailTestDB(t)

	now := time.Now().UTC()
	prov := "openai"
	mdl := "gpt-4"
	conn := "conn-1"
	status := "ok"

	r := RequestDetail{
		ID:           "rd-1",
		Timestamp:    now,
		Provider:     &prov,
		Model:        &mdl,
		ConnectionID: &conn,
		Status:       &status,
		Data:         `{"req":"hello","resp":"hi"}`,
	}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got RequestDetail
	if err := db.First(&got, "id = ?", "rd-1").Error; err != nil {
		t.Fatal(err)
	}
	if got.Data != `{"req":"hello","resp":"hi"}` {
		t.Errorf("data mismatch: %q", got.Data)
	}
	if got.Provider == nil || *got.Provider != "openai" {
		t.Errorf("provider = %v, want openai", got.Provider)
	}
}

// buildLargeJSON generates a synthetic JSON payload of approximately size bytes.
func buildLargeJSON(size int) string {
	var sb strings.Builder
	sb.WriteString(`{"padding":"`)
	for sb.Len() < size-30 {
		sb.WriteString("abcdefghij")
	}
	sb.WriteString(`"}`)
	return sb.String()
}

func TestRequestDetail_LargeJSONRoundtrip(t *testing.T) {
	db := newRequestDetailTestDB(t)

	large := buildLargeJSON(50_000) // ~50KB
	r := RequestDetail{
		ID:        "rd-large",
		Timestamp: time.Now().UTC(),
		Data:      large,
	}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got RequestDetail
	if err := db.First(&got, "id = ?", "rd-large").Error; err != nil {
		t.Fatal(err)
	}
	if got.Data != large {
		t.Errorf("large JSON roundtrip mismatch: got %d bytes, want %d bytes", len(got.Data), len(large))
	}
}

func TestRequestDetail_FindByID(t *testing.T) {
	db := newRequestDetailTestDB(t)

	r := RequestDetail{
		ID:        "rd-find",
		Timestamp: time.Now().UTC(),
		Data:      "{}",
	}
	if err := db.Create(&r).Error; err != nil {
		t.Fatal(err)
	}

	var got RequestDetail
	if err := db.First(&got, "id = ?", "rd-find").Error; err != nil {
		t.Fatal(err)
	}
	if got.ID != "rd-find" {
		t.Errorf("got %q, want rd-find", got.ID)
	}
}

func TestRequestDetail_FilterByProvider(t *testing.T) {
	db := newRequestDetailTestDB(t)

	now := time.Now().UTC()
	openai := "openai"
	claude := "claude"

	rows := []RequestDetail{
		{ID: "rd-1", Timestamp: now, Provider: &openai, Data: "{}"},
		{ID: "rd-2", Timestamp: now, Provider: &openai, Data: "{}"},
		{ID: "rd-3", Timestamp: now, Provider: &claude, Data: "{}"},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	var got []RequestDetail
	if err := db.Where("provider = ?", openai).Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 rows for openai, got %d", len(got))
	}
}

func TestRequestDetail_DeleteOlderThan(t *testing.T) {
	db := newRequestDetailTestDB(t)

	now := time.Now().UTC()
	cutoff := now.Add(-1 * time.Hour)

	rows := []RequestDetail{
		{ID: "rd-old-1", Timestamp: now.Add(-2 * time.Hour), Data: "{}"},
		{ID: "rd-old-2", Timestamp: now.Add(-90 * time.Minute), Data: "{}"},
		{ID: "rd-new-1", Timestamp: now.Add(-30 * time.Minute), Data: "{}"},
		{ID: "rd-new-2", Timestamp: now, Data: "{}"},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Where("timestamp < ?", cutoff).Delete(&RequestDetail{}).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	var count int64
	db.Model(&RequestDetail{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 rows after cutoff delete, got %d", count)
	}
}

func TestRequestDetail_NilPointerFields(t *testing.T) {
	db := newRequestDetailTestDB(t)

	r := RequestDetail{
		ID:        "rd-nil",
		Timestamp: time.Now().UTC(),
		Data:      "{}",
	}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got RequestDetail
	if err := db.First(&got, "id = ?", "rd-nil").Error; err != nil {
		t.Fatal(err)
	}
	if got.Provider != nil {
		t.Errorf("Provider should be nil, got %v", *got.Provider)
	}
	if got.Model != nil {
		t.Errorf("Model should be nil, got %v", *got.Model)
	}
	if got.ConnectionID != nil {
		t.Errorf("ConnectionID should be nil, got %v", *got.ConnectionID)
	}
}
