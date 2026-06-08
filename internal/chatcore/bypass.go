package chatcore

import (
	"strings"
)

// BypassResult is returned by CheckBypass when a claude-cli bypass
// pattern matches. nil means no bypass.
type BypassResult struct {
	Bypass      bool
	NamingBypass bool
	Response     *Response
}

// SkipPatterns are substrings in the user message that trigger a
// bypass. The list mirrors open-sse's known skip markers; add new
// ones here when the Node.js table changes.
var SkipPatterns = []string{
	"skip",
	"ignore",
	"no need",
	"no need to",
	"do not respond",
	"don't respond",
}

// CheckBypass inspects the chat request body for claude-cli bypass
// patterns. If userAgent does not contain "claude-cli" (case-
// insensitive) the function returns nil immediately.
//
// The first matching pattern wins. CC naming is gated by
// ccFilterNaming — when that flag is false the pattern is skipped.
func CheckBypass(body map[string]any, userAgent string, ccFilterNaming bool) *BypassResult {
	if !strings.Contains(strings.ToLower(userAgent), "claude-cli") {
		return nil
	}
	messages, _ := body["messages"].([]any)
	if len(messages) == 0 {
		return nil
	}

	// Pattern 2 — Warmup: the first user message is exactly
	// "Warmup" (case-insensitive, trimmed).
	if isUserMessage(messages, 0, func(text string) bool {
		return strings.TrimSpace(strings.ToLower(text)) == "warmup"
	}) {
		return &BypassResult{
			Bypass:  true,
			Response: &Response{Status: 200, Body: []byte("{}"), Model: "bypass/warmup"},
		}
	}

	// Pattern 3 — Count: single user message whose text is
	// exactly "count" (case-insensitive).
	if len(messages) == 1 && isUserMessage(messages, 0, func(text string) bool {
		return strings.TrimSpace(strings.ToLower(text)) == "count"
	}) {
		return &BypassResult{
			Bypass:  true,
			Response: &Response{Status: 200, Body: []byte("{}"), Model: "bypass/count"},
		}
	}

	// Pattern 4 — Skip patterns: any user message contains a
	// substring from SkipPatterns.
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		if strings.ToLower(role) != "user" {
			continue
		}
		text, _ := m["content"].(string)
		lower := strings.ToLower(text)
		for _, pat := range SkipPatterns {
			if strings.Contains(lower, strings.ToLower(pat)) {
				return &BypassResult{
					Bypass:  true,
					Response: &Response{Status: 200, Body: []byte("{}"), Model: "bypass/skip"},
				}
			}
		}
	}

	// Pattern 5 — CC naming: system message contains "isNewTopic"
	// (case-insensitive). Only when ccFilterNaming is true.
	if ccFilterNaming && hasSystemMessage(messages, func(text string) bool {
		return strings.Contains(strings.ToLower(text), "isnewtopic")
	}) {
		return &BypassResult{
			Bypass:      true,
			NamingBypass: true,
			Response:     &Response{Status: 200, Body: []byte("{}"), Model: "bypass/naming"},
		}
	}

	// Pattern 1 — Title extraction: the last message is an
	// assistant message whose content starts with "{" (a JSON
	// serialised title block from a prior bypass).
	if n := len(messages); n > 0 {
		last := messages[n-1]
		m, ok := last.(map[string]any)
		if ok {
			role, _ := m["role"].(string)
			if strings.ToLower(role) == "assistant" {
				content, _ := m["content"].(string)
				trim := strings.TrimSpace(content)
				if strings.HasPrefix(trim, "{") {
					return &BypassResult{
						Bypass:  true,
						Response: &Response{Status: 200, Body: []byte(content), Model: "bypass/title"},
					}
				}
			}
		}
	}

	return nil
}

func isUserMessage(messages []any, idx int, pred func(string) bool) bool {
	if idx < 0 || idx >= len(messages) {
		return false
	}
	m, ok := messages[idx].(map[string]any)
	if !ok {
		return false
	}
	role, _ := m["role"].(string)
	if strings.ToLower(role) != "user" {
		return false
	}
	content, _ := m["content"].(string)
	return pred(content)
}

func hasSystemMessage(messages []any, pred func(string) bool) bool {
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		if strings.ToLower(role) != "system" {
			continue
		}
		content, _ := m["content"].(string)
		if pred(content) {
			return true
		}
	}
	return false
}
