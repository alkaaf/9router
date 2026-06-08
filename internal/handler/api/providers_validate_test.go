package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/9router/9router/internal/model"
	"github.com/9router/9router/internal/providers"
	"github.com/9router/9router/internal/repository"
)

func TestValidateHandler_UnknownProvider(t *testing.T) {
	db := setupTestDB(t)
	h := ValidateProviderHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/validate", map[string]any{
		"provider": "nonexistent",
		"apiKey":   "x",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown provider: got %d, want 400. body=%s", rr.Code, rr.Body.String())
	}
}

func TestValidateHandler_NoAuthProvider(t *testing.T) {
	db := setupTestDB(t)
	h := ValidateProviderHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/validate", map[string]any{
		"provider": "ollama-local",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("noauth: got %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Valid bool `json:"valid"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if !resp.Valid {
		t.Error("noauth should be valid")
	}
}

func TestValidateHandler_MissingAPIKey(t *testing.T) {
	db := setupTestDB(t)
	h := ValidateProviderHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/validate", map[string]any{
		"provider": "openai",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("missing apikey: got %d, want 200", rr.Code)
	}
	var resp struct {
		Valid bool `json:"valid"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Valid {
		t.Error("missing apikey should NOT be valid")
	}
}

func TestValidateHandler_MissingType(t *testing.T) {
	db := setupTestDB(t)
	h := ValidateProviderHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/validate", map[string]any{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing params: got %d, want 400", rr.Code)
	}
}

func TestTestProviderHandler_NotFound(t *testing.T) {
	db := setupTestDB(t)
	h := TestProviderHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/doesnotexist/test", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("not found: got %d, want 404", rr.Code)
	}
}

func TestTestProviderHandler_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	if err := repo.Create(ctx, &model.ProviderConnection{ID: "t1", Provider: "ollama-local", AuthType: "noauth", Data: "{}"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	h := TestProviderHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/t1/test", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("test: got %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
}

func TestTestBatchHandler_InvalidMode(t *testing.T) {
	db := setupTestDB(t)
	h := TestBatchHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/test-batch", map[string]any{
		"mode": "invalid",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode: got %d, want 400", rr.Code)
	}
}

func TestTestBatchHandler_AllMode(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewProviderRepository(db)
	ctx := context.Background()
	repo.Create(ctx, &model.ProviderConnection{ID: "b1", Provider: "ollama-local", AuthType: "noauth", Data: `{}`})
	h := TestBatchHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/test-batch", map[string]any{
		"mode": "all",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("all: got %d, want 200. body=%s", rr.Code, rr.Body.String())
	}
	var resp providers.BatchResult
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Summary.Total == 0 {
		t.Error("expected at least 1 result")
	}
}

func TestTestBatchHandler_ProviderModeRequiresID(t *testing.T) {
	db := setupTestDB(t)
	h := TestBatchHandler(db)
	rr := doRequest(t, h, http.MethodPost, "/api/providers/test-batch", map[string]any{
		"mode": "provider",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("provider mode: got %d, want 400", rr.Code)
	}
}

func TestSuggestedModelsHandler_MissingParams(t *testing.T) {
	db := setupTestDB(t)
	h := SuggestedModelsHandler(db)
	rr := doRequest(t, h, http.MethodGet, "/api/providers/suggested-models", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing params: got %d, want 400", rr.Code)
	}
}

func TestSuggestedModelsHandler_UnknownType(t *testing.T) {
	db := setupTestDB(t)
	h := SuggestedModelsHandler(db)
	rr := doRequest(t, h, http.MethodGet, "/api/providers/suggested-models?type=unknown&url=http://x", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown type: got %d, want 400", rr.Code)
	}
}
