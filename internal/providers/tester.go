package providers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// TestResult is the outcome of a single connection test.
type TestResult struct {
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
	Refreshed  bool   `json:"refreshed"`
	LatencyMs  int64  `json:"latencyMs,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
}

// TestInput is the request body for /api/providers/validate.
type TestInput struct {
	Provider             string         `json:"provider"`
	AuthType             string         `json:"authType"`
	APIKey               string         `json:"apiKey,omitempty"`
	AccessToken          string         `json:"accessToken,omitempty"`
	RefreshToken         string         `json:"refreshToken,omitempty"`
	ExpiresAt            *int64         `json:"expiresAt,omitempty"`
	ProviderSpecificData map[string]any `json:"providerSpecificData,omitempty"`
}

// ValidateTests tests ad-hoc credentials without touching the DB.
// Used by the /api/providers/validate endpoint (PROV-008).
func ValidateTests(in TestInput) (TestResult, error) {
	if !IsKnownProvider(in.Provider) {
		return TestResult{}, errors.New("unknown provider: " + in.Provider)
	}
	authType := in.AuthType
	if authType == "" {
		authType = inferAuthType(in.Provider)
	}
	switch authType {
	case "noauth", "free":
		return TestResult{Valid: true}, nil
	case "cookie":
		if strings.TrimSpace(in.AccessToken) == "" {
			return TestResult{Valid: false, Error: "cookie auth requires accessToken"}, nil
		}
		return TestResult{Valid: true}, nil
	case "oauth":
		return validateOAuth(in)
	default:
		return validateAPIKey(in)
	}
}

func validateAPIKey(in TestInput) (TestResult, error) {
	if strings.TrimSpace(in.APIKey) == "" {
		return TestResult{Valid: false, Error: "apiKey is required"}, nil
	}
	// Try a known endpoint if we have one.
	if endpoint, ok := knownEndpoint(in.Provider, in.ProviderSpecificData); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		req.Header.Set("Authorization", "Bearer "+in.APIKey)
		started := time.Now()
		resp, err := http.DefaultClient.Do(req)
		lat := time.Since(started).Milliseconds()
		if err != nil {
			return TestResult{Valid: false, Error: err.Error(), LatencyMs: lat}, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return TestResult{Valid: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode), LatencyMs: lat, StatusCode: resp.StatusCode}, nil
		}
		return TestResult{Valid: true, LatencyMs: lat, StatusCode: resp.StatusCode}, nil
	}
	return TestResult{Valid: true}, nil
}

func validateOAuth(in TestInput) (TestResult, error) {
	if strings.TrimSpace(in.AccessToken) == "" {
		return TestResult{Valid: false, Error: "accessToken is required"}, nil
	}
	now := time.Now().UnixMilli()
	if in.ExpiresAt != nil && *in.ExpiresAt > 0 && *in.ExpiresAt <= now {
		if strings.TrimSpace(in.RefreshToken) == "" {
			return TestResult{Valid: false, Error: "token expired and no refreshToken"}, nil
		}
		_, _, err := RefreshOAuthToken(in.Provider, in.RefreshToken, in.ProviderSpecificData)
		if err != nil {
			return TestResult{Valid: false, Error: "refresh failed: " + err.Error()}, nil
		}
		return TestResult{Valid: true, Refreshed: true}, nil
	}
	return TestResult{Valid: true}, nil
}

func knownEndpoint(providerID string, psd map[string]any) (string, bool) {
	if psd != nil {
		if base, ok := psd["baseUrl"].(string); ok && base != "" {
			return strings.TrimRight(base, "/") + "/models", true
		}
	}
	switch providerID {
	case "openai":
		return "https://api.openai.com/v1/models", true
	case "anthropic":
		return "https://api.anthropic.com/v1/models", true
	case "openrouter":
		return "https://openrouter.ai/api/v1/models", true
	case "groq":
		return "https://api.groq.com/openai/v1/models", true
	case "mistral":
		return "https://api.mistral.ai/v1/models", true
	case "deepseek":
		return "https://api.deepseek.com/v1/models", true
	}
	return "", false
}

func inferAuthType(providerID string) string {
	switch providerID {
	case "grok-web", "perplexity-web", "iflow", "cursor", "kiro":
		return "cookie"
	case "ollama-local", "openrouter", "free":
		return "noauth"
	}
	return "apikey"
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// pcRow is an internal alias for the providerConnections table so
// this file does not need to import model directly.
type pcRow struct {
	ID        string    `gorm:"column:id"`
	Provider  string    `gorm:"column:provider"`
	AuthType  string    `gorm:"column:authType"`
	Name      *string   `gorm:"column:name"`
	Priority  *int      `gorm:"column:priority"`
	IsActive  *bool     `gorm:"column:isActive"`
	Data      string    `gorm:"column:data"`
	CreatedAt time.Time `gorm:"column:createdAt"`
	UpdatedAt time.Time `gorm:"column:updatedAt"`
}

func (pcRow) TableName() string {
	return "providerConnections"
}

// loadPC loads one row from providerConnections. gorm.DB must not
// be nil; a nil db signals a programming error.
func loadPC(db *gorm.DB, id string) (*pcRow, error) {
	if db == nil {
		return nil, errors.New("nil db")
	}
	var r pcRow
	if err := db.Where("id = ?", id).First(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// TestSingleConnection runs validate on a stored connection and
// persists the updated testStatus/lastError fields (PROV-009).
// Returns 404 if the connection does not exist.
func TestSingleConnection(db *gorm.DB, id string) (TestResult, error) {
	pc, err := loadPC(db, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
			return TestResult{}, errors.New("NOT_FOUND")
		}
		return TestResult{}, err
	}
	psd := decodePSD(pc.Data)
	in := TestInput{
		Provider:             pc.Provider,
		AuthType:             pc.AuthType,
		APIKey:               stringField(psd, "apiKey"),
		AccessToken:          stringField(psd, "accessToken"),
		RefreshToken:         stringField(psd, "refreshToken"),
		ProviderSpecificData: psd,
	}
	if v, ok := psd["expiresAt"].(float64); ok {
		vInt := int64(v)
		in.ExpiresAt = &vInt
	}
	res, _ := ValidateTests(in)
	updated := decodePSD(pc.Data)
	now := time.Now().UnixMilli()
	if res.Valid {
		updated["testStatus"] = "active"
		delete(updated, "lastError")
		delete(updated, "lastErrorAt")
		delete(updated, "errorCode")
		updated["backoffLevel"] = 0
	} else {
		updated["testStatus"] = "error"
		updated["lastError"] = res.Error
		updated["lastErrorAt"] = now
		updated["errorCode"] = res.StatusCode
	}
	dataBytes, _ := json.Marshal(updated)
	pc.Data = string(dataBytes)
	pc.UpdatedAt = time.Now()
	if err := db.Save(pc).Error; err != nil {
		return res, err
	}
	return res, nil
}

// BatchResult is the response from /api/providers/test-batch.
type BatchResult struct {
	Mode       string       `json:"mode"`
	ProviderID string       `json:"providerId,omitempty"`
	Results    []BatchEntry `json:"results"`
	Summary    BatchSummary `json:"summary"`
	TestedAt   int64        `json:"testedAt"`
}

// BatchEntry is one connection's outcome in the batch.
type BatchEntry struct {
	Provider       string `json:"provider"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName,omitempty"`
	AuthType       string `json:"authType"`
	Valid          bool   `json:"valid"`
	LatencyMs      int64  `json:"latencyMs,omitempty"`
	Error          string `json:"error,omitempty"`
	Diagnosis      string `json:"diagnosis,omitempty"`
	StatusCode     int    `json:"statusCode,omitempty"`
	TestedAt       int64  `json:"testedAt"`
}

// BatchSummary is the rollup of batch results.
type BatchSummary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

var allowedBatchModes = map[string]bool{
	"all":        true,
	"oauth":      true,
	"free":       true,
	"apikey":     true,
	"compatible": true,
	"provider":   true,
}

// TestBatch runs ValidateTests on every connection matching mode
// and persists results (PROV-010).
func TestBatch(db *gorm.DB, mode, providerID string) (*BatchResult, error) {
	if db == nil {
		return nil, errors.New("nil db")
	}
	if !allowedBatchModes[mode] {
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}
	if mode == "provider" && strings.TrimSpace(providerID) == "" {
		return nil, errors.New("providerId is required for provider mode")
	}
	rows, err := queryBatchRows(db, mode, providerID)
	if err != nil {
		return nil, err
	}
	out := &BatchResult{
		Mode:     mode,
		TestedAt: time.Now().UnixMilli(),
	}
	for _, r := range rows {
		entry, _ := testOne(db, r)
		out.Results = append(out.Results, entry)
		out.Summary.Total++
		if entry.Valid {
			out.Summary.Passed++
		} else {
			out.Summary.Failed++
		}
	}
	return out, nil
}

func queryBatchRows(db *gorm.DB, mode, providerID string) ([]pcRow, error) {
	var rows []pcRow
	q := db.Model(&pcRow{})
	switch mode {
	case "oauth", "free", "apikey":
		q = q.Where("authType = ?", mode)
	case "compatible":
		q = q.Where("provider LIKE ? OR provider LIKE ?", "openai-compatible-%", "anthropic-compatible-%")
	case "provider":
		q = q.Where("provider = ?", providerID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	if mode == "free" {
		filtered := make([]pcRow, 0, len(rows))
		for _, r := range rows {
			if r.AuthType == "free" || r.AuthType == "noauth" || IsNoAuthProviderID(r.Provider) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	return rows, nil
}

func testOne(db *gorm.DB, pc pcRow) (BatchEntry, error) {
	entry := BatchEntry{
		Provider:       pc.Provider,
		ConnectionID:   pc.ID,
		AuthType:       pc.AuthType,
		ConnectionName: strDeref(pc.Name),
		TestedAt:       time.Now().UnixMilli(),
	}
	psd := decodePSD(pc.Data)
	in := TestInput{
		Provider:             pc.Provider,
		AuthType:             pc.AuthType,
		APIKey:               stringField(psd, "apiKey"),
		AccessToken:          stringField(psd, "accessToken"),
		RefreshToken:         stringField(psd, "refreshToken"),
		ProviderSpecificData: psd,
	}
	if v, ok := psd["expiresAt"].(float64); ok {
		vInt := int64(v)
		in.ExpiresAt = &vInt
	}
	res, _ := ValidateTests(in)
	entry.Valid = res.Valid
	entry.LatencyMs = res.LatencyMs
	entry.Error = res.Error
	entry.StatusCode = res.StatusCode
	entry.Diagnosis = diagnose(pc.AuthType, res.StatusCode, res.Error)
	// Persist updated testStatus / lastError.
	updated := decodePSD(pc.Data)
	now := time.Now().UnixMilli()
	if res.Valid {
		updated["testStatus"] = "active"
		delete(updated, "lastError")
		delete(updated, "lastErrorAt")
		delete(updated, "errorCode")
		updated["backoffLevel"] = 0
	} else {
		updated["testStatus"] = "error"
		updated["lastError"] = res.Error
		updated["lastErrorAt"] = now
		updated["errorCode"] = res.StatusCode
	}
	dataBytes, _ := json.Marshal(updated)
	pc.Data = string(dataBytes)
	pc.UpdatedAt = time.Now()
	if err := db.Save(&pc).Error; err != nil {
		return entry, err
	}
	return entry, nil
}

func diagnose(authType string, status int, errMsg string) string {
	if status == 401 {
		return "invalid credentials"
	}
	if status == 403 {
		return "forbidden"
	}
	if status == 429 {
		return "rate limited"
	}
	if status >= 500 {
		return "upstream error"
	}
	if strings.Contains(strings.ToLower(errMsg), "proxy") {
		return "proxy failure"
	}
	if authType == "oauth" {
		return "oauth failure"
	}
	return "invalid credentials"
}

// =====================================================================
// Suggested models — PROV-015
// =====================================================================

// SuggestedModel is one entry in the response of
// /api/providers/suggested-models.
type SuggestedModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"contextLength,omitempty"`
}

// SuggestedModelsResult is the response body.
type SuggestedModelsResult struct {
	Data []SuggestedModel `json:"data"`
}

// knownOpencodeFreeIDs lists opencode-zen model IDs that are
// always considered free even when they do not end with "-free".
var knownOpencodeFreeIDs = map[string]bool{
	"big-pickle": true,
	"gpt-5":      true,
	"gpt-5-mini": true,
}

type suggestedCacheEntry struct {
	models   []SuggestedModel
	expireAt time.Time
}

var (
	suggestedCacheMu sync.Mutex
	suggestedCache   = map[string]suggestedCacheEntry{}
)

// FetchFunc is the HTTP fetcher signature. The default fetcher
// dials the real URL; tests inject a stub.
type FetchFunc func(ctx context.Context, rawURL string) (status int, body []byte, err error)

// defaultFetcher dials the real URL. The caller is responsible
// for setting a context timeout.
var defaultFetcher FetchFunc = func(ctx context.Context, rawURL string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", "9router/1.0")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, nil, nil
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return resp.StatusCode, buf, nil
}

func suggestedCacheKey(modelType, rawURL string) string {
	return modelType + "|" + rawURL
}

// FetchSuggestedModels is the public entry point for PROV-015.
// fetcher may be nil (uses the default HTTP client).
func FetchSuggestedModels(modelType, rawURL string, fetcher FetchFunc) (*SuggestedModelsResult, error) {
	if modelType == "" || rawURL == "" {
		return nil, errors.New("url and type are required")
	}
	if _, err := url.Parse(rawURL); err != nil {
		return nil, errors.New("invalid url")
	}
	switch modelType {
	case "openrouter-free", "opencode-free":
	default:
		return nil, errors.New("unknown type: " + modelType)
	}
	if fetcher == nil {
		fetcher = defaultFetcher
	}
	key := suggestedCacheKey(modelType, rawURL)
	if cached, ok := readSuggestedCache(key); ok {
		return &SuggestedModelsResult{Data: cached}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, body, err := fetcher(ctx, rawURL)
	if err != nil || status < 200 || status >= 300 {
		return &SuggestedModelsResult{Data: []SuggestedModel{}}, nil
	}
	models, perr := parseSuggestedModels(modelType, body)
	if perr != nil {
		return &SuggestedModelsResult{Data: []SuggestedModel{}}, nil
	}
	writeSuggestedCache(key, models, 5*time.Minute)
	return &SuggestedModelsResult{Data: models}, nil
}

func readSuggestedCache(key string) ([]SuggestedModel, bool) {
	suggestedCacheMu.Lock()
	defer suggestedCacheMu.Unlock()
	entry, ok := suggestedCache[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expireAt) {
		delete(suggestedCache, key)
		return nil, false
	}
	return entry.models, true
}

func writeSuggestedCache(key string, models []SuggestedModel, ttl time.Duration) {
	suggestedCacheMu.Lock()
	defer suggestedCacheMu.Unlock()
	suggestedCache[key] = suggestedCacheEntry{models: models, expireAt: time.Now().Add(ttl)}
}

func parseSuggestedModels(modelType string, body []byte) ([]SuggestedModel, error) {
	switch modelType {
	case "openrouter-free":
		return parseOpenrouterFree(body)
	case "opencode-free":
		return parseOpencodeFree(body)
	}
	return nil, errors.New("unknown type")
}

func parseOpenrouterFree(body []byte) ([]SuggestedModel, error) {
	var payload struct {
		Data []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			ContextLen int    `json:"context_length"`
			Pricing    struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := make([]SuggestedModel, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.Pricing.Prompt != "0" || m.Pricing.Completion != "0" {
			continue
		}
		if m.ContextLen < 200000 {
			continue
		}
		out = append(out, SuggestedModel{ID: m.ID, Name: m.Name, ContextLength: m.ContextLen})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ContextLength > out[i].ContextLength {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func parseOpencodeFree(body []byte) ([]SuggestedModel, error) {
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		var bare []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err2 := json.Unmarshal(body, &bare); err2 != nil {
			return nil, err
		}
		payload.Data = bare
	}
	out := make([]SuggestedModel, 0, len(payload.Data))
	for _, m := range payload.Data {
		if !strings.HasSuffix(m.ID, "-free") && !knownOpencodeFreeIDs[m.ID] {
			continue
		}
		out = append(out, SuggestedModel{ID: m.ID, Name: m.Name})
	}
	return out, nil
}

// ResetSuggestedCache clears the in-memory cache. Used by tests.
func ResetSuggestedCache() {
	suggestedCacheMu.Lock()
	defer suggestedCacheMu.Unlock()
	suggestedCache = map[string]suggestedCacheEntry{}
}
