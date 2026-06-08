package model

import (
	"time"
)

// UsageDaily stores the full per-day usage aggregation as a single JSON
// blob keyed by YYYY-MM-DD. The blob is the canonical source for the
// dashboard's daily totals panel; the typed rollup tables (ByProvider,
// ByModel, ...) are derived from this and used for fast time-series queries.
//
// Mirrors the `usageDaily` table in the existing Node.js schema:
//   - dateKey TEXT PRIMARY KEY  (format "YYYY-MM-DD")
//   - data    TEXT NOT NULL     (JSON-encoded aggregation blob)
type UsageDaily struct {
	DateKey string `gorm:"primaryKey;type:text;column:dateKey"`
	Data    string `gorm:"not null;type:text;column:data"`
}

// TableName pins the table name to `usageDaily`.
func (UsageDaily) TableName() string {
	return "usageDaily"
}

// DateKeyFrom returns the YYYY-MM-DD key for the given time in UTC.
func DateKeyFrom(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}
