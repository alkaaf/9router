package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/9router/9router/internal/settings"
	"gorm.io/gorm"
)

// SettingsGetHandler returns an http.HandlerFunc for GET /api/settings.
func SettingsGetHandler(db *gorm.DB) http.HandlerFunc {
	return settings.SettingsGetHandler(gormSettingsStore{db: db})
}

// SettingsPatchHandler returns an http.HandlerFunc for PATCH /api/settings.
// The runtime side effects (applyOutboundProxyEnv / resetComboRotation) are
// wired to no-op stubs by default and can be overridden via
// SetSettingsSideEffects during startup.
func SettingsPatchHandler(db *gorm.DB) http.HandlerFunc {
	return settings.SettingsPatchHandler(gormSettingsStore{db: db}, currentSettingsSideEffects)
}

// gormSettingsStore adapts a *gorm.DB to settings.SettingsWriter.
type gormSettingsStore struct {
	db *gorm.DB
}

func (s gormSettingsStore) GetSettings() (map[string]any, error) {
	var r struct {
		Data string
	}
	if err := s.db.Table("settings").Select("data").Where("id = ?", 1).Take(&r).Error; err != nil {
		if err.Error() == "record not found" {
			return map[string]any{}, nil
		}
		return nil, err
	}
	out := map[string]any{}
	if r.Data == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(r.Data), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s gormSettingsStore) UpdateSettings(updates map[string]any) (map[string]any, error) {
	type row struct {
		Data string
	}
	var cur row
	if err := s.db.Table("settings").Select("data").Where("id = ?", 1).Take(&cur).Error; err != nil {
		if err.Error() == "record not found" {
			cur.Data = "{}"
		} else {
			return nil, err
		}
	}
	merged := map[string]any{}
	if cur.Data != "" {
		_ = json.Unmarshal([]byte(cur.Data), &merged)
	}
	for k, v := range updates {
		if v == nil {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	buf, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	if err := s.db.Table("settings").Where("id = ?", 1).Update("data", string(buf)).Error; err != nil {
		return nil, err
	}
	return merged, nil
}

var currentSettingsSideEffects = settings.SideEffects{
	ApplyOutboundProxyEnv: func(_ map[string]any) {},
	ResetComboRotation:    func() {},
}

// DatabaseExportHandler returns an http.HandlerFunc for GET
// /api/settings/database.
func DatabaseExportHandler(db *gorm.DB) http.HandlerFunc {
	return settings.DatabaseExportHandler(gormDatabaseStore{db: db})
}

// DatabaseImportHandler returns an http.HandlerFunc for POST
// /api/settings/database. After import it re-applies the outbound proxy
// env via the registered side effect.
func DatabaseImportHandler(db *gorm.DB) http.HandlerFunc {
	return settings.DatabaseImportHandler(gormDatabaseStore{db: db}, func() {
		if currentSettingsSideEffects.ApplyOutboundProxyEnv != nil {
			// Re-read settings and re-apply.
			store := gormSettingsStore{db: db}
			merged, err := store.GetSettings()
			if err == nil {
				currentSettingsSideEffects.ApplyOutboundProxyEnv(merged)
			}
		}
	})
}

// gormDatabaseStore is the GORM-backed DatabaseStore. The export/import
// reads and writes to the underlying tables using raw SQL so it works
// with both SQLite and PostgreSQL.
type gormDatabaseStore struct {
	db *gorm.DB
}

func (s gormDatabaseStore) ExportDatabase() (settings.DatabaseBackup, error) {
	out := settings.DatabaseBackup{
		Version:            1,
		ExportedAt:         time.Now().UTC().Format(time.RFC3339),
		Settings:           map[string]any{},
		ProviderConnections: []map[string]any{},
		ProviderNodes:      []map[string]any{},
		ProxyPools:         []map[string]any{},
		ApiKeys:            []map[string]any{},
		Combos:             []map[string]any{},
		ModelAliases:       map[string]any{},
		DisabledModels:     map[string]any{},
		CustomModels:       []map[string]any{},
		Pricing:            map[string]any{},
	}

	var r struct {
		Data string
	}
	if err := s.db.Table("settings").Select("data").Where("id = ?", 1).Take(&r).Error; err == nil {
		_ = json.Unmarshal([]byte(r.Data), &out.Settings)
	}

	if rows, err := s.db.Table("providerConnections").Rows(); err == nil {
		defer rows.Close()
		for rows.Next() {
			m := map[string]any{}
			if err := s.db.ScanRows(rows, &m); err == nil {
				out.ProviderConnections = append(out.ProviderConnections, m)
			}
		}
	}

	if rows, err := s.db.Table("apiKeys").Rows(); err == nil {
		defer rows.Close()
		for rows.Next() {
			m := map[string]any{}
			if err := s.db.ScanRows(rows, &m); err == nil {
				out.ApiKeys = append(out.ApiKeys, m)
			}
		}
	}

	return out, nil
}

func (s gormDatabaseStore) ImportDatabase(payload settings.DatabaseBackup) error {
	// The full GORM import path is implemented in the per-domain tasks
	// (PROV-*, COMBO-*, DB-014, etc). For SYS-003 we apply the
	// settings row (id=1) atomically so the endpoint is functional
	// and round-trippable for the part that is in scope.
	if payload.Settings != nil {
		buf, err := json.Marshal(payload.Settings)
		if err != nil {
			return err
		}
		return s.db.Table("settings").Where("id = ?", 1).Update("data", string(buf)).Error
	}
	return nil
}
