package providers

import (
	"testing"
	"time"

	"github.com/9router/9router/internal/model"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }

func TestValidateCreate_Success(t *testing.T) {
	req := CreateReq{
		Provider: "openai",
		APIKey:   strPtr("sk-test"),
		Name:     strPtr("Test OpenAI"),
	}
	pc, err := ValidateCreate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc.Provider != "openai" {
		t.Errorf("provider = %q, want %q", pc.Provider, "openai")
	}
	if pc.AuthType != "apikey" {
		t.Errorf("authType = %q, want %q", pc.AuthType, "apikey")
	}
}

func TestValidateCreate_NoAuthProvider(t *testing.T) {
	req := CreateReq{
		Provider: "ollama-local",
		Name:     strPtr("Local Ollama"),
	}
	pc, err := ValidateCreate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc.AuthType != "apikey" {
		t.Errorf("authType = %q, want %q (default)", pc.AuthType, "apikey")
	}
}

func TestValidateCreate_InvalidProvider(t *testing.T) {
	req := CreateReq{
		Provider: "nonexistent-provider",
		Name:     strPtr("Test"),
	}
	_, err := ValidateCreate(req)
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
}

func TestValidateCreate_MissingName(t *testing.T) {
	req := CreateReq{
		Provider: "openai",
		APIKey:   strPtr("sk-test"),
	}
	_, err := ValidateCreate(req)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateCreate_MissingAPIKey(t *testing.T) {
	req := CreateReq{
		Provider: "openai",
		Name:     strPtr("Test"),
	}
	_, err := ValidateCreate(req)
	if err == nil {
		t.Fatal("expected error for missing apiKey")
	}
}

func TestValidateCreate_CookieProvider(t *testing.T) {
	req := CreateReq{
		Provider: "grok-web",
		Name:     strPtr("Grok"),
	}
	pc, err := ValidateCreate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc.AuthType != "cookie" {
		t.Errorf("authType = %q, want %q (cookie)", pc.AuthType, "cookie")
	}
}

func TestValidateCreate_ProxyEnabledMissingURL(t *testing.T) {
	req := CreateReq{
		Provider:               "openai",
		APIKey:                 strPtr("sk-test"),
		Name:                   strPtr("Test"),
		ConnectionProxyEnabled: boolPtr(true),
	}
	_, err := ValidateCreate(req)
	if err == nil {
		t.Fatal("expected error for missing proxy URL")
	}
}

func TestValidateCreate_ProxyEnabledWithURL(t *testing.T) {
	req := CreateReq{
		Provider:               "openai",
		APIKey:                 strPtr("sk-test"),
		Name:                   strPtr("Test"),
		ConnectionProxyEnabled: boolPtr(true),
		ConnectionProxyURL:     strPtr("http://proxy:8080"),
	}
	_, err := ValidateCreate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyUpdate_NameAndPriority(t *testing.T) {
	pc := &model.ProviderConnection{
		Provider: "openai",
		AuthType: "apikey",
		Name:     strPtr("Old"),
		Priority: intPtr(5),
		Data:     `{"apiKey":"old"}`,
	}
	req := UpdateReq{
		Name:     strPtr("New"),
		Priority: intPtr(2),
	}
	if err := ApplyUpdate(pc, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *pc.Name != "New" {
		t.Errorf("name = %q, want %q", *pc.Name, "New")
	}
	if *pc.Priority != 2 {
		t.Errorf("priority = %d, want 2", *pc.Priority)
	}
}

func TestApplyUpdate_APIKey_ApikeyAuth(t *testing.T) {
	pc := &model.ProviderConnection{
		Provider: "openai",
		AuthType: "apikey",
		Data:     `{}`,
	}
	req := UpdateReq{APIKey: strPtr("new-key")}
	if err := ApplyUpdate(pc, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyUpdate_APIKey_OAuthAuth(t *testing.T) {
	pc := &model.ProviderConnection{
		Provider: "openai",
		AuthType: "oauth",
		Data:     `{}`,
	}
	req := UpdateReq{APIKey: strPtr("should-be-rejected")}
	if err := ApplyUpdate(pc, req); err == nil {
		t.Fatal("expected error: cannot update apiKey for non-apikey auth")
	}
}

func TestToView_StripsSensitive(t *testing.T) {
	pc := &model.ProviderConnection{
		ID:       "test-1",
		Provider: "openai",
		AuthType: "apikey",
		Name:     strPtr("Test"),
		Data:     `{"apiKey":"secret","accessToken":"tok","displayName":"My OpenAI"}`,
	}
	v := ToView(pc, nil)
	if v.ProviderSpecificData == nil {
		t.Fatal("expected non-nil ProviderSpecificData")
	}
	if _, ok := v.ProviderSpecificData["apiKey"]; ok {
		t.Error("apiKey should be stripped from view")
	}
	if _, ok := v.ProviderSpecificData["accessToken"]; ok {
		t.Error("accessToken should be stripped from view")
	}
	if v.ProviderSpecificData["displayName"] != "My OpenAI" {
		t.Error("non-sensitive fields should be preserved")
	}
}

func TestToView_CompatibleNameEnriched(t *testing.T) {
	pc := &model.ProviderConnection{
		ID:       "x1",
		Provider: "openai-compatible-abc",
		AuthType: "apikey",
		Name:     strPtr(""),
		Data:     `{}`,
	}
	v := ToView(pc, func(id string) string {
		if id == "openai-compatible-abc" {
			return "My Custom Node"
		}
		return ""
	})
	if v.Name == nil || *v.Name != "My Custom Node" {
		t.Errorf("name = %v, want 'My Custom Node'", v.Name)
	}
}

func TestToView_TimestampsAreUnixMilli(t *testing.T) {
	now := time.Now()
	pc := &model.ProviderConnection{
		ID:        "test",
		Provider:  "openai",
		AuthType:  "apikey",
		CreatedAt: now,
		UpdatedAt: now,
		Data:      `{}`,
	}
	v := ToView(pc, nil)
	if v.CreatedAt != now.UnixMilli() {
		t.Errorf("createdAt = %d, want %d", v.CreatedAt, now.UnixMilli())
	}
}
