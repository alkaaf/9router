package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthRefreshConfigEntry describes the per-provider refresh flow.
// Provider IDs map to their canonical form (e.g. "claude", not "cc").
type OAuthRefreshConfigEntry struct {
	// TokenURL is the HTTP endpoint to POST to.
	TokenURL string
	// Method is "form" (application/x-www-form-urlencoded) or "json".
	Method string
	// ClientID is required for OAuth2 providers.
	ClientID string
	// ClientSecret is required by some providers (claude, etc.).
	ClientSecret string
	// ExtraBody is a key/value map of additional form fields to send
	// (provider-specific quirks). Refresh token + grant_type are
	// added automatically.
	ExtraBody map[string]string
	// ExtraHeaders is sent verbatim on the refresh request.
	ExtraHeaders map[string]string
	// ResponseField maps the response JSON field to the canonical
	// token field. Defaults: access_token, refresh_token, expires_in.
	ResponseField ResponseFieldMap
	// SkipWhenMissing is a set of PSD keys whose absence should skip
	// the refresh attempt (some providers need projectId etc.).
	SkipWhenMissing []string
}

// ResponseFieldMap lets providers that don't follow the standard
// OAuth2 response shape still be supported.
type ResponseFieldMap struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    string
	ExpiresAt    string
	ProjectID    string
}

// OAuthRefreshConfig is the per-provider refresh config table.
// New providers can be added without touching the dispatcher.
var OAuthRefreshConfig = map[string]OAuthRefreshConfigEntry{
	"claude": {
		TokenURL:     "https://console.anthropic.com/v1/oauth/token",
		Method:       "json",
		ClientID:     "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		ExtraBody:   map[string]string{},
		ExtraHeaders: map[string]string{"anthropic-version": "2023-06-01"},
		ResponseField: ResponseFieldMap{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresIn:    "expires_in",
		},
	},
	"codex": {
		TokenURL:     "https://auth.openai.com/oauth/token",
		Method:       "form",
		ClientID:     "app_EMoamEEZ73f0CkXaXp7hrann",
		ExtraBody:   map[string]string{},
		ExtraHeaders: map[string]string{},
		ResponseField: ResponseFieldMap{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresIn:    "expires_in",
		},
	},
	"gemini-cli": {
		TokenURL:     "https://oauth2.googleapis.com/token",
		Method:       "form",
		ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		ExtraBody:   map[string]string{},
		ExtraHeaders: map[string]string{},
		ResponseField: ResponseFieldMap{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresIn:    "expires_in",
		},
	},
	"antigravity": {
		TokenURL:     "https://oauth2.googleapis.com/token",
		Method:       "form",
		ClientID:     "1071006060591-tmhahs83f761a3qe2g2h3nhfc0ukfq64.apps.googleusercontent.com",
		ExtraBody:   map[string]string{},
		ExtraHeaders: map[string]string{},
		ResponseField: ResponseFieldMap{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresIn:    "expires_in",
		},
	},
	"qwen": {
		TokenURL:     "https://chat.qwen.ai/api/v1/oauth2/token",
		Method:       "form",
		ClientID:     "f0304373b74a44d2b584a3fb70ca9e56",
		ExtraBody:   map[string]string{},
		ExtraHeaders: map[string]string{},
		ResponseField: ResponseFieldMap{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresIn:    "expires_in",
		},
	},
	"kiro": {
		TokenURL:     "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken",
		Method:       "json",
		ClientID:     "",
		ExtraBody:   map[string]string{},
		ExtraHeaders: map[string]string{},
		ResponseField: ResponseFieldMap{
			AccessToken:  "accessToken",
			RefreshToken: "refreshToken",
			ExpiresIn:    "expiresIn",
		},
	},
	"cline": {
		TokenURL:     "https://api.cline.bot/v1/auth/refresh",
		Method:       "json",
		ClientID:     "",
		ExtraBody:   map[string]string{},
		ExtraHeaders: map[string]string{},
		ResponseField: ResponseFieldMap{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresIn:    "expires_in",
		},
	},
}

// RefreshedTokens is the result of a successful token refresh.
type RefreshedTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	ExpiresAt    int64
	ProjectID    string
}

// RefreshOAuthToken performs an OAuth2 refresh against the
// provider's configured endpoint. It is the implementation behind
// the PROV-014 task and is also called by the validate endpoint
// (PROV-008) when an expired token is presented.
//
// The function always returns the canonical tokens it could
// extract; an error is returned only on transport / decode
// failure or when the provider has no config.
func RefreshOAuthToken(providerID, refreshToken string, psd map[string]any) (string, string, error) {
	if refreshToken == "" {
		return "", "", errors.New("refreshToken missing")
	}
	cfg, ok := OAuthRefreshConfig[providerID]
	if !ok {
		return "", "", errors.New("no refresh config for " + providerID)
	}
	for _, k := range cfg.SkipWhenMissing {
		if psd == nil {
			return "", "", errors.New("missing " + k)
		}
		if v, _ := psd[k].(string); v == "" {
			return "", "", errors.New("missing " + k)
		}
	}
	tokens, err := callRefreshEndpoint(cfg, refreshToken, psd)
	if err != nil {
		return "", "", err
	}
	return tokens.AccessToken, tokens.RefreshToken, nil
}

// CheckAndRefreshToken returns the existing credentials when the
// token is still fresh; otherwise it calls the provider's refresh
// endpoint and returns the new tokens. On any failure it returns
// the original credentials — callers fall through to the next
// connection (PROV-011) or surface the error to the chat handler.
func CheckAndRefreshToken(providerID string, creds *Credentials) (*Credentials, error) {
	if creds == nil {
		return nil, errors.New("nil credentials")
	}
	now := time.Now().UnixMilli()
	buffer := int64(5 * 60 * 1000)
	expiresAt := readExpiresAt(creds.ProviderSpecificData)
	if expiresAt > 0 && expiresAt-buffer > now {
		return creds, nil
	}
	if creds.RefreshToken == "" {
		return creds, nil
	}
	newAccess, newRefresh, err := RefreshOAuthToken(providerID, creds.RefreshToken, creds.ProviderSpecificData)
	if err != nil {
		return creds, err
	}
	creds.AccessToken = newAccess
	if newRefresh != "" {
		creds.RefreshToken = newRefresh
	}
	if creds.ProviderSpecificData == nil {
		creds.ProviderSpecificData = map[string]any{}
	}
	creds.ProviderSpecificData["expiresAt"] = now + 3600*1000
	return creds, nil
}

func readExpiresAt(psd map[string]any) int64 {
	if psd == nil {
		return 0
	}
	if v, ok := psd["expiresAt"].(float64); ok {
		return int64(v)
	}
	return 0
}

func callRefreshEndpoint(cfg OAuthRefreshConfigEntry, refreshToken string, psd map[string]any) (*RefreshedTokens, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var req *http.Request
	var err error
	if cfg.Method == "json" {
		body := map[string]any{
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
		}
		if cfg.ClientID != "" {
			body["client_id"] = cfg.ClientID
		}
		if cfg.ClientSecret != "" {
			body["client_secret"] = cfg.ClientSecret
		}
		for k, v := range cfg.ExtraBody {
			body[k] = v
		}
		bodyBytes, _ := json.Marshal(body)
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refreshToken)
		if cfg.ClientID != "" {
			form.Set("client_id", cfg.ClientID)
		}
		if cfg.ClientSecret != "" {
			form.Set("client_secret", cfg.ClientSecret)
		}
		for k, v := range cfg.ExtraBody {
			form.Set(k, v)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range cfg.ExtraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("refresh http %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	rf := cfg.ResponseField
	if rf.AccessToken == "" {
		rf.AccessToken = "access_token"
	}
	if rf.RefreshToken == "" {
		rf.RefreshToken = "refresh_token"
	}
	if rf.ExpiresIn == "" {
		rf.ExpiresIn = "expires_in"
	}
	out := &RefreshedTokens{}
	if v, ok := raw[rf.AccessToken].(string); ok {
		out.AccessToken = v
	}
	if v, ok := raw[rf.RefreshToken].(string); ok {
		out.RefreshToken = v
	}
	switch n := raw[rf.ExpiresIn].(type) {
	case float64:
		out.ExpiresIn = int64(n)
	case int64:
		out.ExpiresIn = n
	}
	if out.ExpiresIn > 0 {
		out.ExpiresAt = time.Now().UnixMilli() + out.ExpiresIn*1000
	} else if rf.ExpiresAt != "" {
		if v, ok := raw[rf.ExpiresAt].(float64); ok {
			out.ExpiresAt = int64(v)
		}
	}
	if rf.ProjectID != "" {
		if v, ok := raw[rf.ProjectID].(string); ok {
			out.ProjectID = v
		}
	}
	if out.AccessToken == "" {
		return nil, errors.New("refresh response missing access_token")
	}
	return out, nil
}
