// Package providers implements the provider-alias resolution and
// model-string parser used by the chat handler and the connection
// CRUD endpoints. The split between the alias table and the parser
// matches the open-sse/services/model.js helpers and lets each piece
// be unit-tested independently.
package providers

import (
	"errors"
	"strings"
)

// ModelInfo is the output of the model-string parser.
type ModelInfo struct {
	Provider string
	Model    string
	IsAlias  bool
}

// ParseModelString splits a model string into { provider, model }.
// It supports three formats:
//
//   - "provider/model" — explicit provider (split on the first "/").
//   - "provider:model" — OpenAI-compatible style (split on the first ":").
//   - "model"          — bare model name; defaultProvider is required.
//
// The provider segment is always passed through ResolveProviderID
// so that short aliases ("cc" → "claude") and compatible prefixes
// are normalised before the result is returned.
func ParseModelString(input, defaultProvider string) (ModelInfo, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ModelInfo{}, errors.New("empty model string")
	}

	if i := strings.Index(input, "/"); i >= 0 {
		provider := ResolveProviderID(input[:i])
		if !IsKnownProvider(provider) {
			return ModelInfo{}, errors.New("unknown provider: " + input[:i])
		}
		return ModelInfo{Provider: provider, Model: input[i+1:]}, nil
	}

	if i := strings.Index(input, ":"); i >= 0 {
		provider := ResolveProviderID(input[:i])
		if !IsKnownProvider(provider) {
			return ModelInfo{}, errors.New("unknown provider: " + input[:i])
		}
		return ModelInfo{Provider: provider, Model: input[i+1:]}, nil
	}

	if defaultProvider == "" {
		return ModelInfo{}, errors.New("bare model with no default provider")
	}
	provider := ResolveProviderID(defaultProvider)
	if !IsKnownProvider(provider) {
		return ModelInfo{}, errors.New("unknown provider: " + defaultProvider)
	}
	return ModelInfo{Provider: provider, Model: input, IsAlias: true}, nil
}
