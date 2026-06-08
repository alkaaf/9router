package chatcore

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExtractAPIKey_BearerToken — AC-001: a "Bearer <key>" header
// yields the key.
func TestExtractAPIKey_BearerToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer sk-123")
	if got := ExtractAPIKey(r); got != "sk-123" {
		t.Errorf("got %q, want %q", got, "sk-123")
	}
}

// TestExtractAPIKey_XAPIKey — AC-002: a lone x-api-key header yields
// the key (Anthropic format).
func TestExtractAPIKey_XAPIKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("x-api-key", "sk-456")
	if got := ExtractAPIKey(r); got != "sk-456" {
		t.Errorf("got %q, want %q", got, "sk-456")
	}
}

// TestExtractAPIKey_BearerPriority — when both headers are set,
// Bearer wins.
func TestExtractAPIKey_BearerPriority(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer sk-123")
	r.Header.Set("x-api-key", "sk-456")
	if got := ExtractAPIKey(r); got != "sk-123" {
		t.Errorf("got %q, want %q (Bearer should win)", got, "sk-123")
	}
}

// TestExtractAPIKey_NoHeaders — AC-003: no auth headers at all → "".
func TestExtractAPIKey_NoHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if got := ExtractAPIKey(r); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// TestExtractAPIKey_BasicAuth — AC-004: "Authorization: Basic abc"
// must NOT be treated as a Bearer token. The Node.js implementation
// only matches the literal "Bearer " prefix.
func TestExtractAPIKey_BasicAuth(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Basic abc")
	if got := ExtractAPIKey(r); got != "" {
		t.Errorf("got %q, want empty string (Basic auth must be rejected)", got)
	}
}

// TestExtractAPIKey_EmptyBearer — AC variant: "Authorization: Bearer "
// (with a space but no token) slices to "". Matches the Node.js
// behaviour of returning the empty string for that case.
func TestExtractAPIKey_EmptyBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer ")
	if got := ExtractAPIKey(r); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// TestExtractAPIKey_CaseInsensitive — AC-005: in production, Go's
// net/http server canonicalises all incoming header names to title
// case, so Header.Get("Authorization") finds a client-sent
// "authorization"/"Authorization"/"AUTHORIZATION" header identically.
//
// Here we use Header.Set (which canonicalises) to confirm parity.
func TestExtractAPIKey_CaseInsensitive(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("authorization", "Bearer sk-abc")
	if got := ExtractAPIKey(r); got != "sk-abc" {
		t.Errorf("'authorization' (lowercase set) got %q, want %q", got, "sk-abc")
	}

	r2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r2.Header.Set("AUTHORIZATION", "Bearer sk-def")
	if got := ExtractAPIKey(r2); got != "sk-def" {
		t.Errorf("'AUTHORIZATION' (uppercase set) got %q, want %q", got, "sk-def")
	}
}

// TestExtractAPIKey_LowercaseBearer — the Node.js implementation
// matches the prefix case-sensitively. "bearer sk-123" does NOT
// match. This is a documented difference and we mirror it so the
// byte-for-byte contract holds.
func TestExtractAPIKey_LowercaseBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "bearer sk-123")
	// Falls through to x-api-key fallback (empty here) → "".
	if got := ExtractAPIKey(r); got != "" {
		t.Errorf("lowercase 'bearer' got %q, want empty string (Node.js parity)", got)
	}
}

// TestExtractAPIKey_NilRequest — defensive: a nil request must not
// panic. The function is called from middleware and unit tests in
// many call sites.
func TestExtractAPIKey_NilRequest(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ExtractAPIKey(nil) panicked: %v", r)
		}
	}()
	if got := ExtractAPIKey(nil); got != "" {
		t.Errorf("nil request got %q, want empty string", got)
	}
}

// TestExtractAPIKey_BearerWithMultipleSegments — a key that itself
// contains spaces (unusual but legal) should be returned intact
// after the first "Bearer " prefix.
func TestExtractAPIKey_BearerWithMultipleSegments(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer my complex key with spaces")
	if got := ExtractAPIKey(r); got != "my complex key with spaces" {
		t.Errorf("got %q, want %q", got, "my complex key with spaces")
	}
}
