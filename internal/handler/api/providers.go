package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/9router/9router/internal/model"
	"github.com/9router/9router/internal/providers"
	"github.com/9router/9router/internal/repository"
	"gorm.io/gorm"
)

// ProvidersHandler returns an http.HandlerFunc that dispatches
// requests under /api/providers and /api/providers/:id based on the
// HTTP method and trailing path segment.
func ProvidersHandler(db *gorm.DB) http.HandlerFunc {
	return providersHandlerImpl(repository.NewProviderRepository(db), db)
}

func providersHandlerImpl(repo *repository.ProviderRepository, db *gorm.DB) http.HandlerFunc {
	nodeNameFn := nodeNameLookup(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id, hasID := extractID(r.URL.Path)
		// Special routes that don't require an id.
		if r.Method == http.MethodGet && r.URL.Path == "/api/providers/suggested-models" {
			handleSuggestedModels(w, r)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/providers/validate" {
			handleValidate(w, r)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/providers/test-batch" {
			handleTestBatch(w, r, db)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if hasID && strings.HasSuffix(r.URL.Path, "/test") {
				handleTestSingle(w, r, db)
				return
			}
			if hasID {
				handleGetProvider(w, r, repo, nodeNameFn, id)
			} else {
				handleListProviders(w, r, repo, nodeNameFn)
			}
		case http.MethodPost:
			if hasID && strings.HasSuffix(r.URL.Path, "/test") {
				handleTestSingle(w, r, db)
				return
			}
			handleCreateProvider(w, r, repo, nodeNameFn)
		case http.MethodPut:
			if !hasID {
				writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing connection id")
				return
			}
			handleUpdateProvider(w, r, repo, nodeNameFn)
		case http.MethodDelete:
			if !hasID {
				writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing connection id")
				return
			}
			handleDeleteProvider(w, r, repo)
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET, POST, PUT, DELETE are supported")
		}
	}
}

// extractID pulls the trailing id from a path like "/api/providers/<id>".
// It returns the id and true when one is present.
func extractID(path string) (string, bool) {
	const prefix = "/api/providers"
	path = strings.TrimPrefix(path, prefix)
	path = strings.Trim(path, "/")
	if path == "" {
		return "", false
	}
	if i := strings.Index(path, "/"); i >= 0 {
		path = path[:i]
	}
	return path, true
}

// handleListProviders implements GET /api/providers.
// Supports ?isActive=true|false filter.
func handleListProviders(w http.ResponseWriter, r *http.Request, repo *repository.ProviderRepository, nodeNameFn func(string) string) {
	filter := repository.ProviderFilter{}
	if isActiveStr := r.URL.Query().Get("isActive"); isActiveStr != "" {
		v, err := strconv.ParseBool(isActiveStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "isActive must be true or false")
			return
		}
		filter.IsActive = &v
	}

	rows, err := repo.List(r.Context(), filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	out := make([]providers.ConnectionView, 0, len(rows))
	for i := range rows {
		out = append(out, providers.ToView(&rows[i], nodeNameFn))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

// handleGetProvider implements GET /api/providers/:id.
func handleGetProvider(w http.ResponseWriter, r *http.Request, repo *repository.ProviderRepository, nodeNameFn func(string) string, id string) {
	pc, err := repo.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrProviderNotFound {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": providers.ToView(pc, nodeNameFn)})
}

// handleCreateProvider implements POST /api/providers.
func handleCreateProvider(w http.ResponseWriter, r *http.Request, repo *repository.ProviderRepository, nodeNameFn func(string) string) {
	var req createProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	pc, err := providers.ValidateCreate(toCreateReq(req))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "VALIDATION", err.Error())
		return
	}
	if err := repo.Create(r.Context(), pc); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"connection": providers.ToView(pc, nodeNameFn)})
}

// handleUpdateProvider implements PUT /api/providers/:id.
func handleUpdateProvider(w http.ResponseWriter, r *http.Request, repo *repository.ProviderRepository, nodeNameFn func(string) string) {
	id, _ := extractID(r.URL.Path)
	var req updateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	req.ID = id

	pc, err := repo.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrProviderNotFound {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	if err := providers.ApplyUpdate(pc, toUpdateReq(req)); err != nil {
		writeJSONError(w, http.StatusBadRequest, "VALIDATION", err.Error())
		return
	}
	if err := repo.Update(r.Context(), pc); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": providers.ToView(pc, nodeNameFn)})
}

// handleDeleteProvider implements DELETE /api/providers/:id.
func handleDeleteProvider(w http.ResponseWriter, r *http.Request, repo *repository.ProviderRepository) {
	id, _ := extractID(r.URL.Path)
	if err := repo.Delete(r.Context(), id); err != nil {
		if err == repository.ErrProviderNotFound {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Connection deleted successfully"})
}

// nodeNameLookup returns a closure that loads provider node names
// from the database. Used to enrich compatible-provider display names.
func nodeNameLookup(db *gorm.DB) func(string) string {
	return func(providerID string) string {
		var n model.ProviderNode
		if err := db.Where("id = ?", providerID).First(&n).Error; err != nil {
			return ""
		}
		if n.Name != nil && *n.Name != "" {
			return *n.Name
		}
		var psd map[string]any
		if err := json.Unmarshal([]byte(n.Data), &psd); err == nil {
			if name, ok := psd["name"].(string); ok && name != "" {
				return name
			}
		}
		return ""
	}
}

// handleTestSingle implements POST /api/providers/:id/test.
func handleTestSingle(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id, _ := extractID(r.URL.Path)
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing connection id")
		return
	}
	res, err := providers.TestSingleConnection(db, id)
	if err != nil {
		if err.Error() == "NOT_FOUND" {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":     res.Valid,
		"error":     res.Error,
		"refreshed": res.Refreshed,
	})
}

// handleTestBatch implements POST /api/providers/test-batch.
func handleTestBatch(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var req struct {
		Mode       string `json:"mode"`
		ProviderID string `json:"providerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if req.Mode == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "mode is required")
		return
	}
	if req.Mode == "provider" && req.ProviderID == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "providerId is required for provider mode")
		return
	}
	res, err := providers.TestBatch(db, req.Mode, req.ProviderID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleValidate implements POST /api/providers/validate.
func handleValidate(w http.ResponseWriter, r *http.Request) {
	var req providers.TestInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	res, err := providers.ValidateTests(req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "VALIDATION", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSuggestedModels implements GET /api/providers/suggested-models.
func handleSuggestedModels(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	modelType := r.URL.Query().Get("type")
	if url == "" || modelType == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "url and type query params are required")
		return
	}
	res, err := providers.FetchSuggestedModels(modelType, url, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// writeJSON writes a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// createProviderRequest is the request body for POST /api/providers.
type createProviderRequest struct {
	Provider               string         `json:"provider"`
	AuthType               string         `json:"authType"`
	Name                   *string        `json:"name"`
	Priority               *int           `json:"priority"`
	IsActive               *bool          `json:"isActive"`
	APIKey                 *string        `json:"apiKey"`
	ConnectionProxyEnabled *bool          `json:"connectionProxyEnabled"`
	ConnectionProxyURL     *string        `json:"connectionProxyUrl"`
	ProviderSpecificData   map[string]any `json:"providerSpecificData"`
	ProxyPoolID            *string        `json:"proxyPoolId"`
}

// updateProviderRequest is the request body for PUT /api/providers/:id.
type updateProviderRequest struct {
	ID                     string        `json:"-"` // from URL path
	Name                   *string       `json:"name"`
	Priority               *int          `json:"priority"`
	IsActive               *bool         `json:"isActive"`
	APIKey                 *string       `json:"apiKey"`
	ProviderSpecificData   map[string]any `json:"providerSpecificData"`
}

// toCreateReq converts the API request struct to the providers
// package's CreateReq shape.
func toCreateReq(r createProviderRequest) providers.CreateReq {
	return providers.CreateReq{
		Provider:               r.Provider,
		AuthType:               r.AuthType,
		Name:                   r.Name,
		Priority:               r.Priority,
		IsActive:               r.IsActive,
		APIKey:                 r.APIKey,
		ConnectionProxyEnabled: r.ConnectionProxyEnabled,
		ConnectionProxyURL:     r.ConnectionProxyURL,
		ProviderSpecificData:   r.ProviderSpecificData,
		ProxyPoolID:            r.ProxyPoolID,
	}
}

// toUpdateReq converts the API request struct to the providers
// package's UpdateReq shape.
func toUpdateReq(r updateProviderRequest) providers.UpdateReq {
	return providers.UpdateReq{
		Name:                 r.Name,
		Priority:             r.Priority,
		IsActive:             r.IsActive,
		APIKey:               r.APIKey,
		ProviderSpecificData: r.ProviderSpecificData,
	}
}
