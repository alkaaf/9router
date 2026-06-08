package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stubSettings struct {
	data  map[string]any
	write func(map[string]any) error
}

func (s *stubSettings) GetSettings() (map[string]any, error) {
	if s.data == nil {
		return map[string]any{}, nil
	}
	return s.data, nil
}

func (s *stubSettings) PutSettings(m map[string]any) error {
	if s.write != nil {
		return s.write(m)
	}
	return nil
}

func TestLoginHandler_CorrectDefaultPassword(t *testing.T) {
	store := &stubSettings{data: map[string]any{}}
	limiter := NewRateLimiter(5, time.Hour)
	handler := LoginHandler(store, store, limiter)

	body := `{"password":"123456"}`
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "10.0.0.5:1234"
	w := httptest.NewRecorder()

	handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Error("expected auth_token cookie")
	} else {
		if cookies[0].Name != "auth_token" {
			t.Errorf("expected auth_token, got %q", cookies[0].Name)
		}
		if cookies[0].Value == "" {
			t.Error("expected non-empty cookie value")
		}
	}
}

func TestLoginHandler_WrongPassword(t *testing.T) {
	store := &stubSettings{data: map[string]any{}}
	limiter := NewRateLimiter(5, time.Hour)
	handler := LoginHandler(store, store, limiter)

	body := `{"password":"wrong"}`
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "10.0.0.6:1234"
	w := httptest.NewRecorder()

	handler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid password") {
		t.Errorf("expected 'invalid password' in body, got %s", w.Body.String())
	}
}

func TestLoginHandler_MissingPassword(t *testing.T) {
	store := &stubSettings{data: map[string]any{}}
	limiter := NewRateLimiter(5, time.Hour)
	handler := LoginHandler(store, store, limiter)

	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "10.0.0.7:1234"
	w := httptest.NewRecorder()

	handler(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLoginHandler_LockoutAfter5Fails(t *testing.T) {
	store := &stubSettings{data: map[string]any{}}
	limiter := NewRateLimiter(5, time.Hour)
	handler := LoginHandler(store, store, limiter)

	for i := 0; i < 5; i++ {
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "10.0.0.8:1234"
		w := httptest.NewRecorder()
		handler(w, r)
	}
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"123456"}`))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "10.0.0.8:1234"
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on lockout, got %d", w.Code)
	}
}

func TestLoginHandler_OIDC(t *testing.T) {
	store := &stubSettings{data: map[string]any{"authMode": "oidc"}}
	limiter := NewRateLimiter(5, time.Hour)
	handler := LoginHandler(store, store, limiter)

	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"123456"}`))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "10.0.0.9:1234"
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for OIDC, got %d", w.Code)
	}
}

func TestLoginHandler_PersistsHashOnFirstLogin(t *testing.T) {
	var persisted map[string]any
	store := &stubSettings{
		data: map[string]any{},
		write: func(m map[string]any) error {
			persisted = m
			return nil
		},
	}
	limiter := NewRateLimiter(5, time.Hour)
	handler := LoginHandler(store, store, limiter)

	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"123456"}`))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "10.0.0.10:1234"
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if persisted == nil {
		t.Fatal("expected settings to be persisted")
	}
	hash, ok := persisted["passwordHash"].(string)
	if !ok {
		t.Fatal("expected passwordHash to be set")
	}
	if !VerifyPassword("123456", hash) {
		t.Error("persisted hash should verify against 123456")
	}
}

func TestLoginHandler_WrongMethod(t *testing.T) {
	store := &stubSettings{}
	limiter := NewRateLimiter(5, time.Hour)
	handler := LoginHandler(store, store, limiter)
	r := httptest.NewRequest("GET", "/api/auth/login", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
