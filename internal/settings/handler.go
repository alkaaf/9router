package settings

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/9router/9router/internal/model"
)

// SettingsStore is an alias kept for backwards compatibility with the
// GET handler. New code should use SettingsReader / SettingsWriter.
type SettingsStore = SettingsReader

// SettingsGetHandler returns an http.HandlerFunc for GET /api/settings.
func SettingsGetHandler(store SettingsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		raw, err := store.GetSettings()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}

		oidcSecret, _ := raw["oidcClientSecret"].(string)
		password, _ := raw["password"].(string)

		safe := redactSettings(raw)
		safe["oidcConfigured"] = oidcConfigured(safe, oidcSecret)
		safe["hasPassword"] = password != ""
		safe["enableRequestLogs"] = envBool("ENABLE_REQUEST_LOGS")
		safe["enableTranslator"] = envBool("ENABLE_TRANSLATOR")

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(safe)
	}
}

// gormStore adapts a *gorm.DB query to the SettingsStore interface used
// by the handler.
type gormStore struct {
	db *model.Setting
}

func newGormStore(s *model.Setting) gormStore {
	return gormStore{db: s}
}

func (s gormStore) GetSettings() (map[string]any, error) {
	out := map[string]any{}
	if s.db == nil || s.db.Data == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(s.db.Data), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// redactSettings strips password and oidcClientSecret from a copy of raw.
func redactSettings(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "password" || k == "oidcClientSecret" {
			continue
		}
		out[k] = v
	}
	return out
}

// oidcConfigured returns true when all three OIDC fields are non-empty.
func oidcConfigured(safe map[string]any, oidcClientSecret string) bool {
	issuer, _ := safe["oidcIssuerUrl"].(string)
	clientID, _ := safe["oidcClientId"].(string)
	return issuer != "" && clientID != "" && oidcClientSecret != ""
}

// envBool returns true when the env var is "1" or "true" (case-insensitive).
func envBool(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return v == "1"
	}
	return b
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message,
		"code":  code,
	})
}
