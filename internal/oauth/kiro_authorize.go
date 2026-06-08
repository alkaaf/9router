package oauth

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// kiroAuthorizeResponse is the success body.
type kiroAuthorizeResponse struct {
	AuthURL string `json:"authUrl"`
	State   string `json:"state"`
}

// KiroAuthorizeBuilder builds the Kiro authorization URL.
type KiroAuthorizeBuilder func(clientID, redirectURI, state, codeChallenge string) string

var (
	kiroBuilderMu sync.RWMutex
	kiroBuilder   = defaultKiroAuthorizeBuilder
)

func defaultKiroAuthorizeBuilder(clientID, redirectURI, state, codeChallenge string) string {
	return fmt.Sprintf("https://kiro.example.com/oauth/authorize?client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s",
		clientID, redirectURI, state, codeChallenge)
}

// SetKiroAuthorizeBuilder overrides the auth-URL builder.
func SetKiroAuthorizeBuilder(fn KiroAuthorizeBuilder) {
	kiroBuilderMu.Lock()
	defer kiroBuilderMu.Unlock()
	if fn == nil {
		kiroBuilder = defaultKiroAuthorizeBuilder
		return
	}
	kiroBuilder = fn
}

// HandleKiroSocialAuthorize implements GET /api/oauth/kiro/social-authorize.
func HandleKiroSocialAuthorize(c *Context) (any, error) {
	clientID := strings.TrimSpace(c.Query["client_id"])
	redirectURI := strings.TrimSpace(c.Query["redirect_uri"])
	state := strings.TrimSpace(c.Query["state"])

	if clientID == "" {
		return nil, NewHandlerError("BAD_REQUEST", "client_id is required")
	}
	if redirectURI == "" {
		return nil, NewHandlerError("BAD_REQUEST", "redirect_uri is required")
	}

	if state == "" {
		state = randomState(16)
	}

	codeChallenge := strings.TrimSpace(c.Query["code_challenge"])

	repo := currentKVRepo()
	if repo != nil {
		ttl := time.Now().UTC().Add(10 * time.Minute)
		if err := repo.Set(c.Ctx, "oauth-state", state, clientID+"|"+redirectURI, &ttl); err != nil {
			return nil, NewHandlerError("KV_ERROR", fmt.Sprintf("failed to store state: %v", err))
		}
	}

	kiroBuilderMu.RLock()
	builder := kiroBuilder
	kiroBuilderMu.RUnlock()

	return kiroAuthorizeResponse{
		AuthURL: builder(clientID, redirectURI, state, codeChallenge),
		State:   state,
	}, nil
}
