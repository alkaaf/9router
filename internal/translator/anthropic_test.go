package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicTranslator_Name(t *testing.T) {
	if NewAnthropicTranslator().Name() != "anthropic" {
		t.Errorf("name = %q", NewAnthropicTranslator().Name())
	}
}

func TestAnthropicTranslator_OpenAIToClaude(t *testing.T) {
	tr := NewAnthropicTranslator()
	in := []byte(`{
		"model":"claude-3-opus",
		"messages":[
			{"role":"system","content":"You are a helper."},
			{"role":"user","content":"hi"}
		],
		"max_tokens":100
	}`)
	out, err := tr.TranslateRequest(in)
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg["system"] != "You are a helper." {
		t.Errorf("system not lifted: %v", msg["system"])
	}
}

func TestAnthropicTranslator_OpenAIToClaude_ToolUse(t *testing.T) {
	tr := NewAnthropicTranslator()
	in := []byte(`{
		"model":"claude-3",
		"messages":[
			{"role":"user","content":"hi"},
			{"role":"assistant","tool_calls":[
				{"id":"c1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}
			]},
			{"role":"tool","tool_call_id":"c1","content":"72F"}
		]
	}`)
	out, _ := tr.TranslateRequest(in)
	if !strings.Contains(string(out), "tool_use") {
		t.Errorf("expected tool_use: %s", out)
	}
	if !strings.Contains(string(out), "tool_result") {
		t.Errorf("expected tool_result: %s", out)
	}
}

func TestAnthropicTranslator_OpenAIToClaude_Tools(t *testing.T) {
	tr := NewAnthropicTranslator()
	in := []byte(`{
		"model":"claude-3",
		"messages":[{"role":"user","content":"x"}],
		"tools":[{"type":"function","function":{"name":"f","description":"d","parameters":{"type":"object"}}}]
	}`)
	out, _ := tr.TranslateRequest(in)
	if !strings.Contains(string(out), "input_schema") {
		t.Errorf("expected input_schema: %s", out)
	}
}

func TestAnthropicTranslator_ClaudeToOpenAI_Streaming(t *testing.T) {
	tr := NewAnthropicTranslator()
	state := tr.NewState()

	events := []string{
		`{"type":"message_start","message":{"model":"claude-3"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`,
		`{"type":"message_delta","stop_reason":"end_turn"}`,
		`{"type":"message_stop"}`,
	}
	var chunks []string
	for _, e := range events {
		out, err := tr.TranslateResponse(state, []byte(e))
		if err != nil {
			t.Fatalf("TranslateResponse: %v", err)
		}
		if out != nil {
			chunks = append(chunks, string(out))
		}
	}
	if state.Content != "hi there" {
		t.Errorf("Content = %q, want %q", state.Content, "hi there")
	}
	if state.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q, want end_turn", state.FinishReason)
	}
	if chunks[len(chunks)-1] != "[DONE]" {
		t.Errorf("last chunk = %q, want [DONE]", chunks[len(chunks)-1])
	}
}

func TestAnthropicTranslator_ThinkingBlock(t *testing.T) {
	tr := NewAnthropicTranslator()
	state := tr.NewState()
	ev := `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","text":"reasoning"}}`
	out, _ := tr.TranslateResponse(state, []byte(ev))
	if !strings.Contains(string(out), "reasoning_content") {
		t.Errorf("expected reasoning_content: %s", out)
	}
	if state.Thinking != "reasoning" {
		t.Errorf("Thinking = %q", state.Thinking)
	}
}

func TestAnthropicTranslator_ToolCallDelta(t *testing.T) {
	tr := NewAnthropicTranslator()
	state := tr.NewState()
	ev := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","text":"{\"a\":"}}`
	out, _ := tr.TranslateResponse(state, []byte(ev))
	if !strings.Contains(string(out), "tool_calls") {
		t.Errorf("expected tool_calls: %s", out)
	}
	if state.ToolCalls[0].Arguments != `{"a":` {
		t.Errorf("ToolCalls[0].Arguments = %q", state.ToolCalls[0].Arguments)
	}
}

func TestAnthropicTranslator_EmptyChunk(t *testing.T) {
	tr := NewAnthropicTranslator()
	if out, err := tr.TranslateResponse(NewState(), nil); err != nil || out != nil {
		t.Errorf("empty chunk should return nil, nil; got %v, %v", out, err)
	}
}
