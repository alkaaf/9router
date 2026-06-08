package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestShutdown_ProductionBlocked(t *testing.T) {
	os.Setenv("NODE_ENV", "production")
	defer os.Unsetenv("NODE_ENV")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	req.Header.Set("Authorization", "Bearer anything")
	ShutdownHandler()(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 in production", rec.Code)
	}
}

func TestShutdown_MissingSecret(t *testing.T) {
	os.Setenv("NODE_ENV", "development")
	os.Unsetenv("SHUTDOWN_SECRET")
	defer os.Unsetenv("NODE_ENV")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	ShutdownHandler()(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when SHUTDOWN_SECRET missing", rec.Code)
	}
}

func TestShutdown_InvalidToken(t *testing.T) {
	os.Setenv("NODE_ENV", "development")
	os.Setenv("SHUTDOWN_SECRET", "correct-secret")
	defer os.Unsetenv("NODE_ENV")
	defer os.Unsetenv("SHUTDOWN_SECRET")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	ShutdownHandler()(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for wrong token", rec.Code)
	}
}

func TestShutdown_ValidTokenReturns200(t *testing.T) {
	os.Setenv("NODE_ENV", "development")
	os.Setenv("SHUTDOWN_SECRET", "my-secret")
	defer os.Unsetenv("NODE_ENV")
	defer os.Unsetenv("SHUTDOWN_SECRET")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	req.Header.Set("Authorization", "Bearer my-secret")
	ShutdownHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["success"] != true {
		t.Errorf("success = %v, want true", body["success"])
	}
}
