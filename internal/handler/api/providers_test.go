package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/9router/9router/internal/model"
	"github.com/9router/9router/internal/providers"
	"github.com/9router/9router/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func strPtr(s string) *string  { return &s }
func boolPtr(b bool) *bool     { return &b }

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.ProviderConnection{}, &model.ProviderNode{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func doRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestProvidersHandler_ListAll(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	repo.Create(ctx, &model.ProviderConnection{ID: "c1", Provider: "openai", AuthType: "apikey", Name: strPtr("OpenAI"), IsActive: boolPtr(true), Data: `{}`})
	repo.Create(ctx, &model.ProviderConnection{ID: "c2", Provider: "anthropic", AuthType: "apikey", Name: strPtr("Claude"), IsActive: boolPtr(true), Data: `{}`})

	h := ProvidersHandler(db)
	rr := doRequest(t, h, http.MethodGet, "/api/providers", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list all: got %d, want 200. body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Connections []providers.ConnectionView `json:"connections"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Connections) != 2 {
		t.Fatalf("list all: got %d, want 2", len(resp.Connections))
	}
}

func TestProvidersHandler_ListFilterActive(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	repo.Create(ctx, &model.ProviderConnection{ID: "a1", Provider: "openai", AuthType: "apikey", IsActive: boolPtr(true), Data: `{}`})
	repo.Create(ctx, &model.ProviderConnection{ID: "a2", Provider: "openai", AuthType: "apikey", IsActive: boolPtr(false), Data: `{}`})

	h := ProvidersHandler(db)
	rr := doRequest(t, h, http.MethodGet, "/api/providers?isActive=true", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("filter active: got %d", rr.Code)
	}

	var resp struct {
		Connections []providers.ConnectionView `json:"connections"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Connections) != 1 {
		t.Fatalf("filter active: got %d, want 1", len(resp.Connections))
	}
}

func TestProvidersHandler_SensitiveFieldsStripped(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	repo.Create(ctx, &model.ProviderConnection{
		ID: "s1", Provider: "openai", AuthType: "apikey", Name: strPtr("Test"), IsActive: boolPtr(true),
		Data: `{"apiKey":"secret","accessToken":"tok","refreshToken":"ref","idToken":"id","displayName":"OK"}`,
	})

	h := ProvidersHandler(db)
	rr := doRequest(t, h, http.MethodGet, "/api/providers", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("sensitive check: got %d", rr.Code)
	}

	body := rr.Body.String()
	for _, field := range []string{"apiKey", "accessToken", "refreshToken", "idToken"} {
		if bytes.Contains([]byte(body), []byte(field)) {
			t.Errorf("response contains sensitive field %q", field)
		}
	}
}

func TestProvidersHandler_CompatibleNameEnriched(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	if err := db.Create(&model.ProviderNode{ID: "openai-compatible-node-abc", Type: strPtr("forward"), Name: strPtr("My Node"), Data: `{"baseUrl":"http://localhost:11434"}`}).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	repo.Create(ctx, &model.ProviderConnection{ID: "c3", Provider: "openai-compatible-node-abc", AuthType: "apikey", Name: strPtr(""), IsActive: boolPtr(true), Data: `{}`})

	h := ProvidersHandler(db)
	rr := doRequest(t, h, http.MethodGet, "/api/providers", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("compatible: got %d", rr.Code)
	}

	var resp struct {
		Connections []providers.ConnectionView `json:"connections"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Connections) != 1 {
		t.Fatalf("compatible: got %d, want 1", len(resp.Connections))
	}
	if resp.Connections[0].Name == nil || *resp.Connections[0].Name != "My Node" {
		t.Errorf("compatible: name = %v, want 'My Node'", resp.Connections[0].Name)
	}
}

func TestProvidersHandler_Create(t *testing.T) {
	db := setupTestDB(t)
	h := ProvidersHandler(db)

	rr := doRequest(t, h, http.MethodPost, "/api/providers", map[string]any{
		"provider": "openai",
		"name":     "My OpenAI",
		"isActive": true,
		"apiKey":   "sk-test",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: got %d, want 201. body=%s", rr.Code, rr.Body.String())
	}
}

func TestProvidersHandler_CreateInvalidProvider(t *testing.T) {
	db := setupTestDB(t)
	h := ProvidersHandler(db)

	rr := doRequest(t, h, http.MethodPost, "/api/providers", map[string]any{
		"provider": "nonexistent",
		"name":     "Test",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("create invalid: got %d, want 400", rr.Code)
	}
}

func TestProvidersHandler_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	repo.Create(ctx, &model.ProviderConnection{ID: "g1", Provider: "openai", AuthType: "apikey", Name: strPtr("Test"), IsActive: boolPtr(true), Data: `{}`})

	h := ProvidersHandler(db)
	rr := doRequest(t, h, http.MethodGet, "/api/providers/g1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get by id: got %d, want 200", rr.Code)
	}
}

func TestProvidersHandler_GetByIDNotFound(t *testing.T) {
	db := setupTestDB(t)
	h := ProvidersHandler(db)

	rr := doRequest(t, h, http.MethodGet, "/api/providers/doesnotexist", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get missing: got %d, want 404", rr.Code)
	}
}

func TestProvidersHandler_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	repo.Create(ctx, &model.ProviderConnection{ID: "u1", Provider: "openai", AuthType: "apikey", Name: strPtr("Old"), IsActive: boolPtr(true), Data: `{}`})

	h := ProvidersHandler(db)
	rr := doRequest(t, h, http.MethodPut, "/api/providers/u1", map[string]any{
		"name": "New Name",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update: got %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
}

func TestProvidersHandler_UpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	h := ProvidersHandler(db)

	rr := doRequest(t, h, http.MethodPut, "/api/providers/doesnotexist", map[string]any{
		"name": "New Name",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("update missing: got %d, want 404", rr.Code)
	}
}

func TestProvidersHandler_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	repo.Create(ctx, &model.ProviderConnection{ID: "d1", Provider: "openai", AuthType: "apikey", Name: strPtr("Test"), IsActive: boolPtr(true), Data: `{}`})

	h := ProvidersHandler(db)
	rr := doRequest(t, h, http.MethodDelete, "/api/providers/d1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: got %d, want 200", rr.Code)
	}
}

func TestProvidersHandler_DeleteNotFound(t *testing.T) {
	db := setupTestDB(t)
	h := ProvidersHandler(db)

	rr := doRequest(t, h, http.MethodDelete, "/api/providers/doesnotexist", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("delete missing: got %d, want 404", rr.Code)
	}
}
