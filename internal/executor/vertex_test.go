package executor

import (
	"testing"
	"time"
)

func TestParseVertexSA_Valid(t *testing.T) {
	raw := []byte(`{
		"type": "service_account",
		"project_id": "my-project",
		"client_email": "sa@my-project.iam.gserviceaccount.com",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIE...==\n-----END PRIVATE KEY-----\n",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`)
	sa, err := ParseVertexSA(raw)
	if err != nil {
		t.Fatalf("ParseVertexSA: %v", err)
	}
	if sa.ProjectID != "my-project" {
		t.Errorf("ProjectID = %q, want my-project", sa.ProjectID)
	}
	if sa.ClientEmail != "sa@my-project.iam.gserviceaccount.com" {
		t.Errorf("ClientEmail = %q, want sa@my-project.iam.gserviceaccount.com", sa.ClientEmail)
	}
	if sa.PrivateKey == "" {
		t.Errorf("PrivateKey should be non-empty")
	}
}

func TestParseVertexSA_DefaultsTokenURI(t *testing.T) {
	raw := []byte(`{"project_id":"p","client_email":"c","private_key":"k"}`)
	sa, _ := ParseVertexSA(raw)
	if sa.TokenURI != "https://oauth2.googleapis.com/token" {
		t.Errorf("TokenURI default = %q", sa.TokenURI)
	}
}

func TestParseVertexSA_MissingFields(t *testing.T) {
	cases := [][]byte{
		[]byte(`{}`),
		[]byte(`{"project_id":"p"}`),
		[]byte(`{"project_id":"p","client_email":"c"}`),
	}
	for i, raw := range cases {
		if _, err := ParseVertexSA(raw); err == nil {
			t.Errorf("case %d: expected error for missing fields", i)
		}
	}
}

func TestParseVertexSA_BadJSON(t *testing.T) {
	if _, err := ParseVertexSA([]byte("not json")); err == nil {
		t.Errorf("expected error for bad JSON")
	}
}

func TestVertexExecutor_GeminiURL(t *testing.T) {
	e, _ := NewVertexExecutor("my-proj", "us-central1", []byte(`{"project_id":"my-proj","client_email":"c","private_key":"k"}`))
	got := e.BuildUrl("gemini-1.5-pro", true, 0)
	want := "https://aiplatform.googleapis.com/v1/projects/my-proj/locations/us-central1/publishers/google/models/gemini-1.5-pro:streamGenerateContent?alt=sse"
	if got != want {
		t.Errorf("BuildUrl = %q, want %q", got, want)
	}
}

func TestVertexExecutor_PartnerURL(t *testing.T) {
	e, _ := NewVertexPartnerExecutor("my-proj", []byte(`{"project_id":"my-proj","client_email":"c","private_key":"k"}`))
	got := e.BuildUrl("llama3", false, 0)
	want := "https://aiplatform.googleapis.com/v1/projects/my-proj/locations/global/endpoints/openapi/chat/completions"
	if got != want {
		t.Errorf("BuildUrl = %q, want %q", got, want)
	}
}

func TestVertexExecutor_FallbackProjectIDFromSA(t *testing.T) {
	e, _ := NewVertexExecutor("", "us-central1", []byte(`{"project_id":"from-sa","client_email":"c","private_key":"k"}`))
	if e.projectID != "from-sa" {
		t.Errorf("projectID = %q, want from-sa", e.projectID)
	}
}

func TestVertexExecutor_Headers_Bearer(t *testing.T) {
	e, _ := NewVertexExecutor("p", "us", []byte(`{"project_id":"p","client_email":"c","private_key":"k"}`))
	e.PutToken("ya29.fake", 3600)
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "ignored-when-cached"})
	if got := h.Get("Authorization"); got != "Bearer ya29.fake" {
		t.Errorf("Authorization = %q, want Bearer ya29.fake", got)
	}
}

func TestVertexExecutor_NeedsRefresh(t *testing.T) {
	e, _ := NewVertexExecutor("p", "us", []byte(`{"project_id":"p","client_email":"c","private_key":"k"}`))
	if !e.NeedsRefresh(nil) {
		t.Errorf("empty cache should trigger refresh")
	}
	e.PutToken("t", 3600)
	if e.NeedsRefresh(nil) {
		t.Errorf("cached token should not trigger refresh")
	}
}

func TestVertexExecutor_TokenCache(t *testing.T) {
	cache := newVertexTokenCache()
	cache.Put("a", "tok-a", 3600)
	if got := cache.Get("a"); got != "tok-a" {
		t.Errorf("Get = %q, want tok-a", got)
	}
	cache.Put("a", "tok-b", 0) // expired
	if got := cache.Get("a"); got != "tok-b" {
		t.Errorf("Get with 0 TTL = %q, want tok-b (zero TTL means default 3600s)", got)
	}

	// Expired token.
	cache.Put("c", "tok-c", -1)
	// expiresIn=-1 → stored with time.Now() + 0 - 60s = 60s ago.
	// Actually with -1 we get stored with time.Now() + (-1-60)s. Let
	// me re-check: stored with 3600 default and -60 skew = 3540s.
	// Re-set with explicit short TTL.
	cache.mu.Lock()
	cache.tokens["c"] = cachedToken{accessToken: "tok-c", expiresAt: time.Now().Add(-time.Second)}
	cache.mu.Unlock()
	if got := cache.Get("c"); got != "" {
		t.Errorf("expired token should be empty, got %q", got)
	}
}

func TestResolveProjectID(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"permission denied on projects/my-proj/loc", "my-proj"},
		{"projects/another/locations/us", "another"},
		{"error: projects/p/whatever", "p"},
		{"no project here", ""},
		{"projects/", ""},
	}
	for _, c := range cases {
		if got := ResolveProjectID(c.body); got != c.want {
			t.Errorf("ResolveProjectID(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

func TestVertexExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("vertex") {
		t.Errorf("vertex should be registered")
	}
	if !HasSpecializedExecutor("vertex-partner") {
		t.Errorf("vertex-partner should be registered")
	}
}
