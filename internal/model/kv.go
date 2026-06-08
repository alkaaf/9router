package model

// KV is a composite-key key-value store. The pair (Scope, Key) is the
// primary key — a single scope can hold many distinct keys.
//
// Mirrors the `kv` table in the existing Node.js schema:
//   - scope  TEXT NOT NULL  (part of composite PK)
//   - key    TEXT NOT NULL  (part of composite PK)
//   - value  TEXT NOT NULL
//
// Used by the pricing system, model alias registry, MITM alias map,
// custom models list, and disabled-models list. Each consumer uses a
// distinct `scope` string to isolate its namespace.
type KV struct {
	Scope string `gorm:"primaryKey;type:text;column:scope;index:idx_kv_scope,priority:1"`
	Key   string `gorm:"primaryKey;type:text;column:key"`
	Value string `gorm:"not null;type:text;column:value"`
}

// TableName pins the table name to `kv`.
func (KV) TableName() string {
	return "kv"
}
