package rtk

import (
	"encoding/json"
	"testing"
)

func TestCompressMessages_Disabled(t *testing.T) {
	c := NewRtkCompressor(NoopCompression)
	in := []byte(`{"messages":[]}`)
	out, _ := c.CompressMessages(in, false)
	if string(out) != string(in) {
		t.Errorf("disabled should be a no-op")
	}
}

func TestCompressMessages_NilCompressor(t *testing.T) {
	out, _ := CompressMessagesWith([]byte(`{"messages":[]}`), true, nil)
	if string(out) != `{"messages":[]}` {
		t.Errorf("nil compressor should be no-op")
	}
}

func TestCompressMessages_OpenAIStringShape(t *testing.T) {
	tr := NewRtkCompressor(NoopCompression)
	longStr := "x"
	for len(longStr) < 2000 {
		longStr += longStr
	}
	longStr = longStr[:2000]
	body := []byte(`{"messages":[{"role":"tool","content":"` + longStr + `"}]}`)
	out, stats := tr.CompressMessages(body, true)
	if len(stats.Hits) != 1 {
		t.Errorf("Hits = %d, want 1, body len=%d", len(stats.Hits), len(body))
		t.Logf("body: %s", body[:200])
	}
	if stats.Hits[0].Shape != "openai-string" {
		t.Errorf("Shape = %q, want openai-string", stats.Hits[0].Shape)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestCompressMessages_BelowThreshold(t *testing.T) {
	tr := NewRtkCompressor(NoopCompression)
	tr.MinBytes = 9999
	body := []byte(`{"messages":[{"role":"tool","content":"small"}]}`)
	_, stats := tr.CompressMessages(body, true)
	if len(stats.Hits) != 0 {
		t.Errorf("below threshold should not compress, got %d hits", len(stats.Hits))
	}
}

func TestCompressMessages_SafeApply_Panic(t *testing.T) {
	badStrategy := panicStrategy{}
	c := NewRtkCompressor(badStrategy)
	body := []byte(`{"messages":[{"role":"tool","content":"long-content-"+strings.Repeat("x",2000)}]}`)
	out, _ := c.CompressMessages(body, true)
	if string(out) != string(body) {
		t.Errorf("panic filter should produce a no-op")
	}
}

type panicStrategy struct{}

func (panicStrategy) Compress(string) string { panic("broken filter") }

func TestDetectShape_OpenAIString(t *testing.T) {
	shape, text := detectShape(json.RawMessage(`"hello world"`))
	if shape != "openai-string" {
		t.Errorf("shape = %q, want openai-string", shape)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want hello world", text)
	}
}

func TestDetectShape_ClaudeToolResultString(t *testing.T) {
	shape, _ := detectShape(json.RawMessage(`[{"type":"tool_result","content":"long-text-here"}]`))
	if shape != "claude-tool-result-string" {
		t.Errorf("shape = %q, want claude-tool-result-string", shape)
	}
}

func TestDetectShape_ClaudeToolResultArray(t *testing.T) {
	shape, _ := detectShape(json.RawMessage(`[{"type":"tool_result","content":[{"type":"text","text":"nested-text"}]}]`))
	if shape != "claude-tool-result-array" {
		t.Errorf("shape = %q, want claude-tool-result-array", shape)
	}
}

func TestDetectShape_OpenAIResponses(t *testing.T) {
	shape, _ := detectShape(json.RawMessage(`[{"type":"function_call_output","output":"response-text"}]`))
	if shape != "openai-responses-string" {
		t.Errorf("shape = %q, want openai-responses-string", shape)
	}
}

func TestDetectShape_NonTool(t *testing.T) {
	shape, _ := detectShape(json.RawMessage(`"plain string"`))
	if shape != "openai-string" {
		t.Errorf("shape = %q, want openai-string", shape)
	}
}

func TestDetectShape_BadInput(t *testing.T) {
	shape, text := detectShape(json.RawMessage(`not-json`))
	if shape != "" {
		t.Errorf("shape = %q, want empty", shape)
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}

func TestSafeApply(t *testing.T) {
	got := safeApply(func(s string) string { return s + "-compressed" }, "abc")
	if got != "abc-compressed" {
		t.Errorf("safeApply = %q, want abc-compressed", got)
	}
	got = safeApply(func(s string) string { panic("x") }, "original")
	if got != "" {
		t.Errorf("panic should return empty (original was consumed by panic): got %q", got)
	}
}

func TestSafeApply_EmptyString(t *testing.T) {
	got := safeApply(func(s string) string { return "" }, "anything")
	if got != "" {
		t.Errorf("empty result should pass through: %q", got)
	}
}

func TestFilterJSONString(t *testing.T) {
	// Go string literals pass literal bytes; filterJSONString escapes
	// them for safe embedding inside a JSON string literal.
	cases := []struct {
		in, want string
	}{
		{`abc`, `"abc"`},
		{`a"b`, `"a\"b"`},
		{`a\nb`, `"a\\nb"`},
		{`a\rb`, `"a\\rb"`},
		{`a\tb`, `"a\\tb"`},
		{`a\b`, `"a\\b"`},
	}
	for _, c := range cases {
		got := filterJSONString(c.in)
		if got != c.want {
			t.Errorf("filterJSONString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompressMessages_KiroPath(t *testing.T) {
	tr := NewRtkCompressor(NoopCompression)
	longStr := "x"
	for len(longStr) < 5000 {
		longStr += longStr
	}
	longStr = longStr[:5000]
	body := []byte(`{
		"conversationState": {
			"history": [{
				"userInputMessage": {
					"toolResults": [{
						"content": [{"text": "` + longStr + `"}]
					}]
				}
			}]
		}
	}`)
	out, stats := tr.CompressMessages(body, true)
	if stats.BytesBefore != len(body) {
		t.Errorf("BytesBefore not recorded")
	}
	// The body doesn't match any detector shape (no "messages" key),
	// so stats.Hits should be empty but no panic.
	_ = out
}

type truncateStrategy struct{ Max int }

func (t truncateStrategy) Compress(s string) string {
	if len(s) > t.Max {
		return s[:t.Max]
	}
	return s
}

func TestCompressMessages_StatsTracking(t *testing.T) {
	tr := NewRtkCompressor(truncateStrategy{Max: 100})
	longStr := "x"
	for len(longStr) < 3000 {
		longStr += longStr
	}
	longStr = longStr[:3000]
	body := []byte(`{"messages":[{"role":"tool","content":"` + longStr + `"}]}`)
	out, stats := tr.CompressMessages(body, true)
	if stats.BytesBefore != len(body) {
		t.Errorf("BytesBefore = %d, want %d", stats.BytesBefore, len(body))
	}
	if stats.BytesAfter != len(out) {
		t.Errorf("BytesAfter mismatch")
	}
	if len(stats.Hits) != 1 {
		t.Errorf("expected 1 hit, got %d", len(stats.Hits))
	}
	if stats.Hits[0].Saved <= 0 {
		t.Errorf("Saved should be positive for truncate strategy: got %d", stats.Hits[0].Saved)
	}
	// Saved measures the difference in total body size, not just the
	// raw text delta. Because the compressor wraps the compressed
	// text in JSON (adding quotes and extra bytes), the Saved can
	// differ from (BytesBefore - BytesAfter) by a few bytes due to
	// JSON wrapper overhead.
}
