package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// WriteJSONOK serialises any value as JSON with HTTP 200.
func WriteJSONOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// WriteJSONError writes a structured JSON error response.
func WriteJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: message})
}

// RequireJSON ensures the request Content-Type is JSON and parses the
// body into dst. Returns false on parse failure (response already sent).
func RequireJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" || !strings.Contains(ct, "application/json") {
		WriteJSONError(w, http.StatusBadRequest, "Content-Type must be application/json")
		return false
	}
	if r.Body == nil {
		WriteJSONError(w, http.StatusBadRequest, "empty body")
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if err == io.EOF {
			WriteJSONError(w, http.StatusBadRequest, "empty body")
			return false
		}
		WriteJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// BearerToken extracts a token from Authorization: Bearer <token> or
// x-api-token header.
func BearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimPrefix(h, "Bearer ")
		}
	}
	return r.Header.Get("x-api-token")
}

// APIKeyHeader extracts an API key from Authorization: Bearer or
// x-api-key header.
func APIKeyHeader(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimPrefix(h, "Bearer ")
		}
	}
	return r.Header.Get("x-api-key")
}

// CLITokenHeader returns the value of the x-9r-cli-token header.
func CLITokenHeader(r *http.Request) string {
	return r.Header.Get("x-9r-cli-token")
}

// IsLoopback reports whether the supplied IP is loopback or private.
func IsLoopback(ip string) bool {
	if ip == "" {
		return false
	}
	if strings.HasPrefix(ip, "127.") || ip == "::1" {
		return true
	}
	if strings.HasPrefix(ip, "10.") {
		return true
	}
	if strings.HasPrefix(ip, "192.168.") {
		return true
	}
	if strings.HasPrefix(ip, "172.") {
		// 172.16.0.0/12 — second octet 16..31
		rest := ip[4:]
		dot := strings.IndexByte(rest, '.')
		if dot > 0 {
			second, ok := parseInt(rest[:dot])
			if ok && second >= 16 && second <= 31 {
				return true
			}
		}
	}
	return false
}

func parseInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// RateLimiter tracks login failures per IP.
type RateLimiter struct {
	mu      map[string]*bucket
	maxFail int
	window  time.Duration
}

type bucket struct {
	fails    int
	first    time.Time
	locked   bool
	unlockAt time.Time
}

// NewRateLimiter returns a RateLimiter.
func NewRateLimiter(maxFail int, window time.Duration) *RateLimiter {
	return &RateLimiter{mu: make(map[string]*bucket), maxFail: maxFail, window: window}
}

// IsLocked reports whether the IP is currently locked out.
func (rl *RateLimiter) IsLocked(ip string) (locked bool, retryAfter int) {
	b, ok := rl.mu[ip]
	if !ok {
		return false, 0
	}
	if !b.locked {
		return false, 0
	}
	if time.Now().After(b.unlockAt) {
		delete(rl.mu, ip)
		return false, 0
	}
	return true, int(time.Until(b.unlockAt).Seconds())
}

// RecordFail increments the fail counter.
func (rl *RateLimiter) RecordFail(ip string) (remaining int, retryAfter int) {
	b, ok := rl.mu[ip]
	now := time.Now()
	if !ok || now.Sub(b.first) > rl.window {
		rl.mu[ip] = &bucket{fails: 1, first: now}
		return rl.maxFail - 1, 0
	}
	b.fails++
	if b.fails >= rl.maxFail {
		b.locked = true
		b.unlockAt = now.Add(rl.window)
		return 0, int(rl.window.Seconds())
	}
	return rl.maxFail - b.fails, 0
}

// RecordSuccess resets the bucket for the IP.
func (rl *RateLimiter) RecordSuccess(ip string) {
	delete(rl.mu, ip)
}

// SetAuthCookie sets the auth_token cookie. secure flag is true when
// the request arrived via HTTPS (x-forwarded-proto: https).
func SetAuthCookie(token string, w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookie removes the auth_token cookie by setting MaxAge=-1.
func ClearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// IsSecureRequest returns true if the request is HTTPS (via
// x-forwarded-proto header).
func IsSecureRequest(r *http.Request) bool {
	return r.Header.Get("x-forwarded-proto") == "https"
}

// AuthMode represents the authentication mode in settings.
type AuthMode string

const (
	AuthModePassword AuthMode = "password"
	AuthModeOIDC     AuthMode = "oidc"
)

// SettingsReader exposes the minimum read surface for settings used by
// the auth handlers.
type SettingsReader interface {
	GetSettings() (map[string]any, error)
}

// SettingsWriter persists settings after a successful first login.
type SettingsWriter interface {
	PutSettings(map[string]any) error
}
