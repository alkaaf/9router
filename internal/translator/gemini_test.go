package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeminiTranslator_Name(t *testing.T) {
	if NewGeminiTranslator().Name() != "gemini" {
		t.Errorf("name = %q", NewGeminiTranslator().Name())
	}
}

// TestGeminiTranslator_OpenAIToGemini_TextPart covers the basic
// "Gemini text part" scenario: a user message becomes a content
// entry with a text part.
func TestGeminiTranslator_OpenAIToGemini_TextPart(t *testing.T) {
	tr := NewGeminiTranslator()
	in := []byte(`{
		"model":"gemini-1.5-pro",
		"messages":[{"role":"user","content":"hello"}]
	}`)
	out, err := tr.TranslateRequest(in)
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	var body geminiBody
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(body.Contents))
	}
	if body.Contents[0].Role != "user" {
		t.Errorf("role = %q, want user", body.Contents[0].Role)
	}
	if len(body.Contents[0].Parts) != 1 || body.Contents[0].Parts[0].Text != "hello" {
		t.Errorf("parts = %+v", body.Contents[0].Parts)
	}
}

// TestGeminiTranslator_OpenAIToGemini_ToolCalls covers the
// "OpenAI tool_calls" scenario: assistant tool_calls become
// functionCall parts on a model-role content.
func TestGeminiTranslator_OpenAIToGemini_ToolCalls(t *testing.T) {
	tr := NewGeminiTranslator()
	in := []byte(`{
		"model":"gemini-1.5-pro",
		"messages":[
			{"role":"user","content":"what's the weather?"},
			{"role":"assistant","tool_calls":[
				{"id":"c1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}
			]},
			{"role":"tool","tool_call_id":"c1","name":"get_weather","content":"72F"}
		]
	}`)
	out, _ := tr.TranslateRequest(in)
	var body geminiBody
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(body.Contents))
	}
	// Second entry: assistant tool_calls → model with functionCall.
	asst := body.Contents[1]
	if asst.Role != "model" {
		t.Errorf("asst role = %q, want model", asst.Role)
	}
	foundFC := false
	for _, p := range asst.Parts {
		if p.FunctionCall != nil && p.FunctionCall.Name == "get_weather" {
			foundFC = true
		}
	}
	if !foundFC {
		t.Errorf("expected functionCall part, got %+v", asst.Parts)
	}
	// Third entry: tool result → functionResponse.
	tool := body.Contents[2]
	if tool.Role != "user" {
		t.Errorf("tool role = %q, want user", tool.Role)
	}
	if len(tool.Parts) != 1 || tool.Parts[0].FunctionResponse == nil {
		t.Fatalf("expected functionResponse, got %+v", tool.Parts)
	}
	if tool.Parts[0].FunctionResponse.Name != "get_weather" {
		t.Errorf("function name = %q", tool.Parts[0].FunctionResponse.Name)
	}
}

// TestGeminiTranslator_OpenAIToGemini_ImageURL covers the
// "OpenAI image_url" scenario: a multimodal content block with
// image_url becomes an inline_data part.
func TestGeminiTranslator_OpenAIToGemini_ImageURL(t *testing.T) {
	tr := NewGeminiTranslator()
	in := []byte(`{
		"model":"gemini-1.5-pro",
		"messages":[{
			"role":"user",
			"content":[
				{"type":"text","text":"what is this?"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo"}}
			]
		}]
	}`)
	out, _ := tr.TranslateRequest(in)
	var body geminiBody
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(body.Contents))
	}
	parts := body.Contents[0].Parts
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Text != "what is this?" {
		t.Errorf("text part = %q", parts[0].Text)
	}
	if parts[1].InlineData == nil {
		t.Fatalf("expected inline_data part, got %+v", parts[1])
	}
	if parts[1].InlineData.MimeType != "image/png" {
		t.Errorf("mime = %q, want image/png", parts[1].InlineData.MimeType)
	}
	if parts[1].InlineData.Data != "iVBORw0KGgo" {
		t.Errorf("data = %q", parts[1].InlineData.Data)
	}
}

// TestGeminiTranslator_OpenAIToGemini_Tools covers tool definitions
// being translated into Gemini's functionDeclarations.
func TestGeminiTranslator_OpenAIToGemini_Tools(t *testing.T) {
	tr := NewGeminiTranslator()
	in := []byte(`{
		"model":"gemini-1.5-pro",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[
			{"type":"function","function":{"name":"f","description":"d","parameters":{"type":"object"}}}
		]
	}`)
	out, _ := tr.TranslateRequest(in)
	if !strings.Contains(string(out), "functionDeclarations") {
		t.Errorf("expected functionDeclarations: %s", out)
	}
}

// TestGeminiTranslator_OpenAIToGemini_RoleMapping covers the
// "Role mapping" scenario: the system message is lifted into
// systemInstruction, and an assistant role is mapped to "model".
func TestGeminiTranslator_OpenAIToGemini_RoleMapping(t *testing.T) {
	tr := NewGeminiTranslator()
	in := []byte(`{
		"model":"gemini-1.5-pro",
		"messages":[
			{"role":"system","content":"you are a helper"},
			{"role":"assistant","content":"hi"}
		]
	}`)
	out, _ := tr.TranslateRequest(in)
	var body geminiBody
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.SystemInstruction == nil {
		t.Fatalf("expected systemInstruction, got nil")
	}
	if body.SystemInstruction.Parts[0].Text != "you are a helper" {
		t.Errorf("system text = %q", body.SystemInstruction.Parts[0].Text)
	}
	if len(body.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(body.Contents))
	}
	if body.Contents[0].Role != "model" {
		t.Errorf("asst role = %q, want model", body.Contents[0].Role)
	}
}

// TestGeminiTranslator_GeminiToOpenAI_Streaming covers the
// "Gemini streaming" scenario: a sequence of Gemini SSE chunks is
// translated to OpenAI SSE chunks, with text deltas accumulating
// and a final finish_reason emitted.
func TestGeminiTranslator_GeminiToOpenAI_Streaming(t *testing.T) {
	tr := NewGeminiTranslator()
	state := tr.NewState()

	chunks := []string{
		`{"modelVersion":"gemini-1.5-pro","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hi"}]}}]}`,
		`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":" there"}]}}]}`,
		`{"candidates":[{"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}`,
	}
	var emitted []string
	for _, c := range chunks {
		out, err := tr.TranslateResponse(state, []byte(c))
		if err != nil {
			t.Fatalf("TranslateResponse: %v", err)
		}
		if out != nil {
			emitted = append(emitted, string(out))
		}
	}
	if state.Content != "hi there" {
		t.Errorf("Content = %q, want %q", state.Content, "hi there")
	}
	if state.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", state.FinishReason)
	}
	if state.Model != "gemini-1.5-pro" {
		t.Errorf("Model = %q, want gemini-1.5-pro", state.Model)
	}
	// The first emitted chunk should have role delta.
	if !strings.Contains(emitted[0], `"role":"assistant"`) {
		t.Errorf("first chunk missing role: %s", emitted[0])
	}
	// The last chunk should have a finishReason.
	if !strings.Contains(emitted[len(emitted)-1], `"finish_reason":"stop"`) {
		t.Errorf("last chunk missing finish_reason: %s", emitted[len(emitted)-1])
	}
	// Usage should be present on the last chunk.
	if !strings.Contains(emitted[len(emitted)-1], `"total_tokens":7`) {
		t.Errorf("last chunk missing usage: %s", emitted[len(emitted)-1])
	}
}

// TestGeminiTranslator_GeminiToOpenAI_FunctionCall covers the
// "Gemini function_call" scenario: a model-emitted functionCall
// becomes an OpenAI tool_calls delta.
func TestGeminiTranslator_GeminiToOpenAI_FunctionCall(t *testing.T) {
	tr := NewGeminiTranslator()
	state := tr.NewState()

	chunk := `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"SF"}}}]}}]}`
	out, err := tr.TranslateResponse(state, []byte(chunk))
	if err != nil {
		t.Fatalf("TranslateResponse: %v", err)
	}
	if !strings.Contains(string(out), `"tool_calls"`) {
		t.Errorf("expected tool_calls: %s", out)
	}
	if !strings.Contains(string(out), `"get_weather"`) {
		t.Errorf("expected function name: %s", out)
	}
}

// TestGeminiTranslator_MapFinishReason checks the safety/length
// finishReason mapping.
func TestGeminiTranslator_MapFinishReason(t *testing.T) {
	cases := map[string]string{
		"STOP":              "stop",
		"MAX_TOKENS":        "length",
		"SAFETY":            "content_filter",
		"RECITATION":        "content_filter",
		"PROHIBITED_CONTENT": "content_filter",
		"OTHER":             "OTHER",
	}
	for in, want := range cases {
		if got := mapGeminiFinishReason(in); got != want {
			t.Errorf("mapGeminiFinishReason(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestGeminiTranslator_SplitDataURL exercises the data: URL parser.
func TestGeminiTranslator_SplitDataURL(t *testing.T) {
	mime, data := splitDataURL("data:image/jpeg;base64,abc")
	if mime != "image/jpeg" || data != "abc" {
		t.Errorf("splitDataURL(data URL) = (%q, %q)", mime, data)
	}
	mime, data = splitDataURL("https://example.com/x.png")
	if mime != "" || data != "https://example.com/x.png" {
		t.Errorf("splitDataURL(http URL) = (%q, %q)", mime, data)
	}
}
