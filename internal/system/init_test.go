package system

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestInit_GET(t *testing.T) {
	rec := httptest.NewRecorder()
	InitHandler()(rec, httptest.NewRequest(http.MethodGet, "/api/init", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "Initialized" {
		t.Errorf("body = %q, want Initialized", got)
	}
	if got := rec.Header().Get("Content-Type"); got == "" || got[:10] != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain*", got)
	}
}

func TestInit_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	InitHandler()(rec, httptest.NewRequest(http.MethodPost, "/api/init", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestInit_Concurrent(t *testing.T) {
	h := InitHandler()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest(http.MethodGet, "/api/init", nil))
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		}()
	}
	wg.Wait()
}
