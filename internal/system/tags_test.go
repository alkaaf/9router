package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTags_GET(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	TagsHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	var tags []string
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tags) == 0 {
		t.Errorf("tags empty, expected non-empty list")
	}
}

func TestTags_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tags", nil)
	TagsHandler()(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestTags_EmptyList(t *testing.T) {
	orig := ollamaModels
	defer func() { ollamaModels = orig }()
	ollamaModels = []string{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	TagsHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var tags []string
	_ = json.Unmarshal(rec.Body.Bytes(), &tags)
	if len(tags) != 0 {
		t.Errorf("expected empty list, got %v", tags)
	}
}
