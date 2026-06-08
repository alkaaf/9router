package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyTest_InvalidURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/proxy-test",
		strings.NewReader(`{"proxyUrl":"not-a-url"}`))
	req.Header.Set("Content-Type", "application/json")
	ProxyTestHandler()(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestProxyTest_MissingProxyURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/proxy-test",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	ProxyTestHandler()(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestProxyTest_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/proxy-test", nil)
	ProxyTestHandler()(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestProxyTest_Defaults(t *testing.T) {
	// A proxy URL pointing to a non-listening port should return ok:false with a
	// connection error (not 500) and use defaults for testURL/timeout.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/proxy-test",
		strings.NewReader(`{"proxyUrl":"http://127.0.0.1:1"}`))
	req.Header.Set("Content-Type", "application/json")
	ProxyTestHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body ProxyTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OK {
		t.Errorf("expected ok:false for dead proxy, got ok:true")
	}
	if body.Error == "" {
		t.Errorf("expected error message, got empty")
	}
	// latencyMs may be 0 for an instantaneous connection failure; AC only
	// requires it to be a non-negative integer on the success path.
	if body.LatencyMs < 0 {
		t.Errorf("expected non-negative latencyMs, got %d", body.LatencyMs)
	}
}
