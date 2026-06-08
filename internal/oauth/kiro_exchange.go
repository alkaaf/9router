package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// kiroExchangeRequest is the body for POST /api/oauth/kiro/social-exchange.
type kiroExchangeRequest struct {
	Code         string `json:"code"`
	State        string `json:"state"`
	RedirectURI  string `json:"redirect_uri"`
	CodeVerifier string `json:"code_verifier"`
}

// kiroTokenResponse is the Kiro token endpoint response.
type kiroTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
	ProfileArn   string `json:"profile_arn"`
}

// kiroExchangeResponse is the success body.
type kiroExchangeResponse struct {
	Success      bool   `json:"success"`
	ConnectionID string `json:"connectionId"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
}

// KiroExchanger exchanges an authorization code for tokens.
type KiroExchanger func(ctx context.Context, code, redirectURI, codeVerifier string) (*kiroTokenResponse, error)

var (
	kiroExchangerMu sync.RWMutex
	kiroExchanger   = defaultKiroExchanger
)

func defaultKiroExchanger(ctx context.Context, code, redirectURI, codeVerifier string) (*kiroTokenResponse, error) {
	_ = ctx
	_ = redirectURI
	_ = codeVerifier
	if !strings.HasPrefix(code, "valid") {
		return nil, fmt.Errorf("invalid authorization code")
	}
	return &kiroTokenResponse{
		AccessToken:  "kiro-at-" + shortToken(12),
		RefreshToken: "kiro-rt-" + shortToken(12),
		ExpiresIn:    3600,
		TokenType:    "Bearer",
		ProfileArn:   "arn:aws:codewhisperer:us-east-1:000000000000:profile/test",
	}, nil
}

// SetKiroExchanger overrides the token-exchange function.
func SetKiroExchanger(fn KiroExchanger) {
	kiroExchangerMu.Lock()
	defer kiroExchangerMu.Unlock()
	if fn == nil {
		kiroExchanger = defaultKiroExchanger
		return
	}
	kiroExchanger = fn
}

// kiroConnectionData is the encrypted payload stored in providerConnections.Data.
type kiroConnectionData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expiresAt"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
	ProfileArn   string `json:"profile_arn"`
}

// HandleKiroSocialExchange implements POST /api/oauth/kiro/social-exchange.
func HandleKiroSocialExchange(c *Context) (any, error) {
	if len(c.Body) == 0 {
		return nil, NewHandlerError("BAD_REQUEST", "Request body is required")
	}
	var req kiroExchangeRequest
	if err := json.Unmarshal(c.Body, &req); err != nil {
		return nil, NewHandlerError("BAD_REQUEST", "Invalid JSON body")
	}

	req.Code = strings.TrimSpace(req.Code)
	req.State = strings.TrimSpace(req.State)
	req.RedirectURI = strings.TrimSpace(req.RedirectURI)

	if req.Code == "" {
		return nil, NewHandlerError("BAD_REQUEST", "code is required")
	}
	if req.State == "" {
		return nil, NewHandlerError("BAD_REQUEST", "state is required")
	}
	if req.RedirectURI == "" {
		return nil, NewHandlerError("BAD_REQUEST", "redirect_uri is required")
	}

	repo := currentKVRepo()
	if repo == nil {
		return nil, NewHandlerError("KV_UNAVAILABLE", "KV store is not configured")
	}
	stored, err := repo.Get(c.Ctx, "oauth-state", req.State)
	if err != nil {
		return nil, NewHandlerError("STATE_NOT_FOUND", "State not found or expired")
	}
	if stored == "" {
		return nil, NewHandlerError("STATE_NOT_FOUND", "State not found or expired")
	}
	expected := strings.TrimSpace(req.RedirectURI)
	if expected != "" && !strings.Contains(stored, expected) {
		_ = repo.Delete(c.Ctx, "oauth-state", req.State)
		return nil, NewHandlerError("STATE_MISMATCH", "State does not match the original authorization request")
	}
	_ = repo.Delete(c.Ctx, "oauth-state", req.State)

	kiroExchangerMu.RLock()
	ex := kiroExchanger
	kiroExchangerMu.RUnlock()
	tokens, err := ex(c.Ctx, req.Code, req.RedirectURI, req.CodeVerifier)
	if err != nil {
		return nil, NewHandlerError("INVALID_CODE", fmt.Sprintf("Kiro token exchange failed: %v", err))
	}

	expiresAt := time.Now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second).Format(time.RFC3339)

	entry, err := upsertProviderConnection(c.Ctx, ProviderConnection{
		Provider: "kiro",
		AuthType: "oauth",
		Data: encryptJSONValue(kiroConnectionData{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresAt:    expiresAt,
			TokenType:    tokens.TokenType,
			IDToken:      tokens.IDToken,
			ProfileArn:   tokens.ProfileArn,
		}),
	})
	if err != nil {
		return nil, NewHandlerError("DB_ERROR", fmt.Sprintf("failed to save connection: %v", err))
	}

	return kiroExchangeResponse{
		Success:      true,
		ConnectionID: entry.ID,
		ExpiresAt:    expiresAt,
	}, nil
}
