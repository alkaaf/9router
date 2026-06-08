package chatcore

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWriteSSEStream_HeadersSet — AC-001: SSE headers are written
// before the callback runs.
func TestWriteSSEStream_HeadersSet(t *testing.T) {
	rr := httptest.NewRecorder()
	w := http.ResponseWriter(rr)
	WriteSSEStream(w, func(bw *bufio.Writer) {
		// headers should already be set when the callback runs.
		h := rr.Header()
		if h.Get("Content-Type") != "text/event-stream" {
			t.Errorf("Content-Type = %q", h.Get("Content-Type"))
		}
		if h.Get("Cache-Control") != "no-cache" {
			t.Errorf("Cache-Control = %q", h.Get("Cache-Control"))
		}
		if h.Get("Connection") != "keep-alive" {
			t.Errorf("Connection = %q", h.Get("Connection"))
		}
		if h.Get("X-Accel-Buffering") != "no" {
			t.Errorf("X-Accel-Buffering = %q", h.Get("X-Accel-Buffering"))
		}
		// Write a chunk and flush.
		bw.WriteString("data: hello\n\n")
		_ = bw.Flush()
	})

	body := rr.Body.String()
	if !strings.Contains(body, "data: hello") {
		t.Errorf("body missing data, got %q", body)
	}
}

// TestWriteSSEStream_Status200 — the first write triggers 200 OK.
func TestWriteSSEStream_Status200(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteSSEStream(http.ResponseWriter(rr), func(bw *bufio.Writer) {
		bw.WriteString("data: x\n\n")
	})
	if rr.Code != http.StatusOK {
		t.Errorf("Code = %d, want 200", rr.Code)
	}
}

// TestWriteSSEStream_RealServer — AC-002: a real net/http server
// serves SSE chunks and `curl --no-buffer` (simulated by reading
// before close) receives the bytes.
func TestWriteSSEStream_RealServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteSSEStream(w, func(bw *bufio.Writer) {
			bw.WriteString("data: first\n\n")
			bw.Flush()
			bw.WriteString("data: second\n\n")
			bw.Flush()
			bw.WriteString("data: [DONE]\n\n")
			bw.Flush()
		})
	}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := string(body)
	for _, want := range []string{"data: first", "data: second", "data: [DONE]"} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing %q, got %q", want, got)
		}
	}
}

// TestWriteNonStreamJSON — AC-003: non-streaming JSON response
// uses the application/json content type and the supplied status.
func TestWriteNonStreamJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	if err := WriteNonStreamJSON(rr, http.StatusOK, map[string]any{"ok": true}); err != nil {
		t.Fatalf("WriteNonStreamJSON: %v", err)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("Code = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Errorf("body missing ok:true, got %q", rr.Body.String())
	}
}

// TestWriteSSEStream_BufferFlush — bytes are flushed to the
// underlying writer at end of callback.
func TestWriteSSEStream_BufferFlush(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteSSEStream(http.ResponseWriter(rr), func(bw *bufio.Writer) {
		// Write without explicit flush; WriteSSEStream flushes
		// after the callback returns.
		bw.WriteString("data: auto-flush\n\n")
	})
	if !strings.Contains(rr.Body.String(), "data: auto-flush") {
		t.Errorf("expected auto-flush to land in body, got %q", rr.Body.String())
	}
}

// TestWriteSSEStream_Timeout — sanity check that the headers
// arrive promptly (no blocking IO).
func TestWriteSSEStream_Timeout(t *testing.T) {
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		WriteSSEStream(http.ResponseWriter(rr), func(bw *bufio.Writer) {
			bw.WriteString("data: ok\n\n")
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WriteSSEStream blocked")
	}
}
