package model

// Meta represents a row in the `_meta` key-value table used for schema
// version tracking, application version stamps, and migration markers.
//
// Mirrors the `_meta` table in the existing Node.js schema:
//   - key   TEXT PRIMARY KEY
//   - value TEXT NOT NULL
//
// The leading underscore in `_meta` is a SQLite convention to force the
// table to the top of the listing; GORM's snake_case naming would otherwise
// produce `meta` or `metas`, so we explicitly pin the table name here.
type Meta struct {
	Key   string `gorm:"primaryKey;not null;type:text;column:key;check:key <> ''"`
	Value string `gorm:"not null;type:text;column:value"`
}

// TableName pins the table name to `_meta` (with leading underscore).
func (Meta) TableName() string {
	return "_meta"
}
