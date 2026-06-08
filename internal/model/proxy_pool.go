package model

import (
	"time"
)

// ProxyPool represents a single proxy IP pool used for outbound traffic
// from the LLM proxy. The pool is enabled/disabled via `isActive` and
// tracks its most recent health check result in `testStatus`.
//
// Mirrors the `proxyPools` table in the existing Node.js schema:
//   - id         TEXT PRIMARY KEY
//   - isActive   BOOLEAN DEFAULT TRUE  (indexed)
//   - testStatus TEXT NULL             (indexed — values like "pass"/"fail"/"pending")
//   - data       JSONB / TEXT NOT NULL (host, port, auth, rotation rules)
//   - createdAt, updatedAt timestamps
type ProxyPool struct {
	ID         string    `gorm:"primaryKey;type:text;column:id"`
	IsActive   *bool     `gorm:"type:boolean;column:isActive;index:idx_pp_active"`
	TestStatus *string   `gorm:"type:text;column:testStatus;index:idx_pp_status"`
	Data       string    `gorm:"not null;type:text;column:data"`
	CreatedAt  time.Time `gorm:"not null;column:createdAt;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

// TableName pins the table name to `proxyPools`.
func (ProxyPool) TableName() string {
	return "proxyPools"
}
