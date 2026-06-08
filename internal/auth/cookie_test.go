package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetAuthCookie_HTTPS(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("x-forwarded-proto", "https")
	w := httptest.NewRecorder()
	SetAuthCookie("token123", w, r)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "auth_token" {
		t.Errorf("expected auth_token, got %q", c.Name)
	}
	if c.Value != "token123" {
		t.Errorf("expected token123, got %q", c.Value)
	}
	if c.Path != "/" {
		t.Errorf("expected path=/, got %q", c.Path)
	}
	if !c.HttpOnly {
		t.Error("expected HttpOnly")
	}
	if !c.Secure {
		t.Error("expected Secure=true when x-forwarded-proto=https")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax, got %v", c.SameSite)
	}
}

func TestSetAuthCookie_HTTP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	SetAuthCookie("token123", w, r)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Secure {
		t.Error("expected Secure=false when no x-forwarded-proto")
	}
}

func TestClearAuthCookie(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	ClearAuthCookie(w, r)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.MaxAge >= 0 {
		t.Errorf("expected MaxAge<0 to delete, got %d", c.MaxAge)
	}
	if c.Value != "" {
		t.Errorf("expected empty value, got %q", c.Value)
	}
	if c.Path != "/" {
		t.Errorf("expected path=/, got %q", c.Path)
	}
}
