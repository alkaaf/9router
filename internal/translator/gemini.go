package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeminiTranslator converts between OpenAI Chat Completions and
// Google Gemini (generateContent) formats. It is bidirectional:
// TranslateRequest accepts an OpenAI-shaped body and emits a
// Gemini-shaped body (contents + systemInstruction + tools), and
// TranslateResponse accepts a Gemini SSE chunk and emits an
// OpenAI-shaped SSE chunk.
type GeminiTranslator struct{}

// NewGeminiTranslator returns a fresh translator.
func NewGeminiTranslator() *GeminiTranslator { return &GeminiTranslator{} }

func (GeminiTranslator) Name() string { return "gemini" }

// NewState returns an empty state.
func (GeminiTranslator) NewState() *State { return NewState() }

// ────────────────────────────────────────────────────────────────────
// Gemini wire types
// ────────────────────────────────────────────────────────────────────

// geminiBody is the request body shape we emit to Gemini.
type geminiBody struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Tools             []geminiToolsEntry      `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

// geminiContent is a single message in the contents array. It can
// also be used for the systemInstruction field.
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart is one element of a content.parts array. It can be a
// text part, inline_data (image), function_call, or function_response.
type geminiPart struct {
	Text            string                  `json:"text,omitempty"`
	InlineData      *geminiInlineData       `json:"inline_data,omitempty"`
	FunctionCall    *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

// geminiInlineData is the Gemini form of an embedded image. It maps
// to OpenAI's image_url content part.
type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// geminiFunctionCall is a model-emitted tool invocation.
type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// geminiFunctionResponse is a tool result mapped from OpenAI's
// {role: "tool"} message.
type geminiFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

// geminiToolsEntry wraps function declarations. Gemini requires a
// single object containing an array of {name, description, parameters}.
type geminiToolsEntry struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// geminiGenerationConfig is a small subset of generation config that
// the OpenAI client commonly sets.
type geminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

// geminiStreamChunk is the upstream SSE payload from Gemini.
type geminiStreamChunk struct {
	ModelVersion string             `json:"modelVersion,omitempty"`
	Candidates   []geminiCandidate  `json:"candidates,omitempty"`
	UsageMetadata *geminiUsageMeta  `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Index        int                `json:"index,omitempty"`
	Content      *geminiContent     `json:"content,omitempty"`
	FinishReason string             `json:"finishReason,omitempty"`
	SafetyRatings []geminiSafetyRating `json:"safetyRatings,omitempty"`
}

// geminiUsageMeta is the Gemini token accounting block.
type geminiUsageMeta struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// geminiSafetyRating is informational only — it is used to map
// blocked content to a non-200 finish reason.
type geminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// ────────────────────────────────────────────────────────────────────
// OpenAI → Gemini request translation
// ────────────────────────────────────────────────────────────────────

// TranslateRequest converts an OpenAI request into a Gemini request.
// The output preserves the model name, splits a "system" message
// into systemInstruction, and maps each subsequent message to a
// Gemini content entry with the appropriate part shape.
func (GeminiTranslator) TranslateRequest(in []byte) ([]byte, error) {
	var req openAIBody
	if err := json.Unmarshal(in, &req); err != nil {
		return nil, fmt.Errorf("gemini: parse openai body: %w", err)
	}
	out := geminiBody{
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens: 4096,
		},
	}
	if req.MaxTokens != nil {
		out.GenerationConfig.MaxOutputTokens = *req.MaxTokens
	}
	if tools := translateToolsOpenAIToGemini(req.Tools); len(tools) > 0 {
		out.Tools = tools
	}
	for _, m := range req.Messages {
		if m.Role == "system" {
			text := extractTextContent(m.Content)
			if text != "" {
				out.SystemInstruction = &geminiContent{
					Parts: []geminiPart{{Text: text}},
				}
			}
			continue
		}
		out.Contents = append(out.Contents, translateMessageOpenAIToGemini(m))
	}
	if len(out.Contents) == 0 {
		// Gemini rejects empty contents — fall back to a single
		// user turn with an empty text part so the upstream call
		// still goes through.
		out.Contents = []geminiContent{{Role: "user", Parts: []geminiPart{{Text: ""}}}}
	}
	return json.Marshal(out)
}

// ────────────────────────────────────────────────────────────────────
// Gemini → OpenAI response translation
// ────────────────────────────────────────────────────────────────────

// TranslateResponse converts a single Gemini SSE chunk into an
// OpenAI-shaped SSE chunk. State carries streaming accumulators
// (model name once, content across text deltas, tool-call deltas).
func (GeminiTranslator) TranslateResponse(state *State, chunk []byte) ([]byte, error) {
	if state == nil {
		state = NewState()
	}
	if len(chunk) == 0 {
		return nil, nil
	}
	var ev geminiStreamChunk
	if err := json.Unmarshal(chunk, &ev); err != nil {
		return nil, fmt.Errorf("gemini: parse chunk: %w", err)
	}
	if ev.ModelVersion != "" && state.Model == "" {
		state.Model = ev.ModelVersion
	}
	out := openAIChunk{Object: "chat.completion.chunk"}
	if state.Model != "" {
		out.Model = state.Model
	}
	// Lazily emit the role delta once per stream.
	if state.Role == "" {
		out.Choices = []openAIChoice{{Index: 0}}
		out.Choices[0].Delta.Role = "assistant"
		state.Role = "assistant"
	}

	if len(ev.Candidates) > 0 {
		cand := ev.Candidates[0]
		if cand.Content != nil {
			choice := openAIChoice{Index: cand.Index}
			for _, p := range cand.Content.Parts {
				switch {
				case p.Text != "":
					choice.Delta.Content += p.Text
					state.Content += p.Text
				case p.FunctionCall != nil:
					tc := openAIToolCallChunk{Index: 0, Type: "function"}
					tc.Function.Name = p.FunctionCall.Name
					if len(p.FunctionCall.Args) > 0 {
						tc.Function.Arguments = string(p.FunctionCall.Args)
					}
					choice.Delta.ToolCalls = []openAIToolCallChunk{tc}
					state.ToolCalls[0] = &ToolCallAccum{
						ID:   "call_" + p.FunctionCall.Name,
						Name: p.FunctionCall.Name,
						Type: "function",
						Arguments: string(p.FunctionCall.Args),
					}
				}
			}
			out.Choices = append(out.Choices, choice)
		}
		if cand.FinishReason != "" {
			reason := mapGeminiFinishReason(cand.FinishReason)
			state.FinishReason = reason
			fin := reason
			out.Choices = append(out.Choices, openAIChoice{
				Index:       cand.Index,
				FinishReason: &fin,
			})
		}
	}
	if ev.UsageMetadata != nil {
		out.Usage = &openAIUsage{
			PromptTokens:     ev.UsageMetadata.PromptTokenCount,
			CompletionTokens: ev.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      ev.UsageMetadata.TotalTokenCount,
		}
	}

	if len(out.Choices) == 0 && out.Usage == nil {
		return nil, nil
	}
	return json.Marshal(out)
}

// ────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────

// translateMessageOpenAIToGemini converts a single OpenAI message to
// a Gemini content entry. Gemini's roles are user / model (not
// assistant); tool results live in functionResponse parts.
func translateMessageOpenAIToGemini(m openAIMessage) geminiContent {
	role := m.Role
	if role == "assistant" {
		role = "model"
	}
	parts := []geminiPart{}

	switch role {
	case "tool":
		// OpenAI tool result → Gemini functionResponse.
		text := extractTextContent(m.Content)
		resp := json.RawMessage(`{}`)
		if text != "" {
			resp = json.RawMessage(mustJSONString(text))
		}
		parts = append(parts, geminiPart{
			FunctionResponse: &geminiFunctionResponse{
				Name:     m.Name,
				Response: resp,
			},
		})
		return geminiContent{Role: "user", Parts: parts}
	case "model":
		// Assistant with tool calls → text + functionCall parts.
		if text := extractTextContent(m.Content); text != "" {
			parts = append(parts, geminiPart{Text: text})
		}
		for i, tc := range m.ToolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			if !json.Valid(args) {
				args = json.RawMessage(`{}`)
			}
			_ = i
			parts = append(parts, geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}
		return geminiContent{Role: "model", Parts: parts}
	default:
		// user / system fallback.
		parts = append(parts, parseUserContent(m.Content)...)
		return geminiContent{Role: "user", Parts: parts}
	}
}

// parseUserContent splits an OpenAI content payload into Gemini
// parts. A string content is a single text part. An array of
// {type, text|image_url} blocks is mapped to text / inline_data.
func parseUserContent(raw json.RawMessage) []geminiPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []geminiPart{{Text: s}}
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return []geminiPart{{Text: extractTextContent(raw)}}
	}
	out := make([]geminiPart, 0, len(blocks))
	for _, blk := range blocks {
		t, _ := blk["type"].(string)
		switch t {
		case "text":
			if txt, ok := blk["text"].(string); ok {
				out = append(out, geminiPart{Text: txt})
			}
		case "image_url":
			iu, _ := blk["image_url"].(map[string]any)
			url, _ := iu["url"].(string)
			mime, data := splitDataURL(url)
			out = append(out, geminiPart{
				InlineData: &geminiInlineData{MimeType: mime, Data: data},
			})
		}
	}
	return out
}

// splitDataURL extracts the mime type and base64 payload from a
// data: URL like "data:image/png;base64,iVBORw0KGgo...". Non-data
// URLs return ("", url) so the caller can fall back to a text part
// if needed.
func splitDataURL(s string) (mime, data string) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return "", s
	}
	rest := s[len(prefix):]
	idx := strings.IndexByte(rest, ',')
	if idx < 0 {
		return rest, ""
	}
	header := rest[:idx]
	payload := rest[idx+1:]
	mime = header
	if semi := strings.IndexByte(header, ';'); semi >= 0 {
		mime = header[:semi]
	}
	return mime, payload
}

// translateToolsOpenAIToGemini maps OpenAI tool definitions into a
// single Gemini tools entry with functionDeclarations.
func translateToolsOpenAIToGemini(tools []openAITool) []geminiToolsEntry {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]geminiFunctionDecl, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		decls = append(decls, geminiFunctionDecl{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}
	if len(decls) == 0 {
		return nil
	}
	return []geminiToolsEntry{{FunctionDeclarations: decls}}
}

// mapGeminiFinishReason converts a Gemini finishReason into the
// OpenAI set. Gemini uses STOP, MAX_TOKENS, SAFETY, RECITATION,
// OTHER; OpenAI uses stop, length, content_filter, tool_calls, null.
func mapGeminiFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "content_filter"
	default:
		return reason
	}
}

// mustJSONString marshals a string into a JSON string literal.
func mustJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
