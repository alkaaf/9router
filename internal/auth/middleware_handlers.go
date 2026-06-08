package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"context"
	"net/http"
	"strings"
)

// PublicPath returns true if the request path is exempt from
// authentication (e.g. /api/health, /api/auth/*, /api/init).
func PublicPath(path string) bool {
	public := []string{
		"/api/health",
		"/api/init",
		"/api/locale",
		"/api/version",
		"/api/settings/require-login",
		"/api/auth/login",
		"/api/auth/logout",
		"/api/auth/status",
	}
	for _, p := range public {
		if path == p {
			return true
		}
	}
	if strings.HasPrefix(path, "/api/auth/") {
		return true
	}
	return false
}

// AlwaysProtected returns true if the path is always protected
// (require JWT even on the dashboard tier).
func AlwaysProtected(path string) bool {
	protected := []string{
		"/api/shutdown",
		"/api/settings/database",
		"/api/settings/oauth",
	}
	for _, p := range protected {
		if path == p {
			return true
		}
	}
	return false
}

// LocalOnly returns true if the path requires CLI token or loopback.
func LocalOnly(path string) bool {
	local := []string{"/api/cli-tools", "/api/tunnel"}
	for _, p := range local {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// ApiKeyMiddleware enforces API key auth for /v1/* and /v1beta/*.
func ApiKeyMiddleware(repo *ApiKeyRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if PublicPath(path) || AlwaysProtected(path) || LocalOnly(path) {
				next.ServeHTTP(w, r)
				return
			}
			// CLI token bypass
			if ValidateCLIToken(CLITokenHeader(r)) {
				next.ServeHTTP(w, r)
				return
			}
			// Loopback bypass
			if IsLoopback(clientIP(r)) {
				next.ServeHTTP(w, r)
				return
			}
			// API key check
			key := APIKeyHeader(r)
			if key == "" {
				WriteJSONError(w, http.StatusUnauthorized, "API key required for remote API access")
				return
			}
			if repo == nil {
				WriteJSONError(w, http.StatusInternalServerError, "auth not configured")
				return
			}
			rec, err := repo.FindValidApiKey(r.Context(), key)
			if err != nil {
				WriteJSONError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			go func() {
				_ = repo.UpdateLastUsed(context.Background(), rec.ID)
			}()
			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}

// JWTMiddleware enforces JWT auth for /api/* (non-v1).
func JWTMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if PublicPath(path) {
				next.ServeHTTP(w, r)
				return
			}
			if LocalOnly(path) {
				if ValidateCLIToken(CLITokenHeader(r)) {
					next.ServeHTTP(w, r)
					return
				}
				if IsLoopback(clientIP(r)) {
					next.ServeHTTP(w, r)
					return
				}
				WriteJSONError(w, http.StatusUnauthorized, "CLI token required for local-only endpoints")
				return
			}
			tok := BearerToken(r)
			if tok == "" {
				WriteJSONError(w, http.StatusUnauthorized, "missing token")
				return
			}
			claims, ok := VerifyJWT(tok)
			if !ok {
				WriteJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), ctxClaimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// DashboardMiddleware enforces JWT auth via the auth_token cookie for
// /dashboard/*. On failure, returns 302 to /login.
func DashboardMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("auth_token")
			if err != nil || cookie.Value == "" {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			claims, ok := VerifyJWT(cookie.Value)
			if !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			ctx := context.WithValue(r.Context(), ctxClaimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type ctxClaimsKey struct{}

// ClaimsFromContext returns the JWT claims set by middleware, or nil.
func ClaimsFromContext(ctx context.Context) *jwt.RegisteredClaims {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(ctxClaimsKey{})
	if v == nil {
		return nil
	}
	c, _ := v.(*jwt.RegisteredClaims)
	return c
}
