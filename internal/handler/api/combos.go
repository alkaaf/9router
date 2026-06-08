package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/9router/9router/internal/model"
	"github.com/9router/9router/internal/repository"
	"gorm.io/gorm"
)

// nameRegexp allows letters, numbers, hyphens, underscores, dots.
var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

// CombosHandler returns an http.HandlerFunc for GET/POST /api/combos.
func CombosHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo := repository.NewComboRepository(db)
		ctx := r.Context()
		switch r.Method {
		case http.MethodGet:
			rows, err := repo.ListAll()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch combos"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"combos": rows})
		case http.MethodPost:
			var body struct {
				Name   string   `json:"name"`
				Models []string `json:"models"`
				Kind   *string  `json:"kind"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
				return
			}
			if strings.TrimSpace(body.Name) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
				return
			}
			if !nameRegexp.MatchString(body.Name) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name can only contain letters, numbers, -, _ and ."})
				return
			}
			existing, err := repo.FindByName(ctx, body.Name)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create combo"})
				return
			}
			if existing != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Combo name already exists"})
				return
			}
			modelsJSON, _ := json.Marshal(body.Models)
			c := &model.Combo{
				Name:   body.Name,
				Models: string(modelsJSON),
				Kind:   body.Kind,
			}
			if err := repo.Create(ctx, c); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create combo"})
				return
			}
			writeJSON(w, http.StatusCreated, c)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

// ComboHandler returns an http.HandlerFunc for GET/PUT/DELETE /api/combos/:id.
func ComboHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo := repository.NewComboRepository(db)
		ctx := r.Context()
		id := extractComboID(r.URL.Path)
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing combo id"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			c, err := repo.FindByID(ctx, id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch combo"})
				return
			}
			if c == nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Combo not found"})
				return
			}
			writeJSON(w, http.StatusOK, c)
		case http.MethodPut:
			c, err := repo.FindByID(ctx, id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update combo"})
				return
			}
			if c == nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Combo not found"})
				return
			}
			var body struct {
				Name   *string  `json:"name"`
				Models []string `json:"models"`
				Kind   *string  `json:"kind"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
				return
			}
			if body.Name != nil {
				c.Name = *body.Name
			}
			if body.Models != nil {
				modelsJSON, _ := json.Marshal(body.Models)
				c.Models = string(modelsJSON)
			}
			if body.Kind != nil {
				c.Kind = body.Kind
			}
			if err := repo.Update(ctx, c); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update combo"})
				return
			}
			writeJSON(w, http.StatusOK, c)
		case http.MethodDelete:
			ok, err := repo.Delete(ctx, id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete combo"})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Combo not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"message": "Combo deleted successfully"})
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func extractComboID(path string) string {
	const prefix = "/api/combos"
	path = strings.TrimPrefix(path, prefix)
	path = strings.Trim(path, "/")
	if i := strings.Index(path, "/"); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		return ""
	}
	return path
}
