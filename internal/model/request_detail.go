package model

import (
	"time"
)

// RequestDetail is a per-request audit log entry. The `data` column holds
// the full request/response payload as a JSON string (potentially tens of
// KB), so the table is append-only — the repository never UPDATEs rows.
//
// Mirrors the `requestDetails` table in the existing Node.js schema:
//   - id           TEXT PRIMARY KEY  (request UUID)
//   - timestamp    TIMESTAMPTZ / TEXT NOT NULL
//   - provider     TEXT NULL
//   - model        TEXT NULL
//   - connectionId TEXT NULL
//   - status       TEXT NULL
//   - data         JSONB / TEXT NOT NULL
//
// Indexes: idx_rd_ts, idx_rd_provider, idx_rd_model, idx_rd_conn.
type RequestDetail struct {
	ID           string    `gorm:"primaryKey;type:text;column:id"`
	Timestamp    time.Time `gorm:"not null;column:timestamp;index:idx_rd_ts,sort:desc"`
	Provider     *string   `gorm:"type:text;column:provider;index:idx_rd_provider"`
	Model        *string   `gorm:"type:text;column:model;index:idx_rd_model"`
	ConnectionID *string   `gorm:"type:text;column:connectionId;index:idx_rd_conn"`
	Status       *string   `gorm:"type:text;column:status"`
	Data         string    `gorm:"not null;type:text;column:data"`
}

// TableName pins the table name to `requestDetails`.
func (RequestDetail) TableName() string {
	return "requestDetails"
}
