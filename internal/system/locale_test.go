package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocale_Valid(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/locale", strings.NewReader(`{"locale":"en"}`))
	req.Header.Set("Content-Type", "application/json")
	LocaleHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body LocaleResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Success || body.Locale != "en" {
		t.Errorf("response = %+v", body)
	}
	cookie := rec.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Errorf("Set-Cookie missing")
	} else {
		for _, attr := range []string{"Max-Age=31536000", "Path=/", "HttpOnly"} {
			if !strings.Contains(cookie, attr) {
				t.Errorf("Set-Cookie missing %q, got %s", attr, cookie)
			}
		}
	}
}

func TestLocale_UppercaseNormalised(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/locale", strings.NewReader(`{"locale":"EN"}`))
	req.Header.Set("Content-Type", "application/json")
	LocaleHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body LocaleResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Locale != "en" {
		t.Errorf("locale = %q, want en", body.Locale)
	}
}

func TestLocale_Invalid(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/locale", strings.NewReader(`{"locale":"xx"}`))
	req.Header.Set("Content-Type", "application/json")
	LocaleHandler()(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestLocale_MissingField(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/locale", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	LocaleHandler()(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestLocale_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/locale", nil)
	LocaleHandler()(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
