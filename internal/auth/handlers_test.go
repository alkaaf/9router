package auth

import "context"

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLogoutHandler(t *testing.T) {
	handler := LogoutHandler()
	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Error("expected cookie cleared")
	} else {
		c := cookies[0]
		if c.MaxAge >= 0 {
			t.Errorf("expected MaxAge<0, got %d", c.MaxAge)
		}
	}
}

func TestLogoutHandler_NoAuth(t *testing.T) {
	handler := LogoutHandler()
	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (idempotent), got %d", w.Code)
	}
}

func TestAuthStatusHandler_NoCookie(t *testing.T) {
	store := &stubSettings{data: map[string]any{}}
	handler := AuthStatusHandler(store)
	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthStatusHandler_ValidCookie(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)
	tok, _ := SignJWT("admin", nil)
	store := &stubSettings{data: map[string]any{}}
	handler := AuthStatusHandler(store)
	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	r.AddCookie(&http.Cookie{Name: "auth_token", Value: tok})
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthStatusHandler_RequireLoginFalse(t *testing.T) {
	store := &stubSettings{data: map[string]any{"requireLogin": false}}
	handler := AuthStatusHandler(store)
	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthStatusHandler_OIDCMode(t *testing.T) {
	store := &stubSettings{data: map[string]any{"authMode": "oidc"}}
	handler := AuthStatusHandler(store)
	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthStatusHandler_ExpiredCookie(t *testing.T) {
	store := &stubSettings{data: map[string]any{}}
	handler := AuthStatusHandler(store)
	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	r.AddCookie(&http.Cookie{Name: "auth_token", Value: "expired-jwt"})
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestClaimsFromContext_Nil(t *testing.T) {
	if ClaimsFromContext(nil) != nil {
		t.Error("expected nil for empty context")
	}
}

func TestApiKeyMiddleware_PublicPath(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := ApiKeyMiddleware(nil)
	r := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if !called {
		t.Error("expected next to be called for public path")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestApiKeyMiddleware_NoKey_Loopback(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := ApiKeyMiddleware(nil)
	r := httptest.NewRequest("GET", "/v1/chat", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if !called {
		t.Error("expected next to be called for loopback")
	}
}

func TestApiKeyMiddleware_NoKey_Remote(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := ApiKeyMiddleware(nil)
	r := httptest.NewRequest("GET", "/v1/chat", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if called {
		t.Error("expected next to NOT be called for remote without key")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestApiKeyMiddleware_ValidKey(t *testing.T) {
	db := newApiKeyTestDB(t)
	repo := NewApiKeyRepository(db)
	ctx := reqContext()
	_, _ = repo.CreateApiKey(ctx, "valid-key", "Test")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := ApiKeyMiddleware(repo)
	r := httptest.NewRequest("GET", "/v1/chat", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	r.Header.Set("Authorization", "Bearer valid-key")
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if !called {
		t.Error("expected next to be called for valid key")
	}
}

func TestApiKeyMiddleware_InvalidKey(t *testing.T) {
	db := newApiKeyTestDB(t)
	repo := NewApiKeyRepository(db)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := ApiKeyMiddleware(repo)
	r := httptest.NewRequest("GET", "/v1/chat", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	r.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if called {
		t.Error("expected next to NOT be called for invalid key")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_PublicPath(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := JWTMiddleware()
	r := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if !called {
		t.Error("expected public path to pass through")
	}
}

func TestJWTMiddleware_NoToken(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := JWTMiddleware()
	r := httptest.NewRequest("GET", "/api/chat", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if called {
		t.Error("expected no-token to be rejected")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)
	tok, _ := SignJWT("admin", nil)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := JWTMiddleware()
	r := httptest.NewRequest("GET", "/api/chat", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if !called {
		t.Error("expected valid token to pass")
	}
}

func TestJWTMiddleware_LocalOnly_Loopback(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := JWTMiddleware()
	r := httptest.NewRequest("GET", "/api/cli-tools/exec", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if !called {
		t.Error("expected loopback to access local-only")
	}
}

func TestJWTMiddleware_LocalOnly_Remote(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := JWTMiddleware()
	r := httptest.NewRequest("GET", "/api/cli-tools/exec", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if called {
		t.Error("expected remote local-only to be rejected")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDashboardMiddleware_NoCookie(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := DashboardMiddleware()
	r := httptest.NewRequest("GET", "/dashboard/", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if called {
		t.Error("expected redirect for no cookie")
	}
	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
}

func TestDashboardMiddleware_ValidCookie(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)
	tok, _ := SignJWT("admin", nil)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := DashboardMiddleware()
	r := httptest.NewRequest("GET", "/dashboard/", nil)
	r.AddCookie(&http.Cookie{Name: "auth_token", Value: tok})
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, r)
	if !called {
		t.Error("expected valid cookie to pass")
	}
}

// Helper to import context for tests
func reqContext() context.Context {
	return context.Background()
}

// wait briefly to keep `time` import alive if needed
var _ = time.Second
