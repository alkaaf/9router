package settings

import (
	"encoding/json"
	"errors"
	"net/http"
)

// PricingStore abstracts reading/writing pricing overrides from the kv store.
type PricingStore interface {
	GetPricing() (map[string]any, error)
	SetPricing(data map[string]any) error
	DeletePricing(provider, model string) error
}

// PricingHandler returns an http.HandlerFunc for GET/PATCH/DELETE
// /api/pricing.
func PricingHandler(store PricingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlePricingGet(w, r, store)
		case http.MethodPatch:
			handlePricingPatch(w, r, store)
		case http.MethodDelete:
			handlePricingDelete(w, r, store)
		default:
			w.Header().Set("Allow", "GET, PATCH, DELETE")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handlePricingGet(w http.ResponseWriter, r *http.Request, store PricingStore) {
	overrides, err := store.GetPricing()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	result := deepMerge(defaultPricing(), overrides)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func handlePricingPatch(w http.ResponseWriter, r *http.Request, store PricingStore) {
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if err := validatePricingPatch(patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	existing, err := store.GetPricing()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	merged := deepMerge(existing, patch)
	if err := store.SetPricing(merged); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(deepMerge(defaultPricing(), merged))
}

func handlePricingDelete(w http.ResponseWriter, r *http.Request, store PricingStore) {
	provider := r.URL.Query().Get("provider")
	model := r.URL.Query().Get("model")

	existing, err := store.GetPricing()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	updated := deepCopy(existing)
	switch {
	case provider == "" && model == "":
		updated = map[string]any{}
	case provider != "" && model == "":
		delete(updated, provider)
	case provider != "" && model != "":
		if p, ok := updated[provider].(map[string]any); ok {
			delete(p, model)
			if len(p) == 0 {
				delete(updated, provider)
			}
		}
	}
	if err := store.SetPricing(updated); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(deepMerge(defaultPricing(), updated))
}

// Valid fields for pricing model entries.
var validPricingFields = map[string]bool{
	"input":          true,
	"output":         true,
	"cached":         true,
	"reasoning":      true,
	"cache_creation": true,
}

func validatePricingPatch(patch map[string]any) error {
	for _, pval := range patch {
		pricingMap, ok := pval.(map[string]any)
		if !ok {
			return errors.New("invalid pricing structure: each provider must be an object")
		}
		for _, mval := range pricingMap {
			entry, ok := mval.(map[string]any)
			if !ok {
				return errors.New("invalid pricing structure: each model must be an object")
			}
			for field, val := range entry {
				if !validPricingFields[field] {
					return errors.New("invalid field: " + field)
				}
				if val != nil {
					if _, ok := val.(float64); !ok {
						return errors.New("field " + field + " must be a number")
					}
					if val.(float64) < 0 {
						return errors.New("field " + field + " must be non-negative")
					}
				}
			}
		}
	}
	return nil
}

// defaultPricing returns the built-in pricing defaults.
func defaultPricing() map[string]any {
	return map[string]any{
		"openai": map[string]any{
			"gpt-4o": map[string]any{
				"input":     2.5,
				"output":    10.0,
				"cached":    1.25,
				"reasoning": nil,
			},
			"gpt-4o-mini": map[string]any{
				"input":     0.15,
				"output":    0.6,
				"cached":    0.075,
				"reasoning": nil,
			},
		},
		"anthropic": map[string]any{
			"claude-3-5-sonnet": map[string]any{
				"input":      3.0,
				"output":     15.0,
				"cached":     3.75,
				"reasoning":   3.75,
			},
		},
	}
}

func deepMerge(base, override map[string]any) map[string]any {
	out := deepCopy(base)
	for k, v := range override {
		if bv, ok := out[k]; ok {
			if bm, bok := bv.(map[string]any); bok {
				if vm, vok := v.(map[string]any); vok {
					out[k] = deepMerge(bm, vm)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func deepCopy(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if vm, ok := v.(map[string]any); ok {
			out[k] = deepCopy(vm)
		} else {
			out[k] = v
		}
	}
	return out
}
