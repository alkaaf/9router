package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// GithubCopilotBaseURL is the canonical Copilot API host.
const GithubCopilotBaseURL = "https://api.githubcopilot.com"

// GithubCopilotIntegrationID is the value sent in the
// copilot-integration-id header. Matches the Node.js open-sse value.
const GithubCopilotIntegrationID = "vscode-chat"

// GithubCopilotEditorVersion is the version string advertised in
// copilot-internal-editor-version-header.
const GithubCopilotEditorVersion = "vscode/1.95.0"

// GithubExecutor handles GitHub Copilot. Copilot has several quirks
// versus the standard OpenAI API:
//
//   - Authentication uses a Copilot-specific token (issued by the
//     GitHub OAuth flow, distinct from the GitHub access token).
//   - The chat-completions endpoint is /chat/completions; the
//     responses API endpoint is /responses. Codex-family models
//     (/gpt-5-codex/) prefer /responses.
//   - Some models (gpt-5+) require max_completion_tokens instead of
//     max_tokens; some (gpt-5.4) ignore temperature; some (Claude
//     family) accept a thinking field.
//   - Claude messages have tool content that must be stripped before
//     the request goes upstream.
type GithubExecutor struct {
	*BaseExecutor
	// tokenCache holds the Copilot-issued token (distinct from the
	// GitHub OAuth access token used to mint it).
	tokenCache sync.Map // map[githubAccessToken]copilotToken
}

type copilotToken struct {
	token     string
	expiresAt int64
}

// NewGithubExecutor returns a GithubExecutor.
func NewGithubExecutor() *GithubExecutor {
	cfg := &ProviderConfig{
		Provider:   "github",
		BaseURLs:   []string{GithubCopilotBaseURL + "/chat/completions"},
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &GithubExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// BuildUrl returns the Copilot chat-completions URL by default; the
// /responses endpoint is used for codex models. Since the choice is
// model-dependent, the chat handler can override via the request's
// Model field and call BuildUrl twice (once per candidate endpoint).
func (g *GithubExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	if g.config == nil || len(g.config.BaseURLs) == 0 {
		return "/chat/completions"
	}
	return g.config.BaseURLs[urlIndex]
}

// BuildHeaders adds the Copilot-specific integration headers.
func (g *GithubExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := g.BaseExecutor.BuildHeaders(req, creds)
	h.Set("copilot-integration-id", GithubCopilotIntegrationID)
	h.Set("Editor-Version", GithubCopilotEditorVersion)
	h.Set("copilot-internal-editor-version-header", GithubCopilotEditorVersion)
	if req.Headers != nil {
		for k, v := range req.Headers {
			h.Set(k, v)
		}
	}
	return h
}

// RequiresMaxCompletionTokens reports whether the model needs the
// max_completion_tokens field instead of max_tokens. As of late 2024
// this is the gpt-5 family.
func RequiresMaxCompletionTokens(model string) bool {
	return strings.HasPrefix(model, "gpt-5")
}

// SupportsTemperature reports whether the model accepts a top-level
// temperature parameter. The gpt-5.4 family doesn't — only the
// default 1.0 is accepted.
func SupportsTemperature(model string) bool {
	return !strings.HasPrefix(model, "gpt-5.4")
}

// SupportsThinking reports whether the model accepts a
// `thinking` config block (Claude family).
func SupportsThinking(model string) bool {
	return strings.HasPrefix(model, "claude-")
}

// RequiresResponsesEndpoint reports whether the model should be
// routed to /responses rather than /chat/completions.
func RequiresResponsesEndpoint(model string) bool {
	return strings.HasPrefix(model, "gpt-5-codex")
}

// TransformRequest applies model-specific body modifications:
//
//   - max_tokens → max_completion_tokens for gpt-5+
//   - drop temperature for gpt-5.4
//   - add thinking config for Claude models
//   - strip tool_use content from Claude messages
func (g *GithubExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return body, nil
	}

	// max_tokens → max_completion_tokens.
	if RequiresMaxCompletionTokens(model) {
		if v, ok := msg["max_tokens"]; ok {
			delete(msg, "max_tokens")
			msg["max_completion_tokens"] = v
		}
	}

	// Drop temperature for gpt-5.4.
	if !SupportsTemperature(model) {
		delete(msg, "temperature")
	}

	// Add thinking config for Claude.
	if SupportsThinking(model) {
		if _, ok := msg["thinking"]; !ok {
			msg["thinking"] = json.RawMessage(`{"type":"enabled","budget_tokens":1024}`)
		}
	}

	// Sanitize messages — strip non-text / image_url content from
	// Claude messages (Copilot's chat-completions endpoint can't
	// handle Anthropic-shaped tool_use content).
	if raw, ok := msg["messages"]; ok {
		msg["messages"] = sanitizeMessages(raw, SupportsThinking(model))
	}

	out, err := json.Marshal(msg)
	if err != nil {
		return body, nil
	}
	return out, nil
}

// sanitizeMessages walks the OpenAI-shaped messages array and
// strips any content block that is neither text nor image_url.
// For Claude messages, tool content is also dropped.
func sanitizeMessages(raw json.RawMessage, isClaude bool) json.RawMessage {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return raw
	}
	for i, m := range msgs {
		role, _ := m["role"].MarshalJSON()
		if isClaude || string(role) == `"assistant"` {
			// For Claude: drop tool_use / tool_result content from
			// assistant messages; replace tool_calls with a textual
			// placeholder so the upstream OpenAI chat-completions
			// endpoint accepts the message.
			delete(m, "tool_calls")
		}
		// For all messages: clean content array — keep only text and
		// image_url blocks.
		if content, ok := m["content"]; ok {
			m["content"] = cleanContent(content)
		}
		msgs[i] = m
	}
	out, err := json.Marshal(msgs)
	if err != nil {
		return raw
	}
	return out
}

func cleanContent(raw json.RawMessage) json.RawMessage {
	// Try as string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return raw
	}
	// Try as array of content blocks.
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return raw
	}
	kept := make([]map[string]json.RawMessage, 0, len(blocks))
	for _, b := range blocks {
		t, _ := b["type"].MarshalJSON()
		ts := string(t)
		if ts == `"text"` || ts == `"image_url"` {
			kept = append(kept, b)
		}
	}
	if len(kept) == 0 {
		return json.RawMessage(`""`)
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return raw
	}
	return out
}

// NeedsRefresh returns true when the supplied token is missing or
// within the standard 60-second skew window.
func (g *GithubExecutor) NeedsRefresh(creds *Credentials) bool {
	return g.BaseExecutor.NeedsRefresh(creds)
}

// RefreshCredentials is a no-op base. The actual GitHub → Copilot
// token exchange is performed by the host application's auth package.
func (g *GithubExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	return g.BaseExecutor.RefreshCredentials(ctx, creds)
}

func init() {
	Register("github", func() Executor { return NewGithubExecutor() })
}
