package model

import (
	"time"
)

// Combo represents a model combo definition — a named list of model
// identifiers that can be referenced by the chat router. Combos are
// referenced by chat requests when the caller passes a combo name in
// place of a single model id.
//
// Mirrors the `combos` table in the existing Node.js schema:
//   - id    TEXT PRIMARY KEY
//   - name  TEXT UNIQUE NOT NULL
//   - kind  TEXT (optional strategy discriminator, e.g. "fallback", "roundrobin")
//   - models TEXT NOT NULL  (JSON-encoded array of model id strings)
//   - createdAt, updatedAt timestamps
type Combo struct {
	ID        string  `gorm:"primaryKey;type:text;column:id"`
	Name      string  `gorm:"not null;uniqueIndex;type:text;column:name"`
	Kind      *string `gorm:"type:text;column:kind"`
	Models    string  `gorm:"not null;type:text;column:models"`
	CreatedAt time.Time `gorm:"not null;column:createdAt;autoCreateTime"`
	UpdatedAt time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

// TableName pins the table name to the camelCase `combos` used by the
// existing Node.js schema.
func (Combo) TableName() string {
	return "combos"
}
