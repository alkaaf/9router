package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/9router/9router/internal/repository"
	"gorm.io/gorm"
)

const (
	modelAliasesScope = "modelAliases"
	disabledModelsScope = "disabledModels"
)

// AIBaseModels is the canonical AI_MODELS list. In production this is
// loaded from the application config; here we keep a representative
// subset that matches the dashboard's expectations.
var AIBaseModels = []map[string]any{
	{"provider": "openai", "model": "gpt-4o"},
	{"provider": "openai", "model": "gpt-4o-mini"},
	{"provider": "openai", "model": "o1"},
	{"provider": "openai", "model": "o1-mini"},
	{"provider": "anthropic", "model": "claude-3-5-sonnet"},
	{"provider": "anthropic", "model": "claude-3-5-haiku"},
	{"provider": "anthropic", "model": "claude-3-opus"},
	{"provider": "google", "model": "gemini-2.0-flash"},
	{"provider": "google", "model": "gemini-1.5-pro"},
}

// ModelsHandler returns an http.HandlerFunc for GET/PUT /api/models
// (legacy model list with alias support).
func ModelsHandler(db *gorm.DB) http.HandlerFunc {
	kv := repository.NewKVRepo(db)
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleModelsGet(w, kv)
		case http.MethodPut:
			handleModelsPut(w, r, kv)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handleModelsGet(w http.ResponseWriter, kv *repository.KVRepo) {
	aliases, _ := readAliases(kv)
	disabled, _ := readDisabled(kv)

	out := make([]map[string]any, 0, len(AIBaseModels))
	for _, m := range AIBaseModels {
		provider, _ := m["provider"].(string)
		model, _ := m["model"].(string)
		fullModel := provider + "/" + model

		if isDisabled(disabled, provider, model) {
			continue
		}
		alias := model
		if a, ok := aliases[fullModel]; ok {
			if s, ok := a.(string); ok {
				alias = s
			}
		}
		out = append(out, map[string]any{
			"provider":  provider,
			"model":     model,
			"fullModel": fullModel,
			"alias":     alias,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"models": out})
}

func handleModelsPut(w http.ResponseWriter, r *http.Request, kv *repository.KVRepo) {
	var body struct {
		Model string `json:"model"`
		Alias string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if body.Model == "" || body.Alias == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "model and alias are required")
		return
	}
	aliases, _ := readAliases(kv)
	// Check for duplicate alias mapping to a different model.
	for existingModel, existingAlias := range aliases {
		if existingAlias == body.Alias && existingModel != body.Model {
			writeJSONError(w, http.StatusBadRequest, "DUPLICATE_ALIAS", "alias already used by a different model")
			return
		}
	}
	aliases[body.Model] = body.Alias
	if err := writeAliases(kv, aliases); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"model":   body.Model,
		"alias":   body.Alias,
	})
}

// ModelAliasesHandler returns an http.HandlerFunc for GET/PUT/DELETE
// /api/models/alias.
func ModelAliasesHandler(db *gorm.DB) http.HandlerFunc {
	kv := repository.NewKVRepo(db)
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			aliases, _ := readAliases(kv)
			if aliases == nil {
				aliases = map[string]any{}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"aliases": aliases})
		case http.MethodPut:
			handleModelsPut(w, r, kv)
		case http.MethodDelete:
			handleModelAliasDelete(w, r, kv)
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handleModelAliasDelete(w http.ResponseWriter, r *http.Request, kv *repository.KVRepo) {
	alias := r.URL.Query().Get("alias")
	if alias == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "alias query param required")
		return
	}
	aliases, _ := readAliases(kv)
	for model, a := range aliases {
		if a == alias {
			delete(aliases, model)
		}
	}
	if err := writeAliases(kv, aliases); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func readAliases(kv *repository.KVRepo) (map[string]any, error) {
	v, err := kv.GetJSON(modelAliasesScope, "default")
	if err != nil || v == nil {
		return map[string]any{}, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func writeAliases(kv *repository.KVRepo, m map[string]any) error {
	return kv.SetJSON(modelAliasesScope, "default", m)
}

func readDisabled(kv *repository.KVRepo) (map[string]any, error) {
	v, err := kv.GetJSON(disabledModelsScope, "default")
	if err != nil || v == nil {
		return map[string]any{}, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func isDisabled(disabled map[string]any, provider, model string) bool {
	if disabled == nil {
		return false
	}
	entry, ok := disabled[provider]
	if !ok {
		return false
	}
	models, ok := entry.([]any)
	if !ok {
		return false
	}
	for _, m := range models {
		if ms, ok := m.(string); ok && strings.EqualFold(ms, model) {
			return true
		}
	}
	return false
}
