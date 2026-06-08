package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// TranslatorState is the persisted translator configuration.
type TranslatorState struct {
	Format      string                 `json:"format"`
	Translators map[string]any         `json:"translators,omitempty"`
	Settings    map[string]any         `json:"settings,omitempty"`
}

// TranslatorSettingsRepo is the repo for the settings row keyed "translator".
type TranslatorSettingsRepo interface {
	GetByKey(ctx context.Context, key string) (string, error)
	UpsertByKey(ctx context.Context, key, value string) error
}

var (
	translatorRepoMu sync.RWMutex
	translatorRepo   TranslatorSettingsRepo
)

// SetTranslatorSettingsRepo sets the translator settings repo.
func SetTranslatorSettingsRepo(r TranslatorSettingsRepo) {
	translatorRepoMu.Lock()
	defer translatorRepoMu.Unlock()
	translatorRepo = r
}

func currentTranslatorRepo() TranslatorSettingsRepo {
	translatorRepoMu.RLock()
	defer translatorRepoMu.RUnlock()
	return translatorRepo
}

func defaultTranslatorState() TranslatorState {
	return TranslatorState{
		Format:      "openai",
		Translators: map[string]any{},
		Settings:    map[string]any{},
	}
}

// HandleTranslatorState implements GET /api/translator/state.
func HandleTranslatorState(c *Context) (any, error) {
	repo := currentTranslatorRepo()
	if repo == nil {
		return defaultTranslatorState(), nil
	}
	raw, err := repo.GetByKey(c.Ctx, "translator")
	if err != nil || raw == "" {
		return defaultTranslatorState(), nil
	}
	var state TranslatorState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return defaultTranslatorState(), nil
	}
	if state.Format == "" {
		state.Format = "openai"
	}
	if state.Translators == nil {
		state.Translators = map[string]any{}
	}
	if state.Settings == nil {
		state.Settings = map[string]any{}
	}
	return state, nil
}

// HandleTranslatorSave implements PUT /api/translator/config.
func HandleTranslatorSave(c *Context) (any, error) {
	if len(c.Body) == 0 {
		return nil, NewHandlerError("BAD_REQUEST", "Request body is required")
	}
	var req TranslatorState
	if err := json.Unmarshal(c.Body, &req); err != nil {
		return nil, NewHandlerError("BAD_REQUEST", "Invalid JSON body")
	}
	if req.Format == "" {
		return nil, NewHandlerError("BAD_REQUEST", "format is required")
	}

	repo := currentTranslatorRepo()
	if repo == nil {
		return map[string]any{"success": true}, nil
	}

	existingRaw, _ := repo.GetByKey(c.Ctx, "translator")
	if existingRaw != "" {
		var existing TranslatorState
		if err := json.Unmarshal([]byte(existingRaw), &existing); err == nil {
			if existing.Format == "" {
				existing.Format = req.Format
			}
			if existing.Translators == nil {
				existing.Translators = map[string]any{}
			}
			for k, v := range req.Translators {
				existing.Translators[k] = v
			}
			if existing.Settings == nil {
				existing.Settings = map[string]any{}
			}
			for k, v := range req.Settings {
				existing.Settings[k] = v
			}
			if req.Format != "" {
				existing.Format = req.Format
			}
			req = existing
		}
	}

	merged, err := json.Marshal(req)
	if err != nil {
		return nil, NewHandlerError("BAD_REQUEST", fmt.Sprintf("marshal: %v", err))
	}
	if err := repo.UpsertByKey(c.Ctx, "translator", string(merged)); err != nil {
		return nil, NewHandlerError("DB_ERROR", fmt.Sprintf("failed to save translator config: %v", err))
	}
	return map[string]any{"success": true}, nil
}
