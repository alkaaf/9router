package model

import (
	"time"
)

// ApiKey represents a single API key issued to a client (CLI tool, dashboard
// user, third-party integration). Auth middleware (DB-014 / AUTH) will look
// keys up by `key` value (UNIQUE indexed) and reject inactive ones.
//
// Mirrors the `apiKeys` table in the existing Node.js schema:
//   - id        TEXT PRIMARY KEY
//   - key       TEXT UNIQUE NOT NULL
//   - name      TEXT (optional display name)
//   - machineId TEXT (optional — bound to a specific machine fingerprint)
//   - isActive  BOOLEAN (0/1 on SQLite, true/false on Postgres) — null = unused
//   - createdAt TIMESTAMP
type ApiKey struct {
	ID        string  `gorm:"primaryKey;type:text;column:id"`
	Key       string  `gorm:"not null;uniqueIndex;type:text;column:key"`
	Name      *string `gorm:"type:text;column:name"`
	MachineID *string `gorm:"type:text;column:machineId"`
	IsActive  *bool   `gorm:"type:boolean;column:isActive"`
	CreatedAt time.Time `gorm:"not null;column:createdAt;autoCreateTime"`
}

// TableName pins the table name to the camelCase `apiKeys` used by the
// existing Node.js schema.
func (ApiKey) TableName() string {
	return "apiKeys"
}
