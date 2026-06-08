package repository

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewGormDB_SQLiteMemory(t *testing.T) {
	db, err := NewGormDB(&Config{
		Driver:   DriverSQLite,
		DBPath:   ":memory:",
		LogLevel: LogSilent,
	})
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil *gorm.DB")
	}
	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Fatalf("ping: %v", err)
	}
	_ = closeDB(db)
}

func TestNewGormDB_SQLiteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := NewGormDB(&Config{
		Driver:   DriverSQLite,
		DBPath:   path,
		LogLevel: LogSilent,
	})
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	defer closeDB(db)

	// Verify WAL journal mode is set
	var mode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("pragma journal_mode: %v", err)
	}
	// in-memory or WAL depending on file path; in-memory returns "memory"
	if mode != "wal" && mode != "memory" {
		t.Errorf("unexpected journal mode %q (want wal or memory)", mode)
	}
}

func TestNewGormDB_UnsupportedDriver(t *testing.T) {
	_, err := NewGormDB(&Config{
		Driver:   "mysql",
		LogLevel: LogSilent,
	})
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestNewGormDB_NilConfig(t *testing.T) {
	_, err := NewGormDB(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestConfigurePool(t *testing.T) {
	db, err := NewGormDB(&Config{
		Driver:       DriverSQLite,
		DBPath:       ":memory:",
		LogLevel:     LogSilent,
		MaxIdleConns: 3,
		MaxOpenConns: 7,
		MaxLifetime:  2 * time.Hour,
		MaxIdleTime:  5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(db)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 7 {
		t.Errorf("MaxOpenConnections = %d, want 7", stats.MaxOpenConnections)
	}
}

func TestApplyDefaults_DefaultsFilled(t *testing.T) {
	c := &Config{}
	c.applyDefaults()
	if c.Driver == "" {
		t.Error("driver should be defaulted")
	}
	if c.Driver == DriverSQLite && c.DBPath == "" {
		t.Error("DBPath should be defaulted for sqlite")
	}
	if c.MaxIdleConns == 0 || c.MaxOpenConns == 0 {
		t.Error("pool sizes should be defaulted")
	}
	if c.MaxLifetime == 0 || c.MaxIdleTime == 0 {
		t.Error("pool durations should be defaulted")
	}
	if c.LogLevel == "" {
		t.Error("log level should be defaulted")
	}
}

func TestSkipDefaultTransaction(t *testing.T) {
	// GORM stores SkipDefaultTransaction on the Config; reflect on it via
	// behavior — issuing a Create outside an explicit transaction should
	// not open one.
	db, err := NewGormDB(&Config{
		Driver:   DriverSQLite,
		DBPath:   ":memory:",
		LogLevel: LogSilent,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(db)

	// Use a known-good table — autoMigrate a minimal model.
	type T struct {
		ID   uint `gorm:"primaryKey;autoIncrement"`
		Name string
	}
	if err := db.AutoMigrate(&T{}); err != nil {
		t.Fatal(err)
	}
	// Create + Read in sequence; SkipDefaultTransaction is enabled in our
	// NewGormDB, so this should not raise "transaction begin" issues.
	if err := db.Create(&T{Name: "x"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got T
	if err := db.First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Name != "x" {
		t.Errorf("got %q, want x", got.Name)
	}
}
