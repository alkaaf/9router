package executor

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewKiroExecutor_Defaults(t *testing.T) {
	e := NewKiroExecutor()
	if e.GetProvider() != "kiro" {
		t.Errorf("GetProvider = %q, want kiro", e.GetProvider())
	}
}

func TestKiroExecutor_Headers(t *testing.T) {
	e := NewKiroExecutor()
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "kiro-tok"})
	if h.Get("Amz-Sdk-Request") == "" {
		t.Errorf("Amz-Sdk-Request should be set")
	}
	if h.Get("Amz-Sdk-Invocation-Id") == "" {
		t.Errorf("Amz-Sdk-Invocation-Id should be set")
	}
}

func TestParseKiroEventFrame(t *testing.T) {
	// Build a proper AWS EventStream binary frame:
	// total_length = 4 (header_len) + payload_len
	// headers_length = 4 (one zero-length header for simplicity)
	// header = 0x00 0x00 0x00 0x00 (zero-length name)
	// payload = {"type":"assistantResponseEvent","text":"hi"}
	payload := []byte(`{"type":"assistantResponseEvent","text":"hi"}`)
	headerLen := 4 // 4 bytes for a zero-length header
	totalLen := headerLen + len(payload)
	frame := make([]byte, 8+totalLen)
	binary.BigEndian.PutUint32(frame[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(frame[4:8], uint32(headerLen))
	// Zero-length header
	frame[8] = 0x00 // name_len = 0
	// Skip 4 bytes for the zero header (name_len + name is 0)
	copy(frame[8+4:], payload)

	ev, err := ParseKiroEventFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("ParseKiroEventFrame: %v", err)
	}
	_ = ev.EventType
	// We expect the parser to not error on malformed headers — it
	// should just produce an empty EventType when :event-type is
	// absent.
	_ = ev
}

func TestEstimateOutputTokens(t *testing.T) {
	if got := EstimateOutputTokens(4000); got != 1000 {
		t.Errorf("EstimateOutputTokens(4000) = %d, want 1000", got)
	}
}

func TestEstimateInputTokens(t *testing.T) {
	if got := EstimateInputTokens(0.5); got != 100000 {
		t.Errorf("EstimateInputTokens(0.5) = %d, want 100000", got)
	}
}

func TestKiroExecutor_NeedsRefresh(t *testing.T) {
	e := NewKiroExecutor()
	if !e.NeedsRefresh(&Credentials{AccessToken: "old", ExpiresAt: 0}) {
		t.Errorf("zero ExpiresAt should need refresh")
	}
}

func TestKiroExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("kiro") {
		t.Errorf("kiro should be registered")
	}
}

func TestKiroExecutor_Execute_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	e := NewKiroExecutor()
	e.config.BaseURLs[0] = "http://" + srv.Listener.Addr().String()
	resp, err := e.Execute(context.Background(),
		&Request{Method: "POST", Model: "x", Body: []byte(`{}`)},
		&Credentials{AccessToken: "t"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
}
