package repository

import (
	"fmt"

	"github.com/9router/9router/internal/model"
	"gorm.io/gorm"
)

// AllModels is the canonical registration slice for the migration step.
// Every model (including rollup tables) must be listed here so
// AutoMigrateAll and VerifyMigration stay in sync.
var AllModels = []interface{}{
	&model.Meta{},
	&model.Setting{},
	&model.ProviderConnection{},
	&model.Combo{},
	&model.ApiKey{},
	&model.KV{},
	&model.ProviderNode{},
	&model.ProxyPool{},
	&model.UsageHistory{},
	&model.UsageDaily{},
	&model.UsageDailyByProvider{},
	&model.UsageDailyByModel{},
	&model.UsageDailyByApiKey{},
	&model.UsageDailyByAccount{},
	&model.UsageDailyByEndpoint{},
	&model.RequestDetail{},
}

// AutoMigrateAll registers all models and runs GORM's additive
// AutoMigrate in a single call. It is safe to call multiple times —
// GORM only adds missing tables/columns/indexes.
func AutoMigrateAll(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	return db.AutoMigrate(AllModels...)
}

// ExpectedTables lists every table name that AutoMigrateAll should
// create. Must be kept in sync with AllModels.
var ExpectedTables = []string{
	model.Meta{}.TableName(),
	model.Setting{}.TableName(),
	model.ProviderConnection{}.TableName(),
	model.Combo{}.TableName(),
	model.ApiKey{}.TableName(),
	model.KV{}.TableName(),
	model.ProviderNode{}.TableName(),
	model.ProxyPool{}.TableName(),
	model.UsageHistory{}.TableName(),
	model.UsageDaily{}.TableName(),
	model.UsageDailyByProvider{}.TableName(),
	model.UsageDailyByModel{}.TableName(),
	model.UsageDailyByApiKey{}.TableName(),
	model.UsageDailyByAccount{}.TableName(),
	model.UsageDailyByEndpoint{}.TableName(),
	model.RequestDetail{}.TableName(),
}

// VerifyMigration checks that every ExpectedTable exists in the database.
// It returns a list of missing table names (empty slice means success).
func VerifyMigration(db *gorm.DB) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	migrator := db.Migrator()
	var missing []string
	for _, tbl := range ExpectedTables {
		if !migrator.HasTable(tbl) {
			missing = append(missing, tbl)
		}
	}
	if len(missing) > 0 {
		return missing, fmt.Errorf("tables not created: %v", missing)
	}
	return nil, nil
}

// CriticalIndexes lists the most important index names that must be
// present after migration. Returns a list of missing index names, or
// nil if all are present.
//
// The check is per-model because HasIndex requires the model struct
// (the table name alone is not enough for GORM's migrator API).
func VerifyCriticalIndexes(db *gorm.DB) []string {
	type idxCheck struct {
		model interface{}
		name  string
	}
	checks := []idxCheck{
		{&model.ProviderConnection{}, "idx_pc_provider"},
		{&model.ProviderConnection{}, "idx_pc_provider_active"},
		{&model.ProviderConnection{}, "idx_pc_priority"},
		{&model.Combo{}, "idx_combos_name"},
		{&model.ApiKey{}, "idx_apiKeys_key"},
		{&model.KV{}, "idx_kv_scope"},
		{&model.UsageHistory{}, "idx_uh_ts"},
		{&model.UsageHistory{}, "idx_uh_provider_ts"},
		{&model.UsageHistory{}, "idx_uh_model_ts"},
		{&model.UsageHistory{}, "idx_uh_conn_ts"},
		{&model.UsageHistory{}, "idx_uh_key_ts"},
		{&model.UsageHistory{}, "idx_uh_status_ts"},
		{&model.UsageHistory{}, "idx_uh_provider_model"},
		{&model.UsageHistory{}, "idx_uh_cost_ts"},
		{&model.ProviderNode{}, "idx_pn_type"},
		{&model.ProxyPool{}, "idx_pp_active"},
		{&model.ProxyPool{}, "idx_pp_status"},
		{&model.RequestDetail{}, "idx_rd_ts"},
		{&model.RequestDetail{}, "idx_rd_provider"},
		{&model.RequestDetail{}, "idx_rd_model"},
		{&model.RequestDetail{}, "idx_rd_conn"},
	}

	var missing []string
	for _, c := range checks {
		if !db.Migrator().HasIndex(c.model, c.name) {
			missing = append(missing, c.name)
		}
	}
	return missing
}
