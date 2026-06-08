package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PerplexityBaseURL is the Perplexity web SSE endpoint.
const PerplexityBaseURL = "https://www.perplexity.ai/rest/thread/run"

// PerplexityModelMap maps the model id the user requests to
// Perplexity's internal model identifier.
var PerplexityModelMap = map[string]string{
	"pplx-sonar":   "experimental",
	"pplx-online":  "turbo",
	"pplx-pro":     "pplx_pro",
	"pplx-r1-1776": "r1",
	"pplx-sonar-r": "sonar",
	"pplx-deep":    "p3",
}

// PerplexityThinkingMap lists models that emit a thinking-mode
// "search_query" delta — these need to be mapped to OpenAI's
// reasoning_content field.
var PerplexityThinkingMap = map[string]bool{
	"pplx-r1-1776": true,
	"pplx-sonar-r": true,
}

// PerplexityWebExecutor talks to Perplexity's web SSE API. The wire
// format is bespoke — the server emits an "answer" event whose
// payload contains a `blocks` array rather than the standard
// `delta.content` shape. The executor reads the response and the
// translator (downstream in the chat handler) decodes the blocks.
type PerplexityWebExecutor struct {
	*BaseExecutor

	sessionCache *perplexitySessionCache
}

// perplexitySessionCache holds Perplexity session cookies with a 1-hour
// TTL. Sessions older than the TTL are evicted on access; a background
// ticker prunes expired entries.
type perplexitySessionCache struct {
	mu      sync.RWMutex
	sessions map[string]perplexitySession
	max     int
}

type perplexitySession struct {
	sessionToken string
	createdAt    time.Time
}

const (
	perplexitySessionTTL  = time.Hour
	perplexityMaxSessions = 200
)

func newPerplexitySessionCache() *perplexitySessionCache {
	c := &perplexitySessionCache{
		sessions: make(map[string]perplexitySession),
		max:      perplexityMaxSessions,
	}
	go c.gc()
	return c
}

// Get returns the session for the key, or empty string if missing or
// expired. Expired entries are removed on access.
func (c *perplexitySessionCache) Get(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.sessions[key]
	if !ok {
		return ""
	}
	if time.Since(s.createdAt) > perplexitySessionTTL {
		delete(c.sessions, key)
		return ""
	}
	return s.sessionToken
}

// Put stores a session, evicting the oldest entry if the cache is
// full. Eviction is FIFO-by-insertion (the oldest createdAt wins).
func (c *perplexitySessionCache) Put(key, sessionToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.sessions[key]; !exists && len(c.sessions) >= c.max {
		// Evict the oldest.
		var oldestKey string
		var oldestAt time.Time
		for k, s := range c.sessions {
			if oldestKey == "" || s.createdAt.Before(oldestAt) {
				oldestKey = k
				oldestAt = s.createdAt
			}
		}
		if oldestKey != "" {
			delete(c.sessions, oldestKey)
		}
	}
	c.sessions[key] = perplexitySession{
		sessionToken: sessionToken,
		createdAt:    time.Now(),
	}
}

// gc periodically prunes expired entries. Runs until the program exits.
func (c *perplexitySessionCache) gc() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		c.mu.Lock()
		for k, s := range c.sessions {
			if time.Since(s.createdAt) > perplexitySessionTTL {
				delete(c.sessions, k)
			}
		}
		c.mu.Unlock()
	}
}

// NewPerplexityWebExecutor returns a PerplexityWebExecutor.
func NewPerplexityWebExecutor() *PerplexityWebExecutor {
	cfg := &ProviderConfig{
		Provider:   "perplexity-web",
		BaseURLs:   []string{PerplexityBaseURL},
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &PerplexityWebExecutor{
		BaseExecutor: NewBaseExecutor(cfg, nil),
		sessionCache: newPerplexitySessionCache(),
	}
}

// BuildUrl returns the Perplexity endpoint.
func (p *PerplexityWebExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	if p.config == nil || len(p.config.BaseURLs) == 0 {
		return PerplexityBaseURL
	}
	return p.config.BaseURLs[urlIndex]
}

// BuildHeaders sets the Authorization Bearer or the session cookie
// depending on which credential the caller supplies. Perplexity uses
// Cookie: __Secure-next-auth.session-token=<value> for SSO and Bearer
// tokens for API-mode callers.
func (p *PerplexityWebExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "text/event-stream")
	if creds != nil && creds.AccessToken != "" {
		// Session token takes precedence over Bearer — the
		// "session-token" cookie is the field the API reads when
		// the Authorization header is absent.
		h.Set("Cookie", "__Secure-next-auth.session-token="+creds.AccessToken)
	}
	if req.Headers != nil {
		for k, v := range req.Headers {
			h.Set(k, v)
		}
	}
	return h
}

// TransformRequest is a passthrough — the upstream translator produces
// the Perplexity request body.
func (p *PerplexityWebExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return body, nil
}

// cleanPplxResponse strips XML declarations and citation markup that
// Perplexity sometimes leaks into the response blocks. The cleaning
// rules are conservative — they preserve the semantic content while
// removing wrapper noise.
func cleanPplxResponse(s string) string {
	// Remove XML declarations.
	if strings.HasPrefix(s, "<?xml") {
		if end := strings.Index(s, "?>"); end >= 0 {
			s = s[end+2:]
		}
	}
	// Remove "sup" tags (citation references).
	for {
		open := strings.Index(s, "<sup>")
		if open < 0 {
			break
		}
		close := strings.Index(s[open:], "</sup>")
		if close < 0 {
			break
		}
		s = s[:open] + s[open+close+len("</sup>"):]
	}
	// Replace doubled brackets.
	s = strings.ReplaceAll(s, "[[", "[")
	s = strings.ReplaceAll(s, "]]", "]")
	return s
}

// ReadPplxSseEvents reads Perplexity's bespoke SSE format from r and
// yields each event's data payload on the returned channel. Each
// event has the shape:
//
//	data: {...JSON...}\n\n
//
// The JSON contains a "blocks" array — the executor returns the raw
// data and the downstream translator decodes the blocks.
func ReadPplxSseEvents(ctx context.Context, r io.Reader) <-chan string {
	out := make(chan string, 8)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		var dataBuf strings.Builder
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return
			}
			line := scanner.Text()
			if strings.HasPrefix(line, ":") {
				continue
			}
			if line == "" {
				if dataBuf.Len() > 0 {
					out <- cleanPplxResponse(dataBuf.String())
					dataBuf.Reset()
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				val := strings.TrimPrefix(line, "data:")
				val = strings.TrimPrefix(val, " ")
				if dataBuf.Len() > 0 {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(val)
			}
		}
		if dataBuf.Len() > 0 {
			out <- cleanPplxResponse(dataBuf.String())
		}
	}()
	return out
}

// NeedsRefresh always returns false — Perplexity session tokens are
// long-lived (1 hour via the session cache) and the host application
// manages their lifecycle.
func (p *PerplexityWebExecutor) NeedsRefresh(creds *Credentials) bool {
	return false
}

// RefreshCredentials is a no-op.
func (p *PerplexityWebExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	return creds, nil
}

func init() {
	Register("perplexity-web", func() Executor { return NewPerplexityWebExecutor() })
}

// FormatPerplexityAnswer extracts the "answer" field from a Perplexity
// SSE event payload. Used by the chat handler / translator to lift
// the answer out of the block-shaped envelope.
func FormatPerplexityAnswer(payload string) (string, error) {
	var p struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return "", fmt.Errorf("perplexity answer: %w", err)
	}
	return p.Answer, nil
}
