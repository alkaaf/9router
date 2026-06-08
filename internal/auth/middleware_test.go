package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/api/health", true},
		{"/api/auth/login", true},
		{"/api/init", true},
		{"/api/v1/chat", false},
		{"/api/settings", false},
	}
	for _, c := range cases {
		if got := PublicPath(c.path); got != c.want {
			t.Errorf("PublicPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestAlwaysProtected(t *testing.T) {
	if !AlwaysProtected("/api/shutdown") {
		t.Error("expected /api/shutdown to be always protected")
	}
	if AlwaysProtected("/api/health") {
		t.Error("expected /api/health not to be always protected")
	}
}

func TestLocalOnly(t *testing.T) {
	if !LocalOnly("/api/cli-tools/exec") {
		t.Error("expected /api/cli-tools/exec to be local only")
	}
	if !LocalOnly("/api/tunnel/status") {
		t.Error("expected /api/tunnel/status to be local only")
	}
	if LocalOnly("/api/health") {
		t.Error("expected /api/health not to be local only")
	}
}

func TestIsLoopback(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"127.0.0.5", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"8.8.8.8", false},
		{"172.32.0.1", false},
		{"172.15.0.1", false},
	}
	for _, c := range cases {
		if got := IsLoopback(c.ip); got != c.want {
			t.Errorf("IsLoopback(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestIsSecureRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if IsSecureRequest(r) {
		t.Error("expected false without x-forwarded-proto")
	}
	r.Header.Set("x-forwarded-proto", "https")
	if !IsSecureRequest(r) {
		t.Error("expected true with x-forwarded-proto: https")
	}
}

func TestRateLimiter_Lockout(t *testing.T) {
	rl := NewRateLimiter(3, time.Hour)

	r1, _ := rl.RecordFail("ip1")
	if r1 != 2 {
		t.Errorf("expected 2 remaining, got %d", r1)
	}
	r2, _ := rl.RecordFail("ip1")
	if r2 != 1 {
		t.Errorf("expected 1 remaining, got %d", r2)
	}
	r3, retryAfter := rl.RecordFail("ip1")
	if r3 != 0 {
		t.Errorf("expected 0 remaining, got %d", r3)
	}
	if retryAfter == 0 {
		t.Error("expected retryAfter > 0 on lockout")
	}
	locked, secs := rl.IsLocked("ip1")
	if !locked {
		t.Error("expected ip1 to be locked")
	}
	if secs == 0 {
		t.Error("expected retryAfter > 0 when locked")
	}
	rl.RecordSuccess("ip1")
	locked, _ = rl.IsLocked("ip1")
	if locked {
		t.Error("expected ip1 to be unlocked after RecordSuccess")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, time.Hour)
	rl.RecordFail("ip1")
	rl.RecordFail("ip1")
	locked, _ := rl.IsLocked("ip2")
	if locked {
		t.Error("expected ip2 not to be locked")
	}
}

func TestBearerToken(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := BearerToken(r); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	r.Header.Set("Authorization", "Bearer abc123")
	if got := BearerToken(r); got != "abc123" {
		t.Errorf("expected abc123, got %q", got)
	}
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("x-api-token", "xyz789")
	if got := BearerToken(r2); got != "xyz789" {
		t.Errorf("expected xyz789, got %q", got)
	}
}

func TestAPIKeyHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer mykey")
	if got := APIKeyHeader(r); got != "mykey" {
		t.Errorf("expected mykey, got %q", got)
	}
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("x-api-key", "key2")
	if got := APIKeyHeader(r2); got != "key2" {
		t.Errorf("expected key2, got %q", got)
	}
}

func TestRequireJSON_Valid(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"}`))
	r.Header.Set("Content-Type", "application/json")
	var dst struct{ Name string }
	w := httptest.NewRecorder()
	if !RequireJSON(w, r, &dst) {
		t.Error("expected success")
	}
}

func TestRequireJSON_NoContentType(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	var dst struct{}
	w := httptest.NewRecorder()
	if RequireJSON(w, r, &dst) {
		t.Error("expected failure for missing content-type")
	}
}

func TestRequireJSON_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`bad`))
	r.Header.Set("Content-Type", "application/json")
	var dst struct{}
	w := httptest.NewRecorder()
	if RequireJSON(w, r, &dst) {
		t.Error("expected failure for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWriteJSONOK(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSONOK(w, map[string]string{"hello": "world"})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestWriteJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSONError(w, http.StatusUnauthorized, "no key")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
