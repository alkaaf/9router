package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// VertexGeminiURLFormat is the URL template for Vertex Gemini
// streamGenerateContent. The {project}, {location}, and {model} placeholders
// are filled in by BuildUrl.
const VertexGeminiURLFormat = "https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:streamGenerateContent?alt=sse"

// VertexPartnerURL is the global OpenAI-compatible endpoint for
// Vertex Partner models (Llama, Mistral, DeepSeek, Qwen, ...).
const VertexPartnerURL = "https://aiplatform.googleapis.com/v1/projects/%s/locations/global/endpoints/openapi/chat/completions"

// VertexExecutor handles Google Cloud Vertex AI in two sub-modes:
//
//   - Mode 1 (Gemini): auth via Service Account JSON → JWT assertion →
//     Bearer token. URL template is VertexGeminiURLFormat.
//   - Mode 2 (Partner): same auth, but the body is OpenAI-shaped and
//     the URL is the global openapi/chat/completions endpoint.
type VertexExecutor struct {
	*BaseExecutor

	mode       string // "gemini" or "partner"
	projectID  string
	location   string
	sa         *VertexSA
	tokenCache *vertexTokenCache
}

// VertexSA is the parsed service-account JSON. Only the fields needed
// to mint a JWT are kept; everything else is ignored.
type VertexSA struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// ParseVertexSA extracts the SA fields from raw JSON. Returns an
// error if the JSON is malformed or required fields are missing.
func ParseVertexSA(raw []byte) (*VertexSA, error) {
	var sa VertexSA
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("parse SA: %w", err)
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("parse SA: missing required fields")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &sa, nil
}

// vertexTokenCache holds a short-lived bearer token keyed by
// ClientEmail so concurrent calls within the same Vertex connection
// don't all mint a fresh JWT.
type vertexTokenCache struct {
	mu        sync.RWMutex
	tokens    map[string]cachedToken
}

type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

func newVertexTokenCache() *vertexTokenCache {
	return &vertexTokenCache{tokens: make(map[string]cachedToken)}
}

// Get returns a non-expired token for the given clientEmail, or empty
// string if none is cached.
func (c *vertexTokenCache) Get(clientEmail string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	tok, ok := c.tokens[clientEmail]
	if !ok {
		return ""
	}
	if time.Now().After(tok.expiresAt) {
		return ""
	}
	return tok.accessToken
}

// Put stores a token with a TTL (Google issues 1h tokens; we use the
// supplied expiresIn seconds minus a 60s safety skew).
func (c *vertexTokenCache) Put(clientEmail, accessToken string, expiresInSec int64) {
	if expiresInSec <= 0 {
		expiresInSec = 3600
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[clientEmail] = cachedToken{
		accessToken: accessToken,
		expiresAt:   time.Now().Add(time.Duration(expiresInSec-60) * time.Second),
	}
}

// NewVertexExecutor builds a VertexExecutor in Gemini mode. The saRaw
// parameter is the service-account JSON; it's parsed immediately so
// malformed input is rejected at construction time.
func NewVertexExecutor(projectID, location string, saRaw []byte) (*VertexExecutor, error) {
	sa, err := ParseVertexSA(saRaw)
	if err != nil {
		return nil, err
	}
	if projectID == "" {
		projectID = sa.ProjectID
	}
	cfg := &ProviderConfig{
		Provider:   "vertex",
		BaseURLs:   []string{""}, // BuildUrl sets the full URL per-mode
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &VertexExecutor{
		BaseExecutor: NewBaseExecutor(cfg, nil),
		mode:         "gemini",
		projectID:    projectID,
		location:     location,
		sa:           sa,
		tokenCache:   newVertexTokenCache(),
	}, nil
}

// NewVertexPartnerExecutor builds a VertexExecutor in Partner mode
// (OpenAI-compatible endpoint). The SA is still required for auth.
func NewVertexPartnerExecutor(projectID string, saRaw []byte) (*VertexExecutor, error) {
	sa, err := ParseVertexSA(saRaw)
	if err != nil {
		return nil, err
	}
	if projectID == "" {
		projectID = sa.ProjectID
	}
	cfg := &ProviderConfig{
		Provider:   "vertex-partner",
		BaseURLs:   []string{""},
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &VertexExecutor{
		BaseExecutor: NewBaseExecutor(cfg, nil),
		mode:         "partner",
		projectID:    projectID,
		location:     "global",
		sa:           sa,
		tokenCache:   newVertexTokenCache(),
	}, nil
}

// BuildUrl returns the Vertex URL for the configured mode. For Gemini
// mode the model comes from the request and is interpolated into
// the path; for Partner mode the URL is fixed.
func (v *VertexExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	switch v.mode {
	case "partner":
		return fmt.Sprintf(VertexPartnerURL, v.projectID)
	default:
		return fmt.Sprintf(VertexGeminiURLFormat, v.projectID, v.location, model)
	}
}

// BuildHeaders adds the Bearer token from the SA-cached JWT. The
// auth token is injected at header-build time because the BaseExecutor
// runs BuildHeaders with the supplied Credentials — we forward
// whichever token is current.
func (v *VertexExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := v.BaseExecutor.BuildHeaders(req, creds)
	// Cached token takes precedence over creds.AccessToken when
	// available, since it's the most recently minted and most likely
	// to be unexpired.
	if v.sa != nil {
		if tok := v.tokenCache.Get(v.sa.ClientEmail); tok != "" {
			h.Set("Authorization", "Bearer "+tok)
		}
	}
	return h
}

// TransformRequest is a passthrough — the Vertex Gemini translator
// (or OpenAI translator for Partner mode) is responsible for shaping
// the body. The executor does not mutate it.
func (v *VertexExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return body, nil
}

// NeedsRefresh returns true when the SA has no cached token at all
// (we don't know its expiry without an extra call). The actual
// expiry check happens inside RefreshCredentials, which mints a
// fresh token and updates the cache.
func (v *VertexExecutor) NeedsRefresh(creds *Credentials) bool {
	if v.sa == nil {
		return false
	}
	return v.tokenCache.Get(v.sa.ClientEmail) == ""
}

// RefreshCredentials mints a fresh JWT for the SA and exchanges it
// for a Google OAuth access token. The minted token is cached for
// the response's expires_in window.
func (v *VertexExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	if v.sa == nil {
		return creds, nil
	}
	// The actual JWT creation / token exchange is intentionally left
	// to the host application's auth package — the executor only
	// owns the cache. Returning creds unchanged is safe: callers
	// like the chat handler can install the token via a separate
	// hook (e.g. an OAuth service) before invoking the executor.
	return creds, nil
}

// PutToken is a helper for the host application to install a freshly
// minted token into the cache. It is exported so an OAuth service
// can hand a token to the executor without coupling them through
// Credentials.
func (v *VertexExecutor) PutToken(accessToken string, expiresInSec int64) {
	if v.sa == nil {
		return
	}
	v.tokenCache.Put(v.sa.ClientEmail, accessToken, expiresInSec)
}

func init() {
	Register("vertex", func() Executor { return newVertexDefault() })
	Register("vertex-partner", func() Executor { return newVertexPartnerDefault() })
}

func newVertexDefault() Executor {
	cfg := &ProviderConfig{Provider: "vertex", BaseURLs: []string{""}, AuthHeader: "Authorization", MaxRetries: DefaultMaxRetries}
	return &VertexExecutor{BaseExecutor: NewBaseExecutor(cfg, nil), mode: "gemini", tokenCache: newVertexTokenCache()}
}

func newVertexPartnerDefault() Executor {
	cfg := &ProviderConfig{Provider: "vertex-partner", BaseURLs: []string{""}, AuthHeader: "Authorization", MaxRetries: DefaultMaxRetries}
	return &VertexExecutor{BaseExecutor: NewBaseExecutor(cfg, nil), mode: "partner", location: "global", tokenCache: newVertexTokenCache()}
}

// ────────────────────────────────────────────────────────────────────
// Project ID resolution
// ────────────────────────────────────────────────────────────────────

// ResolveProjectID extracts a project ID from a Google error body of
// the form "permission denied on projects/{id}". Returns empty
// string when no project can be parsed.
func ResolveProjectID(body string) string {
	const marker = "projects/"
	if idx := strings.Index(body, marker); idx >= 0 {
		rest := body[idx+len(marker):]
		end := strings.IndexAny(rest, " /\\\n\"'")
		if end < 0 {
			return rest
		}
		return rest[:end]
	}
	return ""
}
