// Package repository contains the GORM database connection manager and
// per-model repositories for the 9router backend rewrite.
//
// The connection layer is dual-driver: it picks SQLite (default) or
// PostgreSQL based on configuration, and applies driver-specific
// connection-tuning settings (WAL mode + SkipDefaultTransaction for
// SQLite, PrepareStmt + statement_timeout for PostgreSQL).
package repository

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DBDriver identifies which GORM driver to use.
type DBDriver string

const (
	DriverSQLite   DBDriver = "sqlite"
	DriverPostgres DBDriver = "postgres"
)

// LogLevel mirrors gorm's logger levels with a small string-based enum
// suitable for config files.
type LogLevel string

const (
	LogSilent LogLevel = "silent"
	LogError  LogLevel = "error"
	LogWarn   LogLevel = "warn"
	LogInfo   LogLevel = "info"
)

// Config is the connection configuration for the database layer. Other
// packages may pass an instance of this struct; if Driver is empty the
// constructor will pick a driver from environment variables
// (DATABASE_URL → postgres, else sqlite).
type Config struct {
	Driver      DBDriver
	DBPath      string        // sqlite file path
	DatabaseURL string        // postgres connection URL
	LogLevel    LogLevel      // gorm logger level
	MaxIdleConns int          // connection pool: idle
	MaxOpenConns int          // connection pool: max open
	MaxLifetime  time.Duration // connection pool: max conn lifetime
	MaxIdleTime  time.Duration // connection pool: max idle time
}

// applyDefaults fills in zero values with sensible production defaults
// and resolves the driver / DSN from environment variables when not set.
func (c *Config) applyDefaults() {
	if c.LogLevel == "" {
		c.LogLevel = LogWarn
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 10
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 50
	}
	if c.MaxLifetime == 0 {
		c.MaxLifetime = time.Hour
	}
	if c.MaxIdleTime == 0 {
		c.MaxIdleTime = 10 * time.Minute
	}

	// Driver selection priority: explicit config > env > default (sqlite)
	if c.Driver == "" {
		if os.Getenv("DATABASE_URL") != "" {
			c.Driver = DriverPostgres
		} else {
			c.Driver = DriverSQLite
		}
	}

	if c.Driver == DriverPostgres && c.DatabaseURL == "" {
		c.DatabaseURL = os.Getenv("DATABASE_URL")
	}
	if c.Driver == DriverSQLite && c.DBPath == "" {
		if p := os.Getenv("DB_PATH"); p != "" {
			c.DBPath = p
		} else {
			c.DBPath = "9router.db"
		}
	}
}

// NewGormDB opens a GORM connection based on cfg. The returned *gorm.DB
// has driver-specific GORM settings applied (SkipDefaultTransaction,
// PrepareStmt, custom Logger) and the connection pool has been
// configured via ConfigurePool.
//
// The caller is responsible for closing the underlying *sql.DB when done
// (via db.DB(), then sqlDB.Close()). On any error, the partially-opened
// underlying connection is closed before returning.
func NewGormDB(cfg *Config) (*gorm.DB, error) {
	if cfg == nil {
		return nil, errors.New("db config is nil")
	}
	cfg.applyDefaults()

	gormCfg := &gorm.Config{
		Logger:                 newGormLogger(cfg.LogLevel),
		SkipDefaultTransaction: true,
	}

	var (
		db  *gorm.DB
		err error
	)

	switch cfg.Driver {
	case DriverSQLite:
		gormCfg.PrepareStmt = false
		db, err = gorm.Open(sqlite.Open(cfg.DBPath), gormCfg)
		if err != nil {
			return nil, fmt.Errorf("open sqlite (%s): %w", cfg.DBPath, err)
		}
		if err := configureSQLite(db); err != nil {
			_ = closeDB(db)
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	case DriverPostgres:
		gormCfg.PrepareStmt = true
		db, err = gorm.Open(postgres.Open(cfg.DatabaseURL), gormCfg)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		if err := configurePostgres(db); err != nil {
			_ = closeDB(db)
			return nil, fmt.Errorf("configure postgres: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported DB driver: %s", cfg.Driver)
	}

	ConfigurePool(db, cfg)
	return db, nil
}

// ConfigurePool applies the four connection pool settings to the
// underlying *sql.DB.
func ConfigurePool(db *gorm.DB, cfg *Config) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.MaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.MaxIdleTime)
}

// configureSQLite applies PRAGMA settings (WAL, synchronous, etc.) that
// the existing Node.js schema relies on.
func configureSQLite(db *gorm.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if err := db.Exec(p).Error; err != nil {
			return err
		}
	}
	return nil
}

// configurePostgres sets a per-session statement timeout to defend
// against runaway queries.
func configurePostgres(db *gorm.DB) error {
	return db.Exec("SET statement_timeout = '30s'").Error
}

// newGormLogger returns a GORM logger matching the configured level.
func newGormLogger(level LogLevel) logger.Interface {
	cfg := logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	}
	switch level {
	case LogSilent:
		cfg.LogLevel = logger.Silent
	case LogError:
		cfg.LogLevel = logger.Error
	case LogInfo:
		cfg.LogLevel = logger.Info
	case LogWarn:
		fallthrough
	default:
		cfg.LogLevel = logger.Warn
	}
	// NewLogger writes to stderr; we route through the standard logger so
	// production deployments can capture it with their log pipeline.
	return logger.New(log.New(os.Stderr, "[gorm] ", log.LstdFlags), cfg)
}

// closeDB closes the underlying *sql.DB on a partially-constructed GORM
// instance. Errors are swallowed because this is best-effort cleanup.
func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
