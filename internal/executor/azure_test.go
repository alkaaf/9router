package executor

import (
	"strings"
	"testing"
)

func TestNewAzureExecutor_URL(t *testing.T) {
	e := NewAzureExecutor("https://foo.openai.azure.com", "2024-02-15-preview", "gpt-4", "")
	got := e.BuildUrl("gpt-4", false, 0)
	want := "https://foo.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-15-preview"
	if got != want {
		t.Errorf("BuildUrl = %q, want %q", got, want)
	}
}

func TestNewAzureExecutor_URLWithStream(t *testing.T) {
	e := NewAzureExecutor("https://foo.openai.azure.com", "2024-02-15-preview", "gpt-4", "")
	got := e.BuildUrl("gpt-4", true, 0)
	if !strings.Contains(got, "stream=true") {
		t.Errorf("stream URL should contain stream=true: %q", got)
	}
	if !strings.Contains(got, "api-version=2024-02-15-preview") {
		t.Errorf("URL should contain api-version: %q", got)
	}
}

func TestNewAzureExecutor_DefaultsAPIVersion(t *testing.T) {
	e := NewAzureExecutor("https://x", "", "gpt-4", "")
	if e.apiVersion != DefaultAzureAPIVersion {
		t.Errorf("apiVersion = %q, want %q", e.apiVersion, DefaultAzureAPIVersion)
	}
}

func TestAzureExecutor_BuildUrl_DeploymentFallbackToModel(t *testing.T) {
	e := NewAzureExecutor("https://x", "v", "", "")
	got := e.BuildUrl("claude-3", false, 0)
	if !strings.Contains(got, "/deployments/claude-3/") {
		t.Errorf("model name should fall back as deployment: %q", got)
	}
}

func TestAzureExecutor_Headers_APIKey(t *testing.T) {
	e := NewAzureExecutor("https://x", "v", "d", "")
	h := e.BuildHeaders(&Request{Model: "gpt-4"}, &Credentials{AccessToken: "azure-key-123"})
	if got := h.Get("api-key"); got != "azure-key-123" {
		t.Errorf("api-key = %q, want azure-key-123", got)
	}
	if h.Get("Authorization") != "" {
		t.Errorf("Authorization should not be set for Azure")
	}
}

func TestAzureExecutor_Headers_WithOrganization(t *testing.T) {
	e := NewAzureExecutor("https://x", "v", "d", "my-org")
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "k"})
	if got := h.Get("OpenAI-Organization"); got != "my-org" {
		t.Errorf("OpenAI-Organization = %q, want my-org", got)
	}
}

func TestAzureExecutor_Headers_WithoutOrganization(t *testing.T) {
	e := NewAzureExecutor("https://x", "v", "d", "")
	h := e.BuildHeaders(&Request{}, &Credentials{AccessToken: "k"})
	if h.Get("OpenAI-Organization") != "" {
		t.Errorf("OpenAI-Organization should be empty when org not provided")
	}
}

func TestAzureExecutor_TransformRequest_Passthrough(t *testing.T) {
	e := NewAzureExecutor("https://x", "v", "d", "")
	in := []byte(`{"messages":[]}`)
	out, err := e.TransformRequest("m", in, false, &Credentials{AccessToken: "k"})
	if err != nil || string(out) != string(in) {
		t.Errorf("Azure transform should be a passthrough")
	}
}

func TestAzureConfig_ApplyToExecutor(t *testing.T) {
	e := NewDefaultAzureExecutor()
	cfg := AzureConfig{
		Endpoint:     "https://new.openai.azure.com",
		APIVersion:   "2025-01-01-preview",
		Deployment:   "gpt-4o",
		Organization: "org",
	}
	cfg.ApplyToExecutor(e)
	if e.endpoint != "https://new.openai.azure.com" {
		t.Errorf("endpoint not applied")
	}
	if e.apiVersion != "2025-01-01-preview" {
		t.Errorf("apiVersion not applied")
	}
	if e.deployment != "gpt-4o" {
		t.Errorf("deployment not applied")
	}
	if e.organization != "org" {
		t.Errorf("organization not applied")
	}
}

func TestAzureConfig_ApplyToExecutor_PreservesExisting(t *testing.T) {
	e := NewAzureExecutor("https://orig", "v1", "dep1", "org1")
	cfg := AzureConfig{Endpoint: "https://new"}
	cfg.ApplyToExecutor(e)
	if e.endpoint != "https://new" {
		t.Errorf("endpoint not updated")
	}
	if e.apiVersion != "v1" {
		t.Errorf("apiVersion should be preserved when not in cfg")
	}
}

func TestAzureExecutor_Registered(t *testing.T) {
	if !HasSpecializedExecutor("azure") {
		t.Errorf("azure should be registered")
	}
	e := GetExecutor("azure")
	if _, ok := e.(*AzureExecutor); !ok {
		t.Errorf("GetExecutor(azure) = %T, want *AzureExecutor", e)
	}
}
