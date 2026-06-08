package system

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// ShutdownHandler returns an http.HandlerFunc for POST /api/shutdown.
// In production it returns 403. In development it requires
// Authorization: Bearer <SHUTDOWN_SECRET>. On success it returns
// { "success": true } then exits the process after 500ms.
func ShutdownHandler() http.HandlerFunc {
	secret := os.Getenv("SHUTDOWN_SECRET")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		if os.Getenv("NODE_ENV") == "production" {
			writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "Not allowed in production")
			return
		}

		if secret == "" {
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing SHUTDOWN_SECRET")
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" || auth != "Bearer "+secret {
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "Shutting down...",
		})

		go func() {
			time.Sleep(500 * time.Millisecond)
			os.Exit(0)
		}()
	}
}
