package system

import (
	"encoding/json"
	"net/http"
	"strings"
)

// LocaleResponse is the shape of POST /api/locale's response.
type LocaleResponse struct {
	Success bool   `json:"success"`
	Locale  string `json:"locale"`
}

// supportedLocales is the canonical set. Lowercase, sorted.
var supportedLocales = map[string]bool{
	"en": true, "zh": true, "ja": true, "ko": true,
	"es": true, "fr": true, "de": true, "pt": true,
	"ru": true, "ar": true, "hi": true, "th": true,
	"vi": true, "id": true, "nl": true, "it": true,
}

// LocaleHandler returns an http.HandlerFunc for POST /api/locale.
// It validates the locale, normalises to lowercase, and sets a
// 1-year `locale` cookie.
func LocaleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Locale string `json:"locale"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
			return
		}
		raw := strings.TrimSpace(body.Locale)
		if raw == "" || !supportedLocales[strings.ToLower(raw)] {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid locale")
			return
		}
		norm := strings.ToLower(raw)
		http.SetCookie(w, &http.Cookie{
			Name:   "locale",
			Value:  norm,
			MaxAge: 31536000,
			Path:   "/",
			HttpOnly: true,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(LocaleResponse{Success: true, Locale: norm})
	}
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": code})
}
