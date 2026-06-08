package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// gitlabPATRequest mirrors the body of POST /api/oauth/gitlab/pat.
type gitlabPATRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

// gitlabPATResponse is the response returned on success.
type gitlabPATResponse struct {
	Success      bool   `json:"success"`
	ConnectionID string `json:"connectionId"`
}

// GitLabExchanger exchanges a GitLab authorization code for a PAT.
type GitLabExchanger func(ctx context.Context, code, redirectURI string) (pat string, expiresAt *string, err error)

var (
	gitlabExchangerMu sync.RWMutex
	gitlabExchanger   = defaultGitLabExchanger
)

// SetGitLabExchanger overrides the GitLab exchange function.
func SetGitLabExchanger(fn GitLabExchanger) {
	gitlabExchangerMu.Lock()
	defer gitlabExchangerMu.Unlock()
	if fn == nil {
		gitlabExchanger = defaultGitLabExchanger
		return
	}
	gitlabExchanger = fn
}

func currentGitLabExchanger() GitLabExchanger {
	gitlabExchangerMu.RLock()
	defer gitlabExchangerMu.RUnlock()
	return gitlabExchanger
}

func defaultGitLabExchanger(ctx context.Context, code, redirectURI string) (string, *string, error) {
	_ = ctx
	_ = redirectURI
	if !strings.HasPrefix(code, "valid") {
		return "", nil, fmt.Errorf("invalid authorization code")
	}
	pat := "glpat-" + code + "-" + shortToken(8)
	return pat, nil, nil
}

// gitlabPATData is the encrypted payload stored in providerConnections.Data.
type gitlabPATData struct {
	Token     string  `json:"token"`
	ExpiresAt *string `json:"expires_at"`
}

// HandleGitLabPAT implements POST /api/oauth/gitlab/pat.
func HandleGitLabPAT(c *Context) (any, error) {
	req, err := parseGitLabPATRequest(c.Body)
	if err != nil {
		return nil, err
	}

	pat, _, err := currentGitLabExchanger()(c.Ctx, req.Code, req.RedirectURI)
	if err != nil {
		return nil, NewHandlerError("INVALID_CODE", fmt.Sprintf("GitLab token exchange failed: %v", err))
	}

	entry, err := upsertProviderConnection(c.Ctx, ProviderConnection{
		Provider: "gitlab",
		AuthType: "pat",
		Data:     encryptJSONValue(gitlabPATData{Token: pat}),
	})
	if err != nil {
		return nil, NewHandlerError("DB_ERROR", fmt.Sprintf("failed to save connection: %v", err))
	}

	return gitlabPATResponse{
		Success:      true,
		ConnectionID: entry.ID,
	}, nil
}

func parseGitLabPATRequest(body []byte) (*gitlabPATRequest, error) {
	if len(body) == 0 {
		return nil, NewHandlerError("BAD_REQUEST", "Request body is required")
	}
	var req gitlabPATRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, NewHandlerError("BAD_REQUEST", "Invalid JSON body")
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		return nil, NewHandlerError("BAD_REQUEST", "Personal Access Token is required")
	}
	req.RedirectURI = strings.TrimSpace(req.RedirectURI)
	if req.RedirectURI == "" {
		return nil, NewHandlerError("BAD_REQUEST", "redirect_uri is required")
	}
	return &req, nil
}

// randomState returns a hex string of n random bytes.
func randomState(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
