package providers

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/9router/9router/internal/model"
)

// ErrNotFound is returned when a connection lookup yields no row.
var ErrNotFound = errors.New("provider connection not found")

// ErrInvalidProvider is returned when a request references an
// unknown provider ID.
var ErrInvalidProvider = errors.New("invalid or unknown provider")

// ErrMissingAPIKey is returned when an apikey-authed connection
// is missing the apiKey field (and the provider is not noAuth).
var ErrMissingAPIKey = errors.New("apiKey is required for apikey auth")

// ErrMissingProxyURL is returned when proxy is enabled but URL is
// missing.
var ErrMissingProxyURL = errors.New("connectionProxyUrl is required when proxy is enabled")

// ConnectionView is the API-shaped projection of a provider
// connection. Sensitive fields (apiKey, accessToken, refreshToken,
// idToken) are never present in this struct — they are stripped
// from the underlying Data blob before serialisation.
type ConnectionView struct {
	ID          string         `json:"id"`
	Provider    string         `json:"provider"`
	AuthType    string         `json:"authType"`
	Name        *string        `json:"name,omitempty"`
	Priority    *int           `json:"priority,omitempty"`
	IsActive    *bool          `json:"isActive"`
	TestStatus  *string        `json:"testStatus,omitempty"`
	LastError   *string        `json:"lastError,omitempty"`
	ProviderSpecificData map[string]any `json:"providerSpecificData,omitempty"`
	CreatedAt   int64          `json:"createdAt"`
	UpdatedAt   int64          `json:"updatedAt"`
}

// ToView converts a model.ProviderConnection into a ConnectionView.
// The nodeNameLookup callback is used to enrich compatible-provider
// names from the node registry; pass nil to skip.
func ToView(pc *model.ProviderConnection, nodeNameLookup func(string) string) ConnectionView {
	if pc == nil {
		return ConnectionView{}
	}
	v := ConnectionView{
		ID:        pc.ID,
		Provider:  pc.Provider,
		AuthType:  pc.AuthType,
		Name:      pc.Name,
		Priority:  pc.Priority,
		IsActive:  pc.IsActive,
		CreatedAt: pc.CreatedAt.UnixMilli(),
		UpdatedAt: pc.UpdatedAt.UnixMilli(),
	}
	if pc.Data != "" {
		psd := decodePSD(pc.Data)
		stripSensitive(psd)
		v.ProviderSpecificData = psd
		if s, ok := psd["testStatus"].(string); ok && s != "" {
			v.TestStatus = &s
		}
		if s, ok := psd["lastError"].(string); ok && s != "" {
			v.LastError = &s
		}
	}
	if v.Name != nil && *v.Name == "" && IsCompatible(v.Provider) && nodeNameLookup != nil {
		if n := nodeNameLookup(v.Provider); n != "" {
			v.Name = &n
		}
	}
	return v
}

// decodePSD parses the JSON data column into a map. Returns an
// empty map on parse error so the rest of the view stays populated.
func decodePSD(data string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(data) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// sensitiveKeys is the set of fields that must never appear in an
// API response, even when embedded in the provider-specific data map.
var sensitiveKeys = map[string]struct{}{
	"apiKey":       {},
	"accessToken":  {},
	"refreshToken": {},
	"idToken":      {},
}

func stripSensitive(m map[string]any) {
	if m == nil {
		return
	}
	for k := range m {
		if _, bad := sensitiveKeys[k]; bad {
			delete(m, k)
		}
	}
}

// --- create ---

// ValidateCreate validates a create request and returns the
// model.ProviderConnection to persist. It is exported so the
// handler can call it directly without duplicating logic.
func ValidateCreate(req CreateReq) (*model.ProviderConnection, error) {
	if !IsKnownProvider(req.Provider) {
		return nil, ErrInvalidProvider
	}
	authType := req.AuthType
	if authType == "" {
		if isCookieProvider(req.Provider) {
			authType = "cookie"
		} else {
			authType = "apikey"
		}
	}
	if req.Name == nil || *req.Name == "" {
		return nil, errors.New("name is required")
	}
	if authType == "apikey" && !isNoAuthProvider(req.Provider) {
		if req.APIKey == nil || *req.APIKey == "" {
			return nil, ErrMissingAPIKey
		}
	}
	if req.ConnectionProxyEnabled != nil && *req.ConnectionProxyEnabled {
		if req.ConnectionProxyURL == nil || *req.ConnectionProxyURL == "" {
			return nil, ErrMissingProxyURL
		}
	}

	psd := buildPSDFromCreate(req)
	dataBytes, _ := json.Marshal(psd)

	return &model.ProviderConnection{
		Provider: req.Provider,
		AuthType: authType,
		Name:     req.Name,
		Priority: req.Priority,
		IsActive: req.IsActive,
		Data:     string(dataBytes),
	}, nil
}

// CreateReq is the slim create-request shape. Mirrors the
// api.createProviderRequest in handler/api/providers.go.
type CreateReq struct {
	Provider               string
	AuthType               string
	Name                   *string
	Priority               *int
	IsActive               *bool
	APIKey                 *string
	ConnectionProxyEnabled *bool
	ConnectionProxyURL     *string
	ProviderSpecificData   map[string]any
	ProxyPoolID            *string
}

// buildPSDFromCreate assembles the JSON data blob.
func buildPSDFromCreate(req CreateReq) map[string]any {
	psd := map[string]any{}
	if req.ProviderSpecificData != nil {
		for k, v := range req.ProviderSpecificData {
			psd[k] = v
		}
	}
	if req.APIKey != nil && *req.APIKey != "" {
		psd["apiKey"] = *req.APIKey
	}
	if req.ConnectionProxyEnabled != nil {
		psd["connectionProxyEnabled"] = *req.ConnectionProxyEnabled
	}
	if req.ConnectionProxyURL != nil && *req.ConnectionProxyURL != "" {
		psd["connectionProxyUrl"] = *req.ConnectionProxyURL
	}
	if req.ProxyPoolID != nil && *req.ProxyPoolID != "" {
		psd["proxyPoolId"] = *req.ProxyPoolID
	}
	return psd
}

// --- update ---

// ApplyUpdate mutates pc in place with the supplied partial update.
// It also performs simple validation: apiKey is only accepted for
// apikey authType, and proxy config is validated when present.
func ApplyUpdate(pc *model.ProviderConnection, req UpdateReq) error {
	if pc == nil {
		return ErrNotFound
	}
	if req.Name != nil {
		pc.Name = req.Name
	}
	if req.Priority != nil {
		pc.Priority = req.Priority
	}
	if req.IsActive != nil {
		pc.IsActive = req.IsActive
	}
	psd := decodePSD(pc.Data)
	if req.APIKey != nil {
		if pc.AuthType != "apikey" {
			return errors.New("apiKey can only be updated for apikey authType")
		}
		psd["apiKey"] = *req.APIKey
	}
	if req.ProviderSpecificData != nil {
		for k, v := range req.ProviderSpecificData {
			psd[k] = v
		}
	}
	if enabled, ok := psd["connectionProxyEnabled"].(bool); ok && enabled {
		if url, _ := psd["connectionProxyUrl"].(string); url == "" {
			return ErrMissingProxyURL
		}
	}
	dataBytes, _ := json.Marshal(psd)
	pc.Data = string(dataBytes)
	return nil
}

// UpdateReq mirrors api.updateProviderRequest.
type UpdateReq struct {
	Name                 *string
	Priority             *int
	IsActive             *bool
	APIKey               *string
	ProviderSpecificData map[string]any
}

// --- provider type detection ---

func isCookieProvider(providerID string) bool {
	switch providerID {
	case "grok-web", "perplexity-web", "iflow", "cursor", "kiro":
		return true
	}
	return false
}

func isNoAuthProvider(providerID string) bool {
	switch providerID {
	case "ollama-local", "openrouter", "free":
		return true
	}
	return false
}
