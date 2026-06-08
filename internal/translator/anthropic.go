package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AnthropicTranslator converts between OpenAI Chat Completions and
// Anthropic Messages formats.
type AnthropicTranslator struct{}

// NewAnthropicTranslator returns a fresh translator.
func NewAnthropicTranslator() *AnthropicTranslator { return &AnthropicTranslator{} }

func (AnthropicTranslator) Name() string { return "anthropic" }

// NewState returns an empty state.
func (AnthropicTranslator) NewState() *State { return NewState() }

// openAIBody is the minimal OpenAI request shape.
type openAIBody struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAITool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
	MaxTokens *int           `json:"max_tokens,omitempty"`
}

// openAIMessage is one message in an OpenAI request.
type openAIMessage struct {
	Role      string            `json:"role"`
	Content   json.RawMessage   `json:"content,omitempty"`
	Name      string            `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls []openAIToolCall  `json:"tool_calls,omitempty"`
}

// openAIToolCall is a single tool call in an assistant message.
type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAITool is a tool definition from the OpenAI request.
type openAITool struct {
	Type     string          `json:"type"`
	Function openAIToolFunc  `json:"function"`
}

type openAIToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// claudeBody is the Anthropic request shape we emit.
type claudeBody struct {
	Model     string           `json:"model"`
	Messages  []claudeMessage  `json:"messages"`
	System    string           `json:"system,omitempty"`
	MaxTokens int              `json:"max_tokens"`
	Tools     []claudeTool     `json:"tools,omitempty"`
	Stream    bool             `json:"stream,omitempty"`
}

// claudeMessage is one message in a Claude request.
type claudeMessage struct {
	Role    string              `json:"role"`
	Content []claudeContentBlock `json:"content"`
}

// claudeContentBlock is one of text, tool_use, or tool_result.
type claudeContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// claudeTool is a tool definition in the Claude format.
type claudeTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// openAIChunk is the OpenAI SSE chunk we emit from TranslateResponse.
type openAIChunk struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []openAIChoice  `json:"choices,omitempty"`
	Usage   *openAIUsage    `json:"usage,omitempty"`
}

type openAIChoice struct {
	Index int `json:"index"`
	Delta struct {
		Role             string             `json:"role,omitempty"`
		Content          string             `json:"content,omitempty"`
		ReasoningContent string             `json:"reasoning_content,omitempty"`
		ToolCalls        []openAIToolCallChunk `json:"tool_calls,omitempty"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// openAIToolCallChunk is a single tool-call delta in an OpenAI SSE chunk.
type openAIToolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// claudeEvent is one SSE event payload from the Claude API.
type claudeEvent struct {
	Type string `json:"type"`
	Message *struct{ Model string `json:"model"` } `json:"message,omitempty"`
	Index  int `json:"index,omitempty"`
	Delta  *claudeDelta `json:"delta,omitempty"`
	ContentBlock *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content_block,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

// claudeDelta is the delta payload in a content_block_delta event.
type claudeDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ────────────────────────────────────────────────────────────────────
// OpenAI → Claude request translation
// ────────────────────────────────────────────────────────────────────

// TranslateRequest converts an OpenAI request into a Claude request.
func (AnthropicTranslator) TranslateRequest(in []byte) ([]byte, error) {
	var req openAIBody
	if err := json.Unmarshal(in, &req); err != nil {
		return nil, fmt.Errorf("anthropic: parse openai body: %w", err)
	}
	out := claudeBody{
		Model:     req.Model,
		Stream:    req.Stream,
		MaxTokens: 4096,
		Tools:     translateToolsOpenAIToClaude(req.Tools),
	}
	if req.MaxTokens != nil {
		out.MaxTokens = *req.MaxTokens
	}

	for _, m := range req.Messages {
		if m.Role == "system" {
			out.System = extractTextContent(m.Content)
			continue
		}
		out.Messages = append(out.Messages, translateMessageOpenAIToClaude(m))
	}
	return json.Marshal(out)
}

// ────────────────────────────────────────────────────────────────────
// Claude → OpenAI response translation
// ────────────────────────────────────────────────────────────────────

// TranslateResponse converts a single Claude SSE event into an
// OpenAI-shaped SSE chunk. The state is mutated in place so tool-call
// deltas accumulate across events.
func (AnthropicTranslator) TranslateResponse(state *State, chunk []byte) ([]byte, error) {
	if state == nil {
		state = NewState()
	}
	if len(chunk) == 0 {
		return nil, nil
	}
	var ev claudeEvent
	if err := json.Unmarshal(chunk, &ev); err != nil {
		return nil, fmt.Errorf("anthropic: parse event: %w", err)
	}
	if ev.Message != nil && ev.Message.Model != "" && state.Model == "" {
		state.Model = ev.Message.Model
	}
	out := openAIChunk{Object: "chat.completion.chunk"}
	if state.Model != "" {
		out.Model = state.Model
	}

	switch ev.Type {
	case "message_start":
		out.Choices = []openAIChoice{{
			Index: 0,
			Delta: struct {
				Role             string             `json:"role,omitempty"`
				Content          string             `json:"content,omitempty"`
				ReasoningContent string             `json:"reasoning_content,omitempty"`
				ToolCalls        []openAIToolCallChunk `json:"tool_calls,omitempty"`
			}{Role: "assistant"},
		}}
	case "content_block_start":
		if ev.ContentBlock == nil {
			return nil, nil
		}
		if ev.ContentBlock.Type == "tool_use" {
			out.Choices = []openAIChoice{{
				Index: 0,
				Delta: struct {
					Role             string             `json:"role,omitempty"`
					Content          string             `json:"content,omitempty"`
					ReasoningContent string             `json:"reasoning_content,omitempty"`
					ToolCalls        []openAIToolCallChunk `json:"tool_calls,omitempty"`
				}{ToolCalls: []openAIToolCallChunk{{Index: ev.Index, ID: ev.ContentBlock.Text, Type: "function"}}},
			}}
		}
	case "content_block_delta":
		if ev.Delta == nil {
			return nil, nil
		}
		choice := openAIChoice{Index: 0}
		switch ev.Delta.Type {
		case "text_delta":
			choice.Delta.Content = ev.Delta.Text
			state.Content += ev.Delta.Text
		case "thinking_delta":
			choice.Delta.ReasoningContent = ev.Delta.Text
			state.Thinking += ev.Delta.Text
		case "input_json_delta":
			if _, ok := state.ToolCalls[ev.Index]; !ok {
				state.ToolCalls[ev.Index] = &ToolCallAccum{Type: "function"}
			}
			state.ToolCalls[ev.Index].Arguments += ev.Delta.Text
			choice.Delta.ToolCalls = []openAIToolCallChunk{{
				Index: ev.Index,
				Function: struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{Arguments: state.ToolCalls[ev.Index].Arguments},
			}}
		}
		out.Choices = []openAIChoice{choice}
	case "message_delta":
		if ev.Usage != nil {
			out.Usage = &openAIUsage{
				PromptTokens:     ev.Usage.InputTokens,
				CompletionTokens: ev.Usage.OutputTokens,
				TotalTokens:      ev.Usage.InputTokens + ev.Usage.OutputTokens,
			}
		}
		finish := ev.StopReason
		if finish != "" {
			state.FinishReason = finish
			out.Choices = []openAIChoice{{
				Index: 0,
				Delta: struct {
					Role             string             `json:"role,omitempty"`
					Content          string             `json:"content,omitempty"`
					ReasoningContent string             `json:"reasoning_content,omitempty"`
					ToolCalls        []openAIToolCallChunk `json:"tool_calls,omitempty"`
				}{},
				FinishReason: &finish,
			}}
		}
	case "message_stop":
		return []byte("[DONE]"), nil
	}

	if len(out.Choices) == 0 && out.Usage == nil {
		return nil, nil
	}
	return json.Marshal(out)
}

// ────────────────────────────────────────────────────────────────────
// Shared helpers
// ────────────────────────────────────────────────────────────────────

func translateMessageOpenAIToClaude(m openAIMessage) claudeMessage {
	cm := claudeMessage{Role: m.Role}
	if m.Role == "tool" {
		cm.Role = "user"
		cm.Content = []claudeContentBlock{{
			Type:      "tool_result",
			ToolUseID: m.ToolCallID,
			Content:   m.Content,
		}}
		return cm
	}
	if m.Role == "assistant" && len(m.ToolCalls) > 0 {
		if text := extractTextContent(m.Content); text != "" {
			cm.Content = append(cm.Content, claudeContentBlock{Type: "text", Text: text})
		}
		for _, tc := range m.ToolCalls {
			var input json.RawMessage
			if tc.Function.Arguments != "" {
				input = json.RawMessage(tc.Function.Arguments)
			}
			cm.Content = append(cm.Content, claudeContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
		return cm
	}
	cm.Content = []claudeContentBlock{{Type: "text", Text: extractTextContent(m.Content)}}
	return cm
}

func translateToolsOpenAIToClaude(tools []openAITool) []claudeTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]claudeTool, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		out = append(out, claudeTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return out
}

func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, blk := range blocks {
			if blk.Type == "text" {
				b.WriteString(blk.Text)
			}
		}
		return b.String()
	}
	return ""
}
