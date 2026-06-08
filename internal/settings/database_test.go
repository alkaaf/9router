package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeDatabaseStore is an in-memory DatabaseStore for unit tests.
type fakeDatabaseStore struct {
	data DatabaseBackup
	err  error

	imported DatabaseBackup
	importErr error
	imports   int32
}

func (f *fakeDatabaseStore) ExportDatabase() (DatabaseBackup, error) {
	if f.err != nil {
		return DatabaseBackup{}, f.err
	}
	out := f.data
	out.Version = 1
	if out.ExportedAt == "" {
		out.ExportedAt = "2026-06-04T00:00:00Z"
	}
	return out, nil
}

func (f *fakeDatabaseStore) ImportDatabase(payload DatabaseBackup) error {
	atomic.AddInt32(&f.imports, 1)
	if f.importErr != nil {
		return f.importErr
	}
	f.imported = payload
	f.data = payload
	return nil
}

func TestDatabaseExport_HappyPath(t *testing.T) {
	store := &fakeDatabaseStore{data: DatabaseBackup{
		Settings:           map[string]any{"requireLogin": true},
		ProviderConnections: []map[string]any{{"id": "c1", "provider": "openai"}},
	}}
	rec := httptest.NewRecorder()
	DatabaseExportHandler(store)(rec, httptest.NewRequest(http.MethodGet, "/api/settings/database", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["version"] != float64(1) {
		t.Errorf("version = %v, want 1", body["version"])
	}
	if _, ok := body["exportedAt"]; !ok {
		t.Errorf("exportedAt missing")
	}
	for _, key := range []string{
		"settings", "providerConnections", "providerNodes", "proxyPools",
		"apiKeys", "combos", "modelAliases", "disabledModels",
		"customModels", "pricing",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing top-level key %q in export", key)
		}
	}
}

func TestDatabaseExport_MethodNotAllowed(t *testing.T) {
	store := &fakeDatabaseStore{}
	rec := httptest.NewRecorder()
	DatabaseExportHandler(store)(rec, httptest.NewRequest(http.MethodPost, "/api/settings/database", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestDatabaseImport_HappyPath(t *testing.T) {
	store := &fakeDatabaseStore{}
	body := `{"version":1,"exportedAt":"2026-06-04","settings":{"requireLogin":false}}`
	rec := httptest.NewRecorder()
	DatabaseImportHandler(store, nil)(rec, httptest.NewRequest(http.MethodPost, "/api/settings/database", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
	if atomic.LoadInt32(&store.imports) != 1 {
		t.Errorf("imports = %d, want 1", store.imports)
	}
}

func TestDatabaseImport_InvalidJSON(t *testing.T) {
	store := &fakeDatabaseStore{}
	rec := httptest.NewRecorder()
	DatabaseImportHandler(store, nil)(rec, httptest.NewRequest(http.MethodPost, "/api/settings/database", strings.NewReader(`not json`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDatabaseImport_MethodNotAllowed(t *testing.T) {
	store := &fakeDatabaseStore{}
	rec := httptest.NewRecorder()
	DatabaseImportHandler(store, nil)(rec, httptest.NewRequest(http.MethodGet, "/api/settings/database", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestDatabaseImport_ProxyReapply(t *testing.T) {
	store := &fakeDatabaseStore{}
	body := `{"version":1,"exportedAt":"2026-06-04","settings":{"outboundProxyEnabled":true,"outboundProxyUrl":"http://proxy:8080"}}`
	var called int32
	rec := httptest.NewRecorder()
	DatabaseImportHandler(store, func() { atomic.AddInt32(&called, 1) })(rec, httptest.NewRequest(http.MethodPost, "/api/settings/database", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("onImport called %d times, want 1", called)
	}
}

func TestDatabaseImport_ImportFailure(t *testing.T) {
	store := &fakeDatabaseStore{importErr: errImportFailed}
	body := `{"version":1,"settings":{}}`
	rec := httptest.NewRecorder()
	DatabaseImportHandler(store, nil)(rec, httptest.NewRequest(http.MethodPost, "/api/settings/database", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDatabaseRoundTrip(t *testing.T) {
	// Seed an initial payload, then export, import into a fresh store,
	// and re-export to confirm equivalence.
	original := &fakeDatabaseStore{data: DatabaseBackup{
		Settings: map[string]any{
			"requireLogin": true,
			"comboStrategy": "round-robin",
		},
		ApiKeys: []map[string]any{{"id": "k1", "key": "abc"}},
	}}

	// Export from original.
	rec := httptest.NewRecorder()
	DatabaseExportHandler(original)(rec, httptest.NewRequest(http.MethodGet, "/api/settings/database", nil))
	exported := rec.Body.String()

	// Import into a fresh store.
	fresh := &fakeDatabaseStore{}
	rec2 := httptest.NewRecorder()
	DatabaseImportHandler(fresh, nil)(rec2, httptest.NewRequest(http.MethodPost, "/api/settings/database", strings.NewReader(exported)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("import failed: %d %s", rec2.Code, rec2.Body.String())
	}

	// Re-export and compare settings/apiKeys.
	rec3 := httptest.NewRecorder()
	DatabaseExportHandler(fresh)(rec3, httptest.NewRequest(http.MethodGet, "/api/settings/database", nil))
	var got DatabaseBackup
	if err := json.Unmarshal(rec3.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Settings["requireLogin"] != true {
		t.Errorf("requireLogin = %v, want true", got.Settings["requireLogin"])
	}
	if got.Settings["comboStrategy"] != "round-robin" {
		t.Errorf("comboStrategy = %v, want round-robin", got.Settings["comboStrategy"])
	}
	if len(got.ApiKeys) != 1 || got.ApiKeys[0]["id"] != "k1" {
		t.Errorf("apiKeys = %v, want one key with id k1", got.ApiKeys)
	}
}

var errImportFailed = importErr("disk full")

type importErr string

func (e importErr) Error() string { return string(e) }
