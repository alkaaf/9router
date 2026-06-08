package repository

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/9router/9router/internal/model"
)

// =====================================================================
// Generic JSON helpers
// =====================================================================

// ParseJSON unmarshals s into a new T and returns a *T.
// An empty string returns (nil, nil) — the same convention as the
// existing Node.js parseJson helper, which treats "" as "no data".
// Invalid JSON returns an error.
func ParseJSON[T any](s string) (*T, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out T
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return &out, nil
}

// MustParseJSON is the panicking variant for init-time use only.
func MustParseJSON[T any](s string) *T {
	v, err := ParseJSON[T](s)
	if err != nil {
		panic(err)
	}
	return v
}

// ToJSON marshals v to a JSON string.
func ToJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(b), nil
}

// MustToJSON is the panicking variant for init-time use only.
func MustToJSON(v any) string {
	s, err := ToJSON(v)
	if err != nil {
		panic(err)
	}
	return s
}

// =====================================================================
// Typed data structs
// =====================================================================

// ProviderConnectionData is the canonical payload for a single LLM
// provider credential entry.
type ProviderConnectionData struct {
	AccessToken          string         `json:"accessToken"`
	RefreshToken         string         `json:"refreshToken"`
	ExpiresAt            int64          `json:"expiresAt"`
	ProjectID            string         `json:"projectId"`
	ProviderSpecificData map[string]any `json:"providerSpecificData"`
}

// ProviderNodeData holds the configuration for a single routing node.
// The shape varies by node Type; we keep a permissive map of fields
// (URLs, headers, body transforms) plus a free-form bag for
// type-specific extras.
type ProviderNodeData struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Method  string            `json:"method"`
	Extras  map[string]any    `json:"extras"`
}

// ProxyPoolData is a proxy server configuration.
type ProxyPoolData struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Protocol     string `json:"protocol"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	RotateEvery  int    `json:"rotateEvery"`
	HealthCheck  string `json:"healthCheck"`
}

// ComboModelsData is the ordered list of model identifiers in a combo.
type ComboModelsData []string

// RequestDetailData holds the full request/response metadata captured
// for an audited request.
type RequestDetailData struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	RequestBody string            `json:"requestBody"`
	StatusCode  int               `json:"statusCode"`
	ResponseBody string           `json:"responseBody"`
	DurationMs  int64             `json:"durationMs"`
	Headers     map[string]string `json:"headers"`
}

// UsageHistoryTokens is the per-request token usage breakdown.
type UsageHistoryTokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// UsageHistoryMeta is free-form metadata for a usage row.
type UsageHistoryMeta struct {
	Client string `json:"client"`
	IP     string `json:"ip"`
	User   string `json:"user"`
}

// =====================================================================
// Model Get/Set helpers — typed access to the JSON `Data` column
// =====================================================================
//
// Go does not allow methods on non-local types, so these are
// package-level functions that take the model as the first argument.

func GetConnectionData(m *model.ProviderConnection) (*ProviderConnectionData, error) {
	return ParseJSON[ProviderConnectionData](m.Data)
}

func SetConnectionData(m *model.ProviderConnection, d *ProviderConnectionData) error {
	s, err := ToJSON(d)
	if err != nil {
		return err
	}
	m.Data = s
	return nil
}

func GetNodeData(m *model.ProviderNode) (*ProviderNodeData, error) {
	return ParseJSON[ProviderNodeData](m.Data)
}

func SetNodeData(m *model.ProviderNode, d *ProviderNodeData) error {
	s, err := ToJSON(d)
	if err != nil {
		return err
	}
	m.Data = s
	return nil
}

func GetPoolData(m *model.ProxyPool) (*ProxyPoolData, error) {
	return ParseJSON[ProxyPoolData](m.Data)
}

func SetPoolData(m *model.ProxyPool, d *ProxyPoolData) error {
	s, err := ToJSON(d)
	if err != nil {
		return err
	}
	m.Data = s
	return nil
}

// Combo — Models is the JSON-array column

func GetComboModels(c *model.Combo) (ComboModelsData, error) {
	parsed, err := ParseJSON[[]string](c.Models)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, nil
	}
	return ComboModelsData(*parsed), nil
}

func SetComboModels(c *model.Combo, models ComboModelsData) error {
	s, err := ToJSON([]string(models))
	if err != nil {
		return err
	}
	c.Models = s
	return nil
}

func GetRequestDetailData(m *model.RequestDetail) (*RequestDetailData, error) {
	return ParseJSON[RequestDetailData](m.Data)
}

func SetRequestDetailData(m *model.RequestDetail, d *RequestDetailData) error {
	s, err := ToJSON(d)
	if err != nil {
		return err
	}
	m.Data = s
	return nil
}

// UsageHistory — Tokens and Meta are *string columns holding JSON

func GetUsageTokens(u *model.UsageHistory) (*UsageHistoryTokens, error) {
	if u.Tokens == nil {
		return nil, nil
	}
	return ParseJSON[UsageHistoryTokens](*u.Tokens)
}

func SetUsageTokens(u *model.UsageHistory, t *UsageHistoryTokens) error {
	if t == nil {
		u.Tokens = nil
		return nil
	}
	s, err := ToJSON(t)
	if err != nil {
		return err
	}
	u.Tokens = &s
	return nil
}

func GetUsageMeta(u *model.UsageHistory) (*UsageHistoryMeta, error) {
	if u.Meta == nil {
		return nil, nil
	}
	return ParseJSON[UsageHistoryMeta](*u.Meta)
}

func SetUsageMeta(u *model.UsageHistory, m *UsageHistoryMeta) error {
	if m == nil {
		u.Meta = nil
		return nil
	}
	s, err := ToJSON(m)
	if err != nil {
		return err
	}
	u.Meta = &s
	return nil
}

func GetSettingData(s *model.Setting) (map[string]any, error) {
	parsed, err := ParseJSON[map[string]any](s.Data)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return map[string]any{}, nil
	}
	return *parsed, nil
}

func SetSettingData(s *model.Setting, d map[string]any) error {
	str, err := ToJSON(d)
	if err != nil {
		return err
	}
	s.Data = str
	return nil
}
