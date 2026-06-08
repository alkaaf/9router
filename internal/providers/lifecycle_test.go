package providers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/9router/9router/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupLifecycleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&model.ProviderConnection{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// =====================================================================
// PROV-012: CheckFallbackError
// =====================================================================

func TestCheckFallbackError_429(t *testing.T) {
	r := CheckFallbackError(429, "rate limit", 0)
	if !r.ShouldFallback {
		t.Error("429 should fallback")
	}
	if r.CooldownMs != 30000 {
		t.Errorf("429 cooldown = %d, want 30000", r.CooldownMs)
	}
}

func TestCheckFallbackError_401(t *testing.T) {
	r := CheckFallbackError(401, "unauthorized", 0)
	if !r.ShouldFallback {
		t.Error("401 should fallback")
	}
}

func TestCheckFallbackError_500(t *testing.T) {
	r := CheckFallbackError(500, "server error", 1)
	if !r.ShouldFallback {
		t.Error("500 should fallback")
	}
	if r.CooldownMs != 120000 {
		t.Errorf("500 cooldown = %d, want 120000", r.CooldownMs)
	}
}

func TestCheckFallbackError_400(t *testing.T) {
	r := CheckFallbackError(400, "bad request", 0)
	if r.ShouldFallback {
		t.Error("400 should NOT fallback")
	}
}

func TestCheckFallbackError_ClampsLevel(t *testing.T) {
	r := CheckFallbackError(429, "", -1)
	if r.CooldownMs != 30000 {
		t.Errorf("level -1 clamped to 0, want 30000 got %d", r.CooldownMs)
	}
	r = CheckFallbackError(429, "", 10)
	if r.CooldownMs != 300000 {
		t.Errorf("level 10 clamped to 3, want 300000 got %d", r.CooldownMs)
	}
}

// =====================================================================
// PROV-012: MarkUnavailable
// =====================================================================

func TestMarkUnavailable_429(t *testing.T) {
	db := setupLifecycleDB(t)
	pc := &model.ProviderConnection{
		ID: "c1", Provider: "openai", AuthType: "apikey", Name: strPtr("T"), Data: `{}`,
	}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := MarkUnavailable(db, "c1", 429, "rate limit", "openai", "gpt-4o", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.ShouldFallback {
		t.Error("429 should fallback")
	}
	var got model.ProviderConnection
	if err := db.Where("id = ?", "c1").First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	psd := decodePSD(got.Data)
	if _, ok := psd["modelLock_gpt-4o"]; !ok {
		t.Error("modelLock_gpt-4o should be set")
	}
	if psd["testStatus"] != "unavailable" {
		t.Errorf("testStatus = %v, want unavailable", psd["testStatus"])
	}
}

func TestMarkUnavailable_401(t *testing.T) {
	db := setupLifecycleDB(t)
	pc := &model.ProviderConnection{
		ID: "c1", Provider: "openai", AuthType: "apikey", Data: `{}`,
	}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := MarkUnavailable(db, "c1", 401, "unauthorized", "openai", "gpt-4o", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.ShouldFallback {
		t.Error("401 should fallback")
	}
	var got model.ProviderConnection
	if err := db.Where("id = ?", "c1").First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	psd := decodePSD(got.Data)
	if _, ok := psd["modelLock___all"]; !ok {
		t.Error("401 should set modelLock___all")
	}
}

func TestMarkUnavailable_400(t *testing.T) {
	db := setupLifecycleDB(t)
	pc := &model.ProviderConnection{ID: "c1", Data: `{}`}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := MarkUnavailable(db, "c1", 400, "bad request", "openai", "gpt-4o", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ShouldFallback {
		t.Error("400 should NOT fallback")
	}
	var got model.ProviderConnection
	if err := db.Where("id = ?", "c1").First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	psd := decodePSD(got.Data)
	if _, ok := psd["modelLock_gpt-4o"]; ok {
		t.Error("400 should NOT set modelLock")
	}
}

func TestMarkUnavailable_ResetsAtMs(t *testing.T) {
	db := setupLifecycleDB(t)
	pc := &model.ProviderConnection{ID: "c1", Data: `{}`}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	resetsAt := time.Now().Add(2 * time.Hour).UnixMilli()
	res, err := MarkUnavailable(db, "c1", 429, "rate limit", "codex", "gpt-4", resetsAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.ShouldFallback {
		t.Error("should fallback when resetsAt provided")
	}
	if res.CooldownMs <= 0 {
		t.Errorf("cooldown = %d, want > 0", res.CooldownMs)
	}
}

// =====================================================================
// PROV-013: ClearAccountError
// =====================================================================

func TestClearAccountError_ClearModelLock(t *testing.T) {
	db := setupLifecycleDB(t)
	far := time.Now().Add(1 * time.Hour).UnixMilli()
	pc := &model.ProviderConnection{
		ID:   "c1",
		Data: `{"modelLock_gpt-4o": ` + floatStr(float64(far)) + `, "testStatus": "error", "lastError": "oops"}`,
	}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	changed, err := ClearAccountError(db, "c1", "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true")
	}
	var got model.ProviderConnection
	if err := db.Where("id = ?", "c1").First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	psd := decodePSD(got.Data)
	if _, ok := psd["modelLock_gpt-4o"]; ok {
		t.Error("modelLock_gpt-4o should be cleared")
	}
	if psd["testStatus"] != "active" {
		t.Errorf("testStatus = %v, want active", psd["testStatus"])
	}
}

func TestClearAccountError_NoOp(t *testing.T) {
	db := setupLifecycleDB(t)
	pc := &model.ProviderConnection{ID: "c1", Data: `{}`}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	changed, err := ClearAccountError(db, "c1", "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected no change")
	}
}

func TestClearAccountError_ExpiredLocksCleaned(t *testing.T) {
	db := setupLifecycleDB(t)
	past := time.Now().Add(-1 * time.Hour).UnixMilli()
	future := time.Now().Add(1 * time.Hour).UnixMilli()
	pc := &model.ProviderConnection{
		ID:   "c1",
		Data: `{"modelLock_gpt-4o": ` + floatStr(float64(past)) + `, "modelLock_claude": ` + floatStr(float64(future)) + `}`,
	}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	changed, err := ClearAccountError(db, "c1", "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected change")
	}
	var got model.ProviderConnection
	if err := db.Where("id = ?", "c1").First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	psd := decodePSD(got.Data)
	if _, ok := psd["modelLock_gpt-4o"]; ok {
		t.Error("expired lock should be cleared")
	}
	if _, ok := psd["modelLock_claude"]; !ok {
		t.Error("active lock should be preserved")
	}
}

func TestClearAccountError_ErrorStateResetOnlyWhenNoLocks(t *testing.T) {
	db := setupLifecycleDB(t)
	pc := &model.ProviderConnection{
		ID:   "c1",
		Data: `{"testStatus": "error", "lastError": "oops", "backoffLevel": 2}`,
	}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	changed, err := ClearAccountError(db, "c1", "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected change")
	}
	var got model.ProviderConnection
	if err := db.Where("id = ?", "c1").First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	psd := decodePSD(got.Data)
	if psd["testStatus"] != "active" {
		t.Errorf("testStatus = %v, want active", psd["testStatus"])
	}
	if _, ok := psd["lastError"]; ok {
		t.Error("lastError should be cleared")
	}
}

// =====================================================================
// PROV-011: SelectCredentials
// =====================================================================

func setupCredsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupLifecycleDB(t)
	rows := []*model.ProviderConnection{
		{ID: "c1", Provider: "openai", AuthType: "apikey", IsActive: boolPtr(true), Priority: intPtr(1), Name: strPtr("A"), Data: `{"apiKey":"k1"}`},
		{ID: "c2", Provider: "openai", AuthType: "apikey", IsActive: boolPtr(true), Priority: intPtr(2), Name: strPtr("B"), Data: `{"apiKey":"k2"}`},
		{ID: "c3", Provider: "openai", AuthType: "apikey", IsActive: boolPtr(true), Priority: intPtr(3), Name: strPtr("C"), Data: `{}`},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	return db
}

func TestSelectCredentials_FillFirst(t *testing.T) {
	db := setupCredsDB(t)
	res, err := SelectCredentials(db, "openai", nil, "", "fill-first", 3, CredentialsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Credentials == nil {
		t.Fatal("expected credentials")
	}
	if res.Credentials.ConnectionID != "c1" {
		t.Errorf("fill-first: got %s, want c1", res.Credentials.ConnectionID)
	}
}

func TestSelectCredentials_ExcludeSet(t *testing.T) {
	db := setupCredsDB(t)
	res, err := SelectCredentials(db, "openai", []string{"c1"}, "", "fill-first", 3, CredentialsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Credentials.ConnectionID != "c2" {
		t.Errorf("exclude: got %s, want c2", res.Credentials.ConnectionID)
	}
}

func TestSelectCredentials_NoAuthProvider(t *testing.T) {
	db := setupLifecycleDB(t)
	res, err := SelectCredentials(db, "ollama-local", nil, "", "fill-first", 3, CredentialsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Credentials == nil {
		t.Fatal("expected virtual credentials")
	}
	if res.Credentials.AuthType != "noauth" {
		t.Errorf("noauth: got authType %s", res.Credentials.AuthType)
	}
}

func TestSelectCredentials_ModelLockAllRateLimited(t *testing.T) {
	db := setupLifecycleDB(t)
	farFuture := time.Now().Add(1 * time.Hour).UnixMilli()
	pc := &model.ProviderConnection{
		ID: "c1", Provider: "openai", AuthType: "apikey", IsActive: boolPtr(true), Name: strPtr("A"),
		Data: `{"modelLock___all": ` + floatStr(float64(farFuture)) + `}`,
	}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := SelectCredentials(db, "openai", nil, "", "fill-first", 3, CredentialsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.AllRateLimited == nil {
		t.Fatal("expected allRateLimited")
	}
	if !res.AllRateLimited.AllRateLimited {
		t.Error("expected allRateLimited=true")
	}
}

func TestSelectCredentials_RoundRobinSticky(t *testing.T) {
	defaultRRState.Reset()
	db := setupCredsDB(t)
	for i := 0; i < 3; i++ {
		res, err := SelectCredentials(db, "openai", nil, "", "round-robin", 3, CredentialsOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Credentials.ConnectionID != "c1" {
			t.Errorf("sticky: iteration %d got %s, want c1", i, res.Credentials.ConnectionID)
		}
	}
}

func TestSelectCredentials_ResolveAlias(t *testing.T) {
	db := setupLifecycleDB(t)
	pc := &model.ProviderConnection{
		ID: "c1", Provider: "claude", AuthType: "apikey", IsActive: boolPtr(true), Priority: intPtr(1), Data: `{"apiKey":"k"}`,
	}
	if err := db.Create(pc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := SelectCredentials(db, "cc", nil, "", "fill-first", 3, CredentialsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Credentials == nil {
		t.Fatal("expected credentials")
	}
	if res.Credentials.ConnectionID != "c1" {
		t.Errorf("got %s, want c1", res.Credentials.ConnectionID)
	}
}

// =====================================================================
// PROV-014: Refresh token
// =====================================================================

func TestRefreshOAuthToken_MissingToken(t *testing.T) {
	_, _, err := RefreshOAuthToken("claude", "", nil)
	if err == nil {
		t.Error("expected error for missing refreshToken")
	}
}

func TestRefreshOAuthToken_UnknownProvider(t *testing.T) {
	_, _, err := RefreshOAuthToken("nonexistent", "tok", nil)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestCheckAndRefreshToken_NoRefreshWhenFresh(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).UnixMilli()
	creds := &Credentials{
		AccessToken:          "old",
		RefreshToken:         "refresh",
		ProviderSpecificData: map[string]any{"expiresAt": float64(future)},
	}
	out, err := CheckAndRefreshToken("claude", creds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AccessToken != "old" {
		t.Error("fresh token should NOT be refreshed")
	}
}

func TestCheckAndRefreshToken_NoRefreshToken(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).UnixMilli()
	creds := &Credentials{
		AccessToken:          "old",
		ProviderSpecificData: map[string]any{"expiresAt": float64(past)},
	}
	out, err := CheckAndRefreshToken("claude", creds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AccessToken != "old" {
		t.Error("missing refreshToken should not crash")
	}
}

// =====================================================================
// PROV-015: Suggested Models
// =====================================================================

func TestFetchSuggestedModels_InvalidType(t *testing.T) {
	_, err := FetchSuggestedModels("unknown", "http://x", nil)
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestFetchSuggestedModels_MissingParams(t *testing.T) {
	_, err := FetchSuggestedModels("", "", nil)
	if err == nil {
		t.Error("expected error for missing params")
	}
}

func TestFetchSuggestedModels_Non2xx(t *testing.T) {
	fetcher := func(ctx context.Context, rawURL string) (int, []byte, error) {
		return 500, nil, nil
	}
	out, err := FetchSuggestedModels("openrouter-free", "http://x", fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Data) != 0 {
		t.Error("non-2xx should return empty data")
	}
}

func TestFetchSuggestedModels_CacheHit(t *testing.T) {
	ResetSuggestedCache()
	fetchCount := 0
	fetcher := func(ctx context.Context, rawURL string) (int, []byte, error) {
		fetchCount++
		return 200, []byte(`{"data":[{"id":"m1","name":"M1","pricing":{"prompt":"0","completion":"0"},"context_length":200000}]}`), nil
	}
	_, _ = FetchSuggestedModels("openrouter-free", "http://x", fetcher)
	_, _ = FetchSuggestedModels("openrouter-free", "http://x", fetcher)
	if fetchCount != 1 {
		t.Errorf("cache: fetched %d times, want 1", fetchCount)
	}
}

func TestParseOpenrouterFree_FiltersPaid(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"free-m","name":"Free","pricing":{"prompt":"0","completion":"0"},"context_length":200000},
		{"id":"paid-m","name":"Paid","pricing":{"prompt":"0.001","completion":"0.002"},"context_length":128000}
	]}`)
	out, err := parseOpenrouterFree(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	if out[0].ID != "free-m" {
		t.Errorf("got %s, want free-m", out[0].ID)
	}
}

func TestParseOpenrouterFree_SortByContextLength(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"small","name":"S","pricing":{"prompt":"0","completion":"0"},"context_length":200000},
		{"id":"large","name":"L","pricing":{"prompt":"0","completion":"0"},"context_length":1000000}
	]}`)
	out, err := parseOpenrouterFree(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) < 2 || out[0].ID != "large" {
		t.Errorf("expected 'large' first, got %v", out)
	}
}

func TestParseOpencodeFree_FreeSuffix(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"model-free","name":"Free Model"},
		{"id":"model-paid","name":"Paid Model"}
	]}`)
	out, err := parseOpencodeFree(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	if out[0].ID != "model-free" {
		t.Errorf("got %s, want model-free", out[0].ID)
	}
}

func TestParseOpencodeFree_KnownFreeIDs(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"gpt-5","name":"GPT-5"},
		{"id":"other","name":"Other"}
	]}`)
	out, err := parseOpencodeFree(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].ID != "gpt-5" {
		t.Errorf("got %v, want [gpt-5]", out)
	}
}

func TestFetchSuggestedModels_OpenRouterSuccess(t *testing.T) {
	ResetSuggestedCache()
	fetcher := func(ctx context.Context, rawURL string) (int, []byte, error) {
		return 200, []byte(`{"data":[{"id":"m1","name":"M1","pricing":{"prompt":"0","completion":"0"},"context_length":250000}]}`), nil
	}
	out, err := FetchSuggestedModels("openrouter-free", "http://x", fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Data) != 1 || out.Data[0].ID != "m1" {
		t.Errorf("unexpected: %+v", out.Data)
	}
}

func TestFetchSuggestedModels_InvalidURL(t *testing.T) {
	_, err := FetchSuggestedModels("openrouter-free", "://bad", nil)
	if err == nil {
		t.Error("expected error for invalid url")
	}
}

// =====================================================================
// Helpers
// =====================================================================

func floatStr(f float64) string {
	// Encode via json to avoid importing strconv.
	type wrapper struct {
		V float64 `json:"v"`
	}
	b, _ := json.Marshal(wrapper{f})
	// b = `{"v":1.5}` — extract the number.
	s := string(b)
	s = s[5 : len(s)-1]
	return s
}
