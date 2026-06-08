package rtk

import (
	"encoding/json"
	"strings"
)

// RtkCompressor implements context compression for long tool_result
// messages. The Node.js implementation ("RTK Caveman") detects the
// shape of tool-role messages and compresses them via configurable
// strategies before they reach the provider — reducing input-token
// costs on long agent call chains.
//
// The compressor is intentionally pluggable: the Strategy field is an
// interface that the host application can inject (or leave as
// StrategyNoop for "no compression" mode). The compressor itself
// owns the shape-detection and threshold logic.
type RtkCompressor struct {
	Strategy CompressionStrategy
	MinBytes int // skip messages below this byte length
	MaxBytes int // cap the compressed output at this many bytes
}

// CompressionStrategy compresses a tool-result payload. Return empty
// string to disable compression for a particular block. The strategy
// must be safe to call concurrently.
type CompressionStrategy interface {
	Compress(text string) string
}

// NoopCompression is the default strategy (passthrough). It exists so
// the compressor can be wired up before the actual algorithm lands.
var NoopCompression CompressionStrategy = noopCompression{}

type noopCompression struct{}

func (noopCompression) Compress(text string) string { return text }

// NewRtkCompressor returns a compressor with the supplied strategy and
// sensible defaults (MinBytes=1024, MaxBytes=2048).
func NewRtkCompressor(strategy CompressionStrategy) *RtkCompressor {
	if strategy == nil {
		strategy = NoopCompression
	}
	return &RtkCompressor{
		Strategy: strategy,
		MinBytes: 1024,
		MaxBytes: 2048,
	}
}

// CompressMessages examines a chat-completion request body and
// compresses eligible tool-result messages. It returns the (possibly
// modified) body plus a Stats struct describing what was changed.
func (c *RtkCompressor) CompressMessages(body []byte, enabled bool) ([]byte, Stats) {
	if !enabled || c == nil || c.Strategy == nil {
		return body, Stats{}
	}
	stats := Stats{BytesBefore: len(body)}
	out, hits := c.compress(body)
	stats.BytesAfter = len(out)
	stats.Hits = hits
	return out, stats
}

// Stats is the compression outcome for one CompressMessages call.
type Stats struct {
	BytesBefore int
	BytesAfter  int
	Hits        []StatHit
}

// StatHit describes one compressed block.
type StatHit struct {
	Shape string
	Filter string
	Saved int
}

func (c *RtkCompressor) compress(body []byte) ([]byte, []StatHit) {
	if c == nil {
		return body, nil
	}
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return body, nil
	}
	messagesRaw, ok := req["messages"]
	if !ok {
		return body, nil
	}
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(messagesRaw, &msgs); err != nil {
		return body, nil
	}
	var hits []StatHit
	for i, m := range msgs {
		if m["role"] == nil {
			continue
		}
		var role string
		if err := json.Unmarshal(m["role"], &role); err != nil || role != "tool" {
			continue
		}
		_, compressed, hit := c.compressMessage(m)
		if compressed != nil {
			hits = append(hits, hit)
			out, _ := json.Marshal(compressed)
			msgs[i] = map[string]json.RawMessage{
				"role":    m["role"],
				"content": out,
			}
		}
	}
	outMsgs, _ := json.Marshal(msgs)
	req["messages"] = outMsgs
	out, _ := json.Marshal(req)
	return out, hits
}

func (c *RtkCompressor) compressMessage(m map[string]json.RawMessage) (shape string, compressed map[string]json.RawMessage, hit StatHit) {
	content, ok := m["content"]
	if !ok {
		return "", nil, StatHit{}
	}
	shape, text := detectShape(content)
	if shape == "" || len(text) < c.MinBytes {
		return shape, nil, StatHit{}
	}
	filtered := safeApply(func(s string) string { return c.Strategy.Compress(s) }, text)
	if filtered == "" {
		filtered = text
	}
	saved := len(text) - len(filtered)
	if saved < 0 {
		saved = 0
	}
	compressed = map[string]json.RawMessage{
		"role":    m["role"],
		"content": json.RawMessage(filterJSONString(filtered)),
	}
	return shape, compressed, StatHit{Shape: shape, Filter: "default", Saved: saved}
}

// detectShape returns the detected message shape name and the raw
// text payload that would be compressed. Empty shape means "no tool
// content detected — skip".
func detectShape(raw json.RawMessage) (string, string) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return "openai-string", s
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", ""
	}
	// 1. OpenAI Responses: {"type":"function_call_output","output":...}
	for _, b := range blocks {
		t, _ := b["type"].MarshalJSON()
		if string(t) == `"function_call_output"` {
			if outRaw, ok := b["output"]; ok {
				var out any
				if err := json.Unmarshal(outRaw, &out); err == nil {
					if str, ok := out.(string); ok {
						return "openai-responses-string", str
					}
					// Array output — marshal back to string.
					out2, _ := json.Marshal(out)
					return "openai-responses-array", string(out2)
				}
			}
		}
	}
	// 2. Claude tool_result string: content[{type:"tool_result", content:"..."}]
	for _, b := range blocks {
		t, _ := b["type"].MarshalJSON()
		if string(t) == `"tool_result"` {
			// Single string content.
			if rawContent, ok := b["content"]; ok {
				var s string
				if err := json.Unmarshal(rawContent, &s); err == nil {
					return "claude-tool-result-string", s
				}
				// Array content: content[{type:"text", text:"..."}]
				var sub []map[string]any
				if err := json.Unmarshal(rawContent, &sub); err == nil {
					for _, subBlock := range sub {
						if subType, ok := subBlock["type"].(string); ok && subType == "text" {
							if text, ok := subBlock["text"].(string); ok {
								return "claude-tool-result-array", text
							}
						}
					}
				}
			}
		}
	}
	return "", ""
}

// safeApply runs fn with the given input and returns its result. If
// fn panics, safeApply returns the original input — the compressor
// must never crash the process because a filter is broken.
func safeApply(fn func(string) string, input string) string {
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()
	return fn(input)
}

// filterJSONString escapes a string for embedding inside a JSON
// string literal. It handles backslash and quote characters.
func filterJSONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// CompressMessages is a package-level convenience wrapper that uses
// a default RtkCompressor (NoopCompression). Host code can replace
// it by constructing its own *RtkCompressor and calling
// CompressMessages on it.
func CompressMessages(body []byte, enabled bool) ([]byte, Stats) {
	return CompressMessagesWith(body, enabled, NewRtkCompressor(NoopCompression))
}

// CompressMessagesWith is the full signature for callers that provide
// their own compressor. This is the entry point the chat handler uses
// when it has a configured strategy.
func CompressMessagesWith(body []byte, enabled bool, c *RtkCompressor) ([]byte, Stats) {
	if c == nil {
		return body, Stats{}
	}
	return c.CompressMessages(body, enabled)
}
