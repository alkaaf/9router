package model

import (
	"time"
)

// ProviderConnection represents a single LLM provider credential/config entry.
//
// Mirrors the `providerConnections` table in the existing SQLite/PostgreSQL schema:
//   - SQLite:  columns are camelCase TEXT/INTEGER, indexes on (provider), (provider,isActive), (provider,priority)
//   - PostgreSQL: BOOLEAN/JSONB/TIMESTAMPTZ, same indexes
//
// GORM tags keep the original camelCase column names so the model works against
// either driver without renaming the underlying tables.
type ProviderConnection struct {
	ID       string `gorm:"primaryKey;type:text;column:id"`
	Provider string `gorm:"not null;type:text;column:provider;index:idx_pc_provider,priority:1;index:idx_pc_provider_active,priority:1;index:idx_pc_priority,priority:1"`
	AuthType string `gorm:"not null;type:text;column:authType"`
	Name     *string `gorm:"type:text;column:name"`
	Email    *string `gorm:"type:text;column:email"`
	Priority *int    `gorm:"type:integer;column:priority;index:idx_pc_priority,priority:2"`
	IsActive *bool   `gorm:"type:boolean;column:isActive;index:idx_pc_provider_active,priority:2"`
	Data     string  `gorm:"not null;type:text;column:data"`

	CreatedAt time.Time `gorm:"not null;column:createdAt;autoCreateTime"`
	UpdatedAt time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

// TableName pins the GORM table name to the camelCase `providerConnections` used
// by the existing Node.js schema. We intentionally do not rely on GORM's
// snake_case default.
func (ProviderConnection) TableName() string {
	return "providerConnections"
}
