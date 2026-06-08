package api

import (
	"encoding/json"
	"net/http"
)

// writeJSONError writes a JSON error response with the given status,
// code, and human-readable message.
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message,
		"code":  code,
	})
}
