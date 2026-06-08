package auth

import (
	"net/http"
)

// LogoutHandler returns an http.HandlerFunc for POST /api/auth/logout.
// Idempotent — always returns 200 with cleared cookie.
func LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		ClearAuthCookie(w, r)
		WriteJSONOK(w, struct {
			Success bool `json:"success"`
		}{Success: true})
	}
}
