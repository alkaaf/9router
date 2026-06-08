// Package translator provides format converters between LLM wire
// formats. The chat handler runs a translator (Claude → OpenAI,
// OpenAI → Claude, Gemini → OpenAI, ...) before dispatching the
// request to the appropriate executor; the inverse translator runs
// on the streaming response so the client always sees the wire format
// they asked for.
package translator

// Direction identifies the two translation flows:
//
//	RequestIn   — translate the request the user sent into the
//	              executor's native format.
//	RequestOut  — translate an executor's native request into another
//	              format (rare, used by combo fallback).
//	ResponseIn  — translate an upstream SSE/NDJSON response into the
//	              client-facing format.
//	ResponseOut — translate the client-facing response into the
//	              executor's native format (rare).
type Direction int

const (
	DirectionRequestIn Direction = iota
	DirectionRequestOut
	DirectionResponseIn
	DirectionResponseOut
)

// Translator is the interface every format converter implements.
// Most translators only override the directions they care about —
// the un-overridden direction is a no-op (identity).
//
// TranslateRequest takes a JSON body and returns the body in the
// target format. TranslateResponse operates on a single chunk of an
// upstream response stream; the chunk's format depends on the
// upstream (OpenAI SSE chunk, Claude SSE event, Gemini JSON, etc.).
//
// TranslateRequest is pure-functional: given the same input, it
// always produces the same output, and it does not retain references
// to the input slice. TranslateResponse is stateful: the streaming
// state (accumulated tool-call deltas, last finish_reason, ...) is
// carried in the state pointer, which the chat handler allocates
// once per request and threads through every call.
type Translator interface {
	Name() string
	TranslateRequest(in []byte) (out []byte, err error)
	TranslateResponse(state *State, chunk []byte) (out []byte, err error)
	NewState() *State
}

// State is the streaming-state carrier for TranslateResponse. Each
// translator uses a subset of fields; the others stay zero.
//
//	Role         — accumulated assistant role from the first delta
//	Content      — accumulated text content
//	ToolCalls    — accumulated tool calls, keyed by their upstream index
//	FinishReason — last finish_reason seen (string, since some providers
//	              emit values like "tool_calls" that don't match Go
//	              enums)
//	Model        — first model string seen (some providers echo it
//	              only in the first chunk)
//	Thinking     — accumulated reasoning content (Claude thinking blocks)
type State struct {
	Role         string
	Content      string
	ToolCalls    map[int]*ToolCallAccum
	FinishReason string
	Model        string
	Thinking     string

	// extra is a per-translator escape hatch for state that doesn't
	// fit the common fields (e.g. Gemini candidates vs choices).
	extra map[string]any
}

// ToolCallAccum accumulates a single tool call's streamed fields.
type ToolCallAccum struct {
	ID        string
	Name      string
	Arguments string
	Type      string
}

// NewState returns a zero-valued state.
func NewState() *State {
	return &State{ToolCalls: make(map[int]*ToolCallAccum)}
}

// GetExtra returns a value stored in the per-translator extra map.
func (s *State) GetExtra(key string) (any, bool) {
	if s.extra == nil {
		return nil, false
	}
	v, ok := s.extra[key]
	return v, ok
}

// SetExtra stores a value in the per-translator extra map.
func (s *State) SetExtra(key string, val any) {
	if s.extra == nil {
		s.extra = make(map[string]any)
	}
	s.extra[key] = val
}

// Identity is a translator that returns its inputs unchanged. It is
// the default when the request format already matches the executor.
type Identity struct{}

func (Identity) Name() string                              { return "identity" }
func (Identity) TranslateRequest(in []byte) ([]byte, error) { return in, nil }
func (Identity) TranslateResponse(_ *State, in []byte) ([]byte, error) { return in, nil }
func (Identity) NewState() *State { return NewState() }
