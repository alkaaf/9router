package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/9router/9router/internal/providers"
	"gorm.io/gorm"
)

// ValidateProviderHandler implements POST /api/providers/validate.
// Tests ad-hoc credentials without persisting anything. Returns
// 400 if the provider is unknown (PROV-008).
func ValidateProviderHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var in providers.TestInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
			return
		}
		if strings.TrimSpace(in.Provider) == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "provider is required")
			return
		}
		res, err := providers.ValidateTests(in)
		if err != nil {
			if errors.Is(err, providers.ErrInvalidProvider) || strings.Contains(err.Error(), "unknown provider") {
				writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":     res.Valid,
			"error":     nullableString(res.Error),
			"refreshed": res.Refreshed,
		})
	}
}

// TestProviderHandler implements POST /api/providers/:id/test.
// Tests a stored connection and persists the result (PROV-009).
func TestProviderHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		id, _ := extractID(r.URL.Path)
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing connection id")
			return
		}
		res, err := providers.TestSingleConnection(db, id)
		if err != nil {
			if errors.Is(err, providers.ErrNotFound) || strings.Contains(err.Error(), "NOT_FOUND") {
				writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":     res.Valid,
			"error":     nullableString(res.Error),
			"refreshed": res.Refreshed,
		})
	}
}

// TestBatchHandler implements POST /api/providers/test-batch
// (PROV-010).
func TestBatchHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Mode       string `json:"mode"`
			ProviderID string `json:"providerId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
			return
		}
		out, err := providers.TestBatch(db, body.Mode, body.ProviderID)
		if err != nil {
			if strings.Contains(err.Error(), "invalid mode") || strings.Contains(err.Error(), "providerId is required") {
				writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// SuggestedModelsHandler implements GET /api/providers/suggested-models
// (PROV-015).
func SuggestedModelsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		typ := r.URL.Query().Get("type")
		rawURL := r.URL.Query().Get("url")
		if typ == "" || rawURL == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "url and type are required")
			return
		}
		out, err := providers.FetchSuggestedModels(typ, rawURL, nil)
		if err != nil {
			if strings.Contains(err.Error(), "unknown type") {
				writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
