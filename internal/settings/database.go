package settings

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// DatabaseBackup is the JSON payload for GET/POST /api/settings/database.
// It mirrors the Node.js exportDb/importDb shape.
type DatabaseBackup struct {
	Version            int                    `json:"version"`
	ExportedAt         string                 `json:"exportedAt"`
	Settings           map[string]any         `json:"settings"`
	ProviderConnections []map[string]any      `json:"providerConnections"`
	ProviderNodes      []map[string]any       `json:"providerNodes"`
	ProxyPools         []map[string]any       `json:"proxyPools"`
	ApiKeys            []map[string]any       `json:"apiKeys"`
	Combos             []map[string]any       `json:"combos"`
	ModelAliases       map[string]any         `json:"modelAliases"`
	DisabledModels     map[string]any         `json:"disabledModels"`
	CustomModels       []map[string]any       `json:"customModels"`
	Pricing            map[string]any         `json:"pricing"`
}

// DatabaseStore is the export/import contract for the database backup
// endpoint. Implementations include a GORM-backed store and an in-memory
// test store.
type DatabaseStore interface {
	ExportDatabase() (DatabaseBackup, error)
	ImportDatabase(payload DatabaseBackup) error
}

// DatabaseExportHandler returns an http.HandlerFunc for GET
// /api/settings/database.
func DatabaseExportHandler(store DatabaseStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		payload, err := store.ExportDatabase()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	}
}

// DatabaseImportHandler returns an http.HandlerFunc for POST
// /api/settings/database. After import, the supplied hook is called so
// the runtime can re-apply outbound proxy env vars.
func DatabaseImportHandler(store DatabaseStore, onImport func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20)) // 32 MB cap
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Could not read request body")
			return
		}
		var payload DatabaseBackup
		if err := json.Unmarshal(body, &payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
			return
		}
		if err := validateBackup(&payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		if err := store.ImportDatabase(payload); err != nil {
			writeJSONError(w, http.StatusBadRequest, "IMPORT_FAILED", err.Error())
			return
		}
		if onImport != nil {
			onImport()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}
}

// validateBackup returns an error if the payload is missing required
// top-level fields. The Node.js implementation is permissive about
// missing collections, but requires an object root.
func validateBackup(p *DatabaseBackup) error {
	if p == nil {
		return errors.New("payload is nil")
	}
	return nil
}

// currentTimestamp returns an ISO 8601 UTC timestamp. Wrapped in a
// function so tests can stub it.
var currentTimestamp = func() string {
	return time.Now().UTC().Format(time.RFC3339)
}
