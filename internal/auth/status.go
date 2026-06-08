package auth

import (
	"net/http"
)

// AuthStatusHandler returns an http.HandlerFunc for GET /api/auth/status.
// If RequireLogin=false in settings, the user is considered authenticated
// regardless of any cookie.
func AuthStatusHandler(store SettingsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		settings, err := store.GetSettings()
		if err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		authMode := getString(settings, "authMode", string(AuthModePassword))
		requireLogin := getBool(settings, "requireLogin", true)

		authenticated := false
		if !requireLogin {
			authenticated = true
		} else {
			cookie, cerr := r.Cookie("auth_token")
			if cerr == nil && cookie.Value != "" {
				_, ok := VerifyJWT(cookie.Value)
				if ok {
					authenticated = true
				}
			}
		}

		WriteJSONOK(w, AuthStatusResponse{
			Authenticated: authenticated,
			AuthMode:      authMode,
		})
	}
}

func getBool(m map[string]any, k string, def bool) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return def
}
