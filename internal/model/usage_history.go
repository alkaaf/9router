package model

import (
	"time"
)

// UsageHistory is the append-only log of every API request processed by
// the gateway. It is the highest-write-rate table in the system; the
// repository configures `SkipDefaultTransaction: true` to keep insert
// throughput high.
//
// Mirrors the `usageHistory` table in the existing Node.js schema:
//   - id               BIGSERIAL / AUTOINCREMENT primary key
//   - timestamp        TIMESTAMPTZ / TEXT NOT NULL  (indexed everywhere)
//   - provider, model  TEXT NULL
//   - connectionId, apiKey TEXT NULL
//   - endpoint         TEXT NULL
//   - promptTokens, completionTokens INTEGER DEFAULT 0
//   - cost             NUMERIC(12,6) / REAL DEFAULT 0
//   - status           TEXT NULL
//   - tokens, meta     JSONB / TEXT NULL
//
// 10 indexes support time-series slicing by (provider, timestamp),
// (model, timestamp), (connectionId, timestamp), (apiKey, timestamp),
// (status, timestamp), (provider, model) and (cost, timestamp).
type UsageHistory struct {
	ID               uint      `gorm:"primaryKey;autoIncrement;column:id"`
	Timestamp        time.Time `gorm:"not null;column:timestamp;index:idx_uh_ts,sort:desc;index:idx_uh_provider_ts,priority:2;index:idx_uh_model_ts,priority:2;index:idx_uh_conn_ts,priority:2;index:idx_uh_key_ts,priority:2;index:idx_uh_status_ts,priority:2;index:idx_uh_provider_model,priority:2;index:idx_uh_cost_ts,priority:2"`
	Provider         *string   `gorm:"type:text;column:provider;index:idx_uh_provider_ts,priority:1;index:idx_uh_provider_model,priority:1,priority:1"`
	Model            *string   `gorm:"type:text;column:model;index:idx_uh_model_ts,priority:1;index:idx_uh_provider_model,priority:1,priority:2"`
	ConnectionID     *string   `gorm:"type:text;column:connectionId;index:idx_uh_conn_ts,priority:1"`
	ApiKey           *string   `gorm:"type:text;column:apiKey;index:idx_uh_key_ts,priority:1"`
	Endpoint         *string   `gorm:"type:text;column:endpoint"`
	PromptTokens     int       `gorm:"type:integer;default:0;column:promptTokens"`
	CompletionTokens int       `gorm:"type:integer;default:0;column:completionTokens"`
	Cost             float64   `gorm:"type:numeric(12,6);default:0;column:cost;index:idx_uh_cost_ts,priority:1"`
	Status           *string   `gorm:"type:text;column:status;index:idx_uh_status_ts,priority:1"`
	Tokens           *string   `gorm:"type:text;column:tokens"`
	Meta             *string   `gorm:"type:text;column:meta"`
}

// TableName pins the table name to the camelCase `usageHistory` used by
// the existing Node.js schema.
func (UsageHistory) TableName() string {
	return "usageHistory"
}
