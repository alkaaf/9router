package model

import (
	"time"
)

// UsageDailyByProvider is one of five typed rollup tables. The composite
// PK (date, provider) supports fast upserts via ON CONFLICT and time-bucketed
// reads in the chart endpoint.
//
// Mirrors the `usageDailyByProvider` table in the existing Node.js schema:
//   - date         DATE
//   - provider     TEXT
//   - requestCount BIGINT DEFAULT 0
//   - inputTokens  BIGINT DEFAULT 0
//   - outputTokens BIGINT DEFAULT 0
//   - totalTokens  BIGINT DEFAULT 0
//   - cost         NUMERIC(12,6) DEFAULT 0
//   - updatedAt    TIMESTAMPTZ
type UsageDailyByProvider struct {
	Date         time.Time `gorm:"primaryKey;type:date;column:date"`
	Provider     string    `gorm:"primaryKey;type:text;column:provider"`
	RequestCount int64     `gorm:"not null;default:0;column:requestCount"`
	InputTokens  int64     `gorm:"not null;default:0;column:inputTokens"`
	OutputTokens int64     `gorm:"not null;default:0;column:outputTokens"`
	TotalTokens  int64     `gorm:"not null;default:0;column:totalTokens"`
	Cost         float64   `gorm:"not null;default:0;type:numeric(12,6);column:cost"`
	UpdatedAt    time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

func (UsageDailyByProvider) TableName() string { return "usageDailyByProvider" }

type UsageDailyByModel struct {
	Date         time.Time `gorm:"primaryKey;type:date;column:date"`
	Model        string    `gorm:"primaryKey;type:text;column:model"`
	RequestCount int64     `gorm:"not null;default:0;column:requestCount"`
	InputTokens  int64     `gorm:"not null;default:0;column:inputTokens"`
	OutputTokens int64     `gorm:"not null;default:0;column:outputTokens"`
	TotalTokens  int64     `gorm:"not null;default:0;column:totalTokens"`
	Cost         float64   `gorm:"not null;default:0;type:numeric(12,6);column:cost"`
	UpdatedAt    time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

func (UsageDailyByModel) TableName() string { return "usageDailyByModel" }

type UsageDailyByApiKey struct {
	Date         time.Time `gorm:"primaryKey;type:date;column:date"`
	ApiKey       string    `gorm:"primaryKey;type:text;column:apiKey"`
	RequestCount int64     `gorm:"not null;default:0;column:requestCount"`
	InputTokens  int64     `gorm:"not null;default:0;column:inputTokens"`
	OutputTokens int64     `gorm:"not null;default:0;column:outputTokens"`
	TotalTokens  int64     `gorm:"not null;default:0;column:totalTokens"`
	Cost         float64   `gorm:"not null;default:0;type:numeric(12,6);column:cost"`
	UpdatedAt    time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

func (UsageDailyByApiKey) TableName() string { return "usageDailyByApiKey" }

type UsageDailyByAccount struct {
	Date         time.Time `gorm:"primaryKey;type:date;column:date"`
	Account      string    `gorm:"primaryKey;type:text;column:account"`
	RequestCount int64     `gorm:"not null;default:0;column:requestCount"`
	InputTokens  int64     `gorm:"not null;default:0;column:inputTokens"`
	OutputTokens int64     `gorm:"not null;default:0;column:outputTokens"`
	TotalTokens  int64     `gorm:"not null;default:0;column:totalTokens"`
	Cost         float64   `gorm:"not null;default:0;type:numeric(12,6);column:cost"`
	UpdatedAt    time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

func (UsageDailyByAccount) TableName() string { return "usageDailyByAccount" }

type UsageDailyByEndpoint struct {
	Date         time.Time `gorm:"primaryKey;type:date;column:date"`
	Endpoint     string    `gorm:"primaryKey;type:text;column:endpoint"`
	RequestCount int64     `gorm:"not null;default:0;column:requestCount"`
	InputTokens  int64     `gorm:"not null;default:0;column:inputTokens"`
	OutputTokens int64     `gorm:"not null;default:0;column:outputTokens"`
	TotalTokens  int64     `gorm:"not null;default:0;column:totalTokens"`
	Cost         float64   `gorm:"not null;default:0;type:numeric(12,6);column:cost"`
	UpdatedAt    time.Time `gorm:"not null;column:updatedAt;autoUpdateTime"`
}

func (UsageDailyByEndpoint) TableName() string { return "usageDailyByEndpoint" }
