package chatcore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// StreamingWriter abstracts the Fiber / http writer so the rest of
// chatcore can stay framework-agnostic. The default implementation,
// httpsetResponseStreamer, uses net/http's SetBodyStreamWriter
// equivalent (manual hijack + async write).
type StreamingWriter interface {
	// WriteStream sets the response headers (Content-Type etc.)
	// and invokes fn with a *bufio.Writer that the callback uses
	// to push bytes. The function MUST call Flush before
	// returning.
	WriteStream(w http.ResponseWriter, fn func(w *bufio.Writer))
}

// SetStreamHeaders sets the canonical SSE headers on a
// net/http response writer.
func SetStreamHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
}

// SetJSONHeaders sets the canonical JSON headers.
func SetJSONHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
}

// WriteSSEStream is the net/http equivalent of Fiber's
// SetBodyStreamWriter. It writes the headers, then invokes fn with a
// *bufio.Writer wrapped around the response writer. The callback is
// responsible for flushing before it returns.
//
// The first write to w triggers the implicit 200 OK status; if the
// caller wants to return a different status they must set it
// before calling.
func WriteSSEStream(w http.ResponseWriter, fn func(bw *bufio.Writer)) {
	SetStreamHeaders(w)
	bw := bufio.NewWriter(w)
	fn(bw)
	_ = bw.Flush()
}

// StreamChunkCallback is the (data → bytes) signature the streaming
// handler invokes repeatedly. Returning io.EOF signals end of
// stream. Any other error terminates the stream and is returned to
// the caller.
type StreamChunkCallback func() error

// ErrClientDisconnect is returned by AwaitClientDisconnect when
// the context fires before the supplied channel reports completion.
var ErrClientDisconnect = errors.New("client disconnected")

// WriteNonStreamJSON serialises v as a single JSON object.
func WriteNonStreamJSON(w http.ResponseWriter, status int, v any) error {
	SetJSONHeaders(w)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}
