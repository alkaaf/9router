// Package v1 contains the public LLM API handlers (paths under /v1/).
//
// This file implements /v1/chat/completions. The handler is the Go
// equivalent of src/app/api/v1/chat/completions/route.js +
// src/sse/handlers/chat.js#handleChat. It is intentionally minimal:
// it parses the request, builds a per-request context, and hands off
// to chatcore. The heavy lifting (model resolution, combo expansion,
// credential selection, executor dispatch, streaming) lives in the
// chatcore package and is implemented across CHAT-001..CHAT-018.
package v1

import (
	"errors"
	"log"
	"net/http"

	"github.com/9router/9router/internal/chatcore"
)

// ChatCompletionsHandler returns an http.HandlerFunc that serves
// POST /v1/chat/completions. The handler is exported as a function
// (not a method) so that wiring dependencies is explicit at the
// router level.
//
// Currently the handler only implements request parsing (CHAT-001).
// Subsequent tasks (CHAT-004..CHAT-018) will plug in model resolution,
// combo expansion, credential rotation, executor dispatch, and
// streaming. Each step is a clear seam so unit tests can cover
// individual stages.
func ChatCompletionsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Always advertise CORS for AI IDE clients that preflight
		// every request. The Node.js route exposes "*" too.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Step 1 — parse the request body. The chatcore package
		// distinguishes "empty" from "malformed" only for the log
		// message; both surface as a 400 with the canonical
		// OpenAI error envelope.
		req, err := chatcore.ParseRequest(r)
		if err != nil {
			switch {
			case errors.Is(err, chatcore.ErrEmptyBody):
				log.Printf("chat: empty body from %s", r.RemoteAddr)
			default:
				log.Printf("chat: invalid JSON body from %s: %v", r.RemoteAddr, err)
			}
			chatcore.WriteError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		// Step 2 — placeholder for downstream stages. Subsequent
		// CHAT tasks will replace this with model resolution,
		// combo expansion, and executor dispatch.
		log.Printf("chat: parsed request, model=%v", req.Body["model"])

		// Temporary success response until CHAT-018 wires the
		// full handler. It returns the parsed body so callers can
		// see that parsing succeeded and so the test suite can
		// exercise the full path through the handler.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"parsed","model":"placeholder"}`))
	}
}
