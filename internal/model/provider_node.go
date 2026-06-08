package model

import (
	"time"
)

// ProviderNode represents a single routing/middleware node in the LLM
// pipeline — e.g. a "forward" target, a "middleware" interceptor, or a
// "transform" step. Each node has a type-specific configuration payload
// stored as JSON in the `data` column.
//
// Mirrors the `providerNodes` table in the existing Node.js schema:
//   - id        TEXT PRIMARY KEY
//   - type      TEXT NULL  (indexed — for category filtering)
//   - name      TEXT NULL
//   - data      JSONB / TEXT NOT NULL
//   - createdAt, updatedAt timestamps
type ProviderNode struct {
	ID        string    `gorm:"primaryKey;type:text;column:id"`
	Type      *string   `gorm:"type:text;column:type;index:idx_pn_type"`
	Name      *string   `gorm:"type:text;column:name"`
	Data      string    `gorm:"not null;type:text;column:data"`
	CreatedAt time.Time `gorm:"not null;column:createdAt;autoCreateTime"`
	UpdatedAt time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

// TableName pins the table name to `providerNodes`.
func (ProviderNode) TableName() string {
	return "providerNodes"
}
