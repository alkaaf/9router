// Package chatcore implements the core request parsing, validation,
// and routing logic for the /v1/chat/completions endpoint.
//
// It is intentionally decoupled from the HTTP framework so that the
// same parser can be exercised in tests, by an internal CLI tool, or
// by alternative transports (e.g. the MITM proxy that funnels AI IDE
// traffic through Go).
//
// The package is the Go successor to src/sse/handlers/chat.js — it
// produces the same byte-for-byte error responses and accepts the
// same request shape (OpenAI chat completions, with model, messages,
// stream, tools, reasoning_effort, etc.).
package chatcore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxChatBodyBytes caps the size of a single chat request body. It
// matches the 10 MB ceiling applied to the OAuth handler so a single
// bad client cannot exhaust memory. Realistic chat payloads are well
// under 1 MB; the headroom is for tool definitions and large system
// prompts.
const maxChatBodyBytes = 10 << 20

// ChatRequest is the parsed, validated request body for
// /v1/chat/completions. The Body field carries the raw decoded JSON
// (a map[string]any) so downstream stages — model resolution, combo
// expansion, translator selection — can read fields without a second
// decode pass.
type ChatRequest struct {
	Body map[string]any
}

// ErrInvalidJSON is returned when the request body cannot be parsed
// as a JSON object. The HTTP layer translates it into a 400 response
// with the canonical OpenAI error envelope.
var ErrInvalidJSON = errors.New("invalid JSON body")

// ErrEmptyBody is returned when the body is empty or whitespace-only.
// The Node.js implementation treats empty and malformed bodies the
// same way (400 with "Invalid JSON body"), but separating the cause
// here keeps the log message accurate for operators.
var ErrEmptyBody = errors.New("empty request body")

// ParseRequest reads and decodes the chat request body. It enforces:
//
//   - Content-Length / MaxBytesReader to prevent memory blow-up.
//   - The body must be a single JSON *object* (not an array, string,
//     or scalar). This matches Node.js behaviour where a top-level
//     array would not match the { model, messages, ... } shape and
//     would fail downstream field access.
//
// The function returns ErrInvalidJSON or ErrEmptyBody for the
// documented rejection modes; any other error is treated as malformed
// JSON as well. The caller is expected to translate the error to a
// 400 response.
func ParseRequest(r *http.Request) (*ChatRequest, error) {
	// Step 1 — read the body, capped at maxChatBodyBytes. The
	// MaxBytesReader replaces the Body with an io.ReadCloser that
	// returns an error once the cap is exceeded, so a misbehaving
	// client cannot pin us at 100% memory forever.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxChatBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if len(body) > maxChatBodyBytes {
		return nil, fmt.Errorf("%w: body exceeds %d bytes", ErrInvalidJSON, maxChatBodyBytes)
	}

	// Step 2 — empty body (after trimming whitespace) is a 400.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, ErrEmptyBody
	}

	// Step 3 — decode into a generic map first so we can detect
	// top-level arrays/scalars before any field-level validation.
	var raw any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // preserve numeric precision for token counts, etc.
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	// Step 4 — the body must be a JSON object. The Node.js code
	// relies on `body.model` returning undefined for non-objects,
	// which is later caught as "Missing model". We match that
	// shape by rejecting non-objects up front, but with the same
	// error message ("Invalid JSON body") so existing client
	// code that pattern-matches on the message keeps working.
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, ErrInvalidJSON
	}

	return &ChatRequest{Body: m}, nil
}
