package api

import (
	"net/http"

	"github.com/9router/9router/internal/repository"
	"github.com/9router/9router/internal/settings"
	"gorm.io/gorm"
)

const pricingScope = "pricing"

// PricingHandler returns an http.HandlerFunc for GET/PATCH/DELETE
// /api/pricing. The store is backed by the kv table.
func PricingHandler(db *gorm.DB) http.HandlerFunc {
	kv := repository.NewKVRepo(db)
	store := &kvPricingStore{kv: kv}
	return settings.PricingHandler(store)
}

type kvPricingStore struct {
	kv *repository.KVRepo
}

func (s *kvPricingStore) GetPricing() (map[string]any, error) {
	v, err := s.kv.GetJSON(pricingScope, "default")
	if err != nil {
		return nil, err
	}
	if v == nil {
		return map[string]any{}, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func (s *kvPricingStore) SetPricing(data map[string]any) error {
	return s.kv.SetJSON(pricingScope, "default", data)
}

func (s *kvPricingStore) DeletePricing(provider, model string) error {
	current, err := s.GetPricing()
	if err != nil {
		return err
	}
	switch {
	case provider == "" && model == "":
		current = map[string]any{}
	case provider != "" && model == "":
		delete(current, provider)
	default:
		if p, ok := current[provider].(map[string]any); ok {
			delete(p, model)
			if len(p) == 0 {
				delete(current, provider)
			}
		}
	}
	return s.kv.SetJSON(pricingScope, "default", current)
}

// Compile-time check that kvPricingStore implements settings.PricingStore.
var _ settings.PricingStore = (*kvPricingStore)(nil)
