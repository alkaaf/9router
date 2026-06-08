package auth

import (
	"net/http"
	"strings"
)

// LoginRequest is the expected body for POST /api/auth/login.
type LoginRequest struct {
	Password string `json:"password"`
}

// LoginHandler returns an http.HandlerFunc for POST /api/auth/login.
func LoginHandler(store SettingsReader, writer SettingsWriter, limiter *RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req LoginRequest
		if !RequireJSON(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.Password) == "" {
			WriteJSONError(w, http.StatusBadRequest, "missing password")
			return
		}

		ip := clientIP(r)

		if locked, _ := limiter.IsLocked(ip); locked {
			WriteJSONError(w, http.StatusTooManyRequests, "locked")
			return
		}

		settings, err := store.GetSettings()
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		authMode := AuthMode(getString(settings, "authMode", string(AuthModePassword)))
		if authMode == AuthModeOIDC {
			WriteJSONError(w, http.StatusForbidden, "use OIDC login")
			return
		}

		expected := getString(settings, "passwordHash", "")
		match := VerifyPassword(req.Password, expected)
		if expected == "" {
			match = req.Password == "123456" || req.Password == getString(settings, "initialPassword", "123456")
		}

		if !match {
			remaining, _ := limiter.RecordFail(ip)
			if remaining <= 0 {
				w.WriteHeader(http.StatusTooManyRequests)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
			WriteJSONOK(w, struct {
				Error               string `json:"error"`
				RemainingBeforeLock int    `json:"remainingBeforeLock"`
			}{
				Error:               "invalid password",
				RemainingBeforeLock: remaining,
			})
			return
		}

		limiter.RecordSuccess(ip)

		if expected == "" && writer != nil {
			hashed, err := HashPassword(req.Password)
			if err == nil {
				settings["passwordHash"] = hashed
				_ = writer.PutSettings(settings)
			}
		}

		tok, err := SignJWT("admin", nil)
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "token error")
			return
		}
		SetAuthCookie(tok, w, r)
		WriteJSONOK(w, LoginResponse{Success: true})
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("x-forwarded-for"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("x-real-ip"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

func getString(m map[string]any, k, def string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return def
}
