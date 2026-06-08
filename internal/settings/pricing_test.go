package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakePricingStore struct {
	mu  sync.Mutex
	data map[string]any
}

func (f *fakePricingStore) GetPricing() (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data == nil {
		return map[string]any{}, nil
	}
	return deepCopy(f.data), nil
}

func (f *fakePricingStore) SetPricing(d map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = deepCopy(d)
	return nil
}

func (f *fakePricingStore) DeletePricing(provider, model string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if provider == "" && model == "" {
		f.data = map[string]any{}
	} else if provider != "" && model == "" {
		delete(f.data, provider)
	} else {
		if p, ok := f.data[provider].(map[string]any); ok {
			delete(p, model)
			if len(p) == 0 {
				delete(f.data, provider)
			}
		}
	}
	return nil
}

func TestPricing_GET_Defaults(t *testing.T) {
	store := &fakePricingStore{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["openai"]; !ok {
		t.Errorf("expected openai in default pricing")
	}
}

func TestPricing_PATCH_Valid(t *testing.T) {
	store := &fakePricingStore{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/pricing",
		strings.NewReader(`{"openai":{"gpt-4o":{"input":2.5}}}`))
	req.Header.Set("Content-Type", "application/json")
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPricing_PATCH_InvalidField(t *testing.T) {
	store := &fakePricingStore{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/pricing",
		strings.NewReader(`{"openai":{"gpt-4o":{"invalidField":1}}}`))
	req.Header.Set("Content-Type", "application/json")
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestPricing_PATCH_NegativeValue(t *testing.T) {
	store := &fakePricingStore{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/pricing",
		strings.NewReader(`{"openai":{"gpt-4o":{"input":-1}}}`))
	req.Header.Set("Content-Type", "application/json")
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestPricing_PATCH_NonNumber(t *testing.T) {
	store := &fakePricingStore{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/pricing",
		strings.NewReader(`{"openai":{"gpt-4o":{"input":"free"}}}`))
	req.Header.Set("Content-Type", "application/json")
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestPricing_DELETE_All(t *testing.T) {
	store := &fakePricingStore{data: map[string]any{
		"openai": map[string]any{"gpt-4o": map[string]any{"input": 2.5}},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/pricing", nil)
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Defaults should still be present in the response (overrides cleared).
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["openai"]; !ok {
		t.Errorf("expected defaults in response after delete-all")
	}
}

func TestPricing_DELETE_Provider(t *testing.T) {
	store := &fakePricingStore{data: map[string]any{
		"openai": map[string]any{"gpt-4o": map[string]any{"input": 2.5}},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/pricing?provider=openai", nil)
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestPricing_DELETE_Model(t *testing.T) {
	store := &fakePricingStore{data: map[string]any{
		"openai": map[string]any{
			"gpt-4o":    map[string]any{"input": 2.5},
			"gpt-4o-mini": map[string]any{"input": 0.15},
		},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/pricing?provider=openai&model=gpt-4o", nil)
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Verify only gpt-4o was removed.
	d, _ := store.GetPricing()
	openai := d["openai"].(map[string]any)
	if _, ok := openai["gpt-4o"]; ok {
		t.Errorf("gpt-4o should be removed")
	}
	if _, ok := openai["gpt-4o-mini"]; !ok {
		t.Errorf("gpt-4o-mini should be preserved")
	}
}

func TestPricing_PATCH_NullReasoning(t *testing.T) {
	store := &fakePricingStore{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/pricing",
		strings.NewReader(`{"openai":{"gpt-4o":{"reasoning":null}}}`))
	req.Header.Set("Content-Type", "application/json")
	PricingHandler(store)(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (null should be accepted)", rec.Code)
	}
}
