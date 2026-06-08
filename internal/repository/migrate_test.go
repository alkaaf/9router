package repository

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMigrateDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestAutoMigrateAll_AllTables(t *testing.T) {
	db := newMigrateDB(t)
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("AutoMigrateAll: %v", err)
	}
	missing, err := VerifyMigration(db)
	if err != nil {
		t.Fatalf("VerifyMigration: %v (missing: %v)", err, missing)
	}
	if len(missing) != 0 {
		t.Errorf("missing tables: %v", missing)
	}
}

func TestAutoMigrateAll_Idempotent(t *testing.T) {
	db := newMigrateDB(t)
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("second migrate (should be idempotent): %v", err)
	}
}

func TestAutoMigrateAll_NilDB(t *testing.T) {
	if err := AutoMigrateAll(nil); err == nil {
		t.Error("expected error when db is nil")
	}
}

func TestVerifyMigration_Partial(t *testing.T) {
	db := newMigrateDB(t)
	// Create a small unrelated table to simulate partial migration
	if err := db.Exec("CREATE TABLE not_in_list (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	missing, err := VerifyMigration(db)
	if err == nil {
		t.Error("expected error for partial migration")
	}
	if len(missing) == 0 {
		t.Error("expected missing tables, got none")
	}
}

func TestVerifyCriticalIndexes_AfterMigrate(t *testing.T) {
	db := newMigrateDB(t)
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("AutoMigrateAll: %v", err)
	}
	missing := VerifyCriticalIndexes(db)
	if len(missing) > 0 {
		t.Errorf("missing critical indexes: %v", missing)
	}
}

func TestAutoMigrateAll_PreservesExistingData(t *testing.T) {
	db := newMigrateDB(t)

	if err := AutoMigrateAll(db); err != nil {
		t.Fatal(err)
	}

	// Insert a real row in usageHistory (provider/model/etc are nullable)
	if err := db.Exec(`INSERT INTO usageHistory (timestamp, provider, promptTokens, completionTokens, cost) VALUES (?, ?, ?, ?, ?)`,
		"2025-01-01T00:00:00Z", nil, 0, 0, 0).Error; err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM usageHistory").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("pre-migrate row count = %d, want 1", count)
	}

	// Re-run migration, count must still be 1
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if err := db.Raw("SELECT COUNT(*) FROM usageHistory").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("post-migrate row count = %d, want 1 (idempotency broken)", count)
	}
}

func TestAllModels_Count(t *testing.T) {
	// We have 16 entries: 10 base + 5 rollup + 1 detail
	if len(AllModels) != 16 {
		t.Errorf("AllModels has %d entries, want 16", len(AllModels))
	}
}

func TestExpectedTables_TableNameConsistency(t *testing.T) {
	// Each ExpectedTables entry should match the TableName() of the
	// corresponding model in AllModels.
	if len(AllModels) != len(ExpectedTables) {
		t.Fatalf("AllModels (%d) and ExpectedTables (%d) length mismatch",
			len(AllModels), len(ExpectedTables))
	}
	for i, m := range AllModels {
		switch v := m.(type) {
		case interface{ TableName() string }:
			want := v.TableName()
			if ExpectedTables[i] != want {
				t.Errorf("ExpectedTables[%d] = %q, want TableName() = %q",
					i, ExpectedTables[i], want)
			}
		}
	}
}

func TestVerifyCriticalIndexes_MissingReported(t *testing.T) {
	// This test ensures the VerifyCriticalIndexes function actually
	// reports a missing index. It does so by checking the empty-
	// migrated state returns errors (we won't trigger the bug; we
	// just verify behavior when list is empty).
	db := newMigrateDB(t)
	missing := VerifyCriticalIndexes(db)
	// Fresh DB has no tables, so all indexes are "missing"
	if len(missing) == 0 {
		t.Error("expected missing indexes on fresh DB")
	}
	for _, name := range missing {
		if !strings.HasPrefix(name, "idx_") {
			t.Errorf("unexpected missing index name: %q", name)
		}
	}
}
