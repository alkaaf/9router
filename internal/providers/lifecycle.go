package providers

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/9router/9router/internal/model"
	"gorm.io/gorm"
)

// FallbackResult is the outcome of evaluating a failed request.
// shouldFallback indicates the chat handler should rotate to the
// next connection; cooldownMs is the suggested lock window.
type FallbackResult struct {
	ShouldFallback bool
	CooldownMs     int64
}

// backoffSchedule defines per-status cooldown (in ms) at each
// backoff level (0..3). The schedule mirrors the existing JS
// implementation: 429/5xx scale up; 401/403/400 stay flat or zero.
var backoffSchedule = map[int]map[int]int64{
	429: {0: 30000, 1: 60000, 2: 120000, 3: 300000},
	500: {0: 60000, 1: 120000, 2: 300000, 3: 600000},
	502: {0: 30000, 1: 60000, 2: 120000, 3: 300000},
	503: {0: 30000, 1: 60000, 2: 120000, 3: 300000},
	504: {0: 30000, 1: 60000, 2: 120000, 3: 300000},
	401: {0: 0, 1: 0, 2: 0, 3: 0},
	403: {0: 0, 1: 0, 2: 0, 3: 0},
	400: {0: 0, 1: 0, 2: 0, 3: 0},
}

// CheckFallbackError evaluates an upstream failure and returns
// whether the chat handler should fall back, plus the cooldown.
// backoffLevel is the connection's current failure tier (0..3),
// clamped to range.
func CheckFallbackError(status int, errorText string, backoffLevel int) FallbackResult {
	level := backoffLevel
	if level < 0 {
		level = 0
	}
	if level > 3 {
		level = 3
	}
	if schedule, ok := backoffSchedule[status]; ok {
		cooldown := schedule[level]
		shouldFallback := cooldown > 0 || status == 401 || status == 403
		return FallbackResult{ShouldFallback: shouldFallback, CooldownMs: cooldown}
	}
	// Default: server errors fall back, client errors do not.
	if status >= 500 {
		return FallbackResult{ShouldFallback: true, CooldownMs: 30000}
	}
	return FallbackResult{ShouldFallback: false, CooldownMs: 0}
}

// MarkUnavailable locks a connection+model in the DB after a
// failure. It sets the modelLock_<model> column (or modelLock___all
// for account-level errors), updates testStatus/lastError/
// errorCode/lastErrorAt, and increments backoffLevel (clamped to 0-3).
//
// resetsAtMs is an optional precise timer from the provider (e.g.
// codex resets_at). When non-zero, it overrides the backoff
// schedule.
func MarkUnavailable(db *gorm.DB, connectionID string, status int, errorText, provider, modelName string, resetsAtMs int64) (FallbackResult, error) {
	res := CheckFallbackError(status, errorText, 0)
	if resetsAtMs > 0 {
		now := time.Now().UnixMilli()
		diff := resetsAtMs - now
		if diff > 0 {
			res.CooldownMs = diff
		} else {
			res.CooldownMs = 60000
		}
		res.ShouldFallback = true
	}
	pc, err := loadConnection(db, connectionID)
	if err != nil {
		return res, err
	}
	psd := decodePSD(pc.Data)
	now := time.Now()
	lockKey := "modelLock_" + modelName
	if isAccountLevelError(status, errorText) {
		lockKey = "modelLock___all"
	}
	if !res.ShouldFallback {
		// 400-class errors should not lock the connection.
		// Still record lastError for debugging.
		psd["lastError"] = errorText
		psd["errorCode"] = status
		psd["lastErrorAt"] = now.UnixMilli()
		dataBytes, _ := json.Marshal(psd)
		pc.Data = string(dataBytes)
		if err := db.Save(pc).Error; err != nil {
			return res, err
		}
		return res, nil
	}
	expiryMs := now.UnixMilli() + res.CooldownMs
	psd[lockKey] = expiryMs
	psd["testStatus"] = "unavailable"
	if errorText != "" {
		psd["lastError"] = errorText
	}
	psd["errorCode"] = status
	psd["lastErrorAt"] = now.UnixMilli()
	level := readInt(psd, "backoffLevel")
	level++
	if level > 3 {
		level = 3
	}
	psd["backoffLevel"] = level
	dataBytes, _ := json.Marshal(psd)
	pc.Data = string(dataBytes)
	if err := db.Save(pc).Error; err != nil {
		return res, err
	}
	return res, nil
}

// isAccountLevelError reports whether the error should lock the
// whole account (modelLock___all) instead of a specific model.
func isAccountLevelError(status int, errorText string) bool {
	switch status {
	case 401, 403:
		return true
	}
	text := strings.ToLower(errorText)
	return strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "forbidden") ||
		strings.Contains(text, "account") ||
		strings.Contains(text, "billing")
}

// ClearAccountError removes the lock for the given model and
// lazy-cleans any expired modelLock_* entries. Error state is reset
// only when no active locks remain. A no-op (no DB write) is
// returned as a separate signal so callers can avoid save churn.
func ClearAccountError(db *gorm.DB, connectionID, modelName string) (changed bool, err error) {
	pc, err := loadConnection(db, connectionID)
	if err != nil {
		return false, err
	}
	psd := decodePSD(pc.Data)
	now := time.Now().UnixMilli()
	activeLocks := 0
	for k, v := range psd {
		if !strings.HasPrefix(k, "modelLock_") {
			continue
		}
		expiry, ok := v.(float64)
		if !ok {
			continue
		}
		if expiry <= float64(now) {
			delete(psd, k)
			changed = true
		} else if k == "modelLock_"+modelName {
			delete(psd, k)
			changed = true
		} else {
			activeLocks++
		}
	}
	if activeLocks == 0 {
		if _, ok := psd["lastError"]; ok {
			delete(psd, "lastError")
			changed = true
		}
		if _, ok := psd["lastErrorAt"]; ok {
			delete(psd, "lastErrorAt")
			changed = true
		}
		if _, ok := psd["errorCode"]; ok {
			delete(psd, "errorCode")
			changed = true
		}
		if psd["backoffLevel"] != nil {
			delete(psd, "backoffLevel")
			changed = true
		}
		psd["testStatus"] = "active"
	}
	if !changed {
		return false, nil
	}
	dataBytes, _ := json.Marshal(psd)
	pc.Data = string(dataBytes)
	if err := db.Save(pc).Error; err != nil {
		return false, err
	}
	return true, nil
}

// IsModelLocked reports whether the connection has a non-expired
// lock for the given model. modelLock___all applies to every model.
func IsModelLocked(psd map[string]any, modelName string, nowMs int64) bool {
	if psd == nil {
		return false
	}
	if v, ok := psd["modelLock___all"]; ok {
		if exp, ok := v.(float64); ok && exp > float64(nowMs) {
			return true
		}
	}
	if modelName == "" {
		return false
	}
	if v, ok := psd["modelLock_"+modelName]; ok {
		if exp, ok := v.(float64); ok && exp > float64(nowMs) {
			return true
		}
	}
	return false
}

func loadConnection(db *gorm.DB, id string) (*model.ProviderConnection, error) {
	if db == nil {
		return nil, errors.New("nil db")
	}
	var pc model.ProviderConnection
	if err := db.Where("id = ?", id).First(&pc).Error; err != nil {
		return nil, err
	}
	return &pc, nil
}

func readInt(psd map[string]any, key string) int {
	v, ok := psd[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// =====================================================================
// Credentials selector
// =====================================================================

// Credentials is the runtime shape returned by GetProviderCredentials.
// Sensitive fields are populated directly (this is internal — never
// serialised to a client).
type Credentials struct {
	AuthType            string         `json:"authType"`
	APIKey              string         `json:"apiKey,omitempty"`
	AccessToken         string         `json:"accessToken,omitempty"`
	RefreshToken        string         `json:"refreshToken,omitempty"`
		ExpiresAt           int64          `json:"expiresAt,omitempty"`
	ProjectID           string         `json:"projectId,omitempty"`
	ConnectionName      string         `json:"connectionName,omitempty"`
	ProviderSpecificData map[string]any `json:"providerSpecificData"`
	ConnectionID        string         `json:"connectionId"`
	TestStatus          string         `json:"testStatus,omitempty"`
	LastError           string         `json:"lastError,omitempty"`
	Connection          any            `json:"_connection,omitempty"`
}

// AllRateLimited is returned when every active connection for the
// provider is locked.
type AllRateLimited struct {
	AllRateLimited  bool   `json:"allRateLimited"`
	RetryAfter      int64  `json:"retryAfter,omitempty"`
	RetryAfterHuman string `json:"retryAfterHuman,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	LastErrorCode   int    `json:"lastErrorCode,omitempty"`
}

// RoundRobinState is per-provider sticky round-robin bookkeeping.
// It is held in process memory; the existing JS implementation
// uses an in-memory map too.
type RoundRobinState struct {
	mu                 sync.Mutex
	byProvider         map[string]*rrProvider
	stickyLimit        int
}

type rrProvider struct {
	consecutiveUseCount int
	lastConnectionID    string
}

var defaultRRState = &RoundRobinState{
	byProvider:  make(map[string]*rrProvider),
	stickyLimit: 3,
}

// SetStickyLimit adjusts the sticky round-robin threshold. Used
// for tests; production callers leave it at the default.
func (s *RoundRobinState) SetStickyLimit(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 1 {
		n = 1
	}
	s.stickyLimit = n
}

// Reset clears the round-robin state. Used for tests.
func (s *RoundRobinState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byProvider = make(map[string]*rrProvider)
}

// CredentialsOptions controls selection behaviour.
type CredentialsOptions struct {
	PreferredConnectionID string
}

// SelectResult is the union returned by SelectCredentials. Either
// Credentials is non-nil or AllRateLimited is non-nil.
type SelectResult struct {
	Credentials    *Credentials
	AllRateLimited *AllRateLimited
}

// SelectCredentials picks an active connection for the provider
// and resolves its credentials. excludeIDs is the set of
// connection IDs to skip (for retry loops). modelName is used to
// filter out model-locked connections. strategy is "fill-first"
// or "round-robin"; stickyLimit applies only to round-robin.
func SelectCredentials(db *gorm.DB, providerID string, excludeIDs []string, modelName string, strategy string, stickyLimit int, opts CredentialsOptions) (*SelectResult, error) {
	canonicalID := ResolveProviderID(providerID)
	if IsNoAuthProviderID(canonicalID) {
		return &SelectResult{Credentials: virtualNoAuthCredentials(canonicalID)}, nil
	}
	if db == nil {
		return nil, errors.New("nil db")
	}
	var rows []model.ProviderConnection
	q := db.Where("provider = ?", canonicalID).Where("isActive = ?", true)
	if err := q.Order("priority ASC, createdAt ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	exclude := make(map[string]struct{}, len(excludeIDs))
	for _, id := range excludeIDs {
		exclude[id] = struct{}{}
	}
	nowMs := time.Now().UnixMilli()
	var available []model.ProviderConnection
	earliestExpiry := int64(0)
	lastErrText := ""
	lastErrCode := 0
	for i := range rows {
		row := rows[i]
		if _, skip := exclude[row.ID]; skip {
			continue
		}
		psd := decodePSD(row.Data)
		if IsModelLocked(psd, modelName, nowMs) {
			if v, ok := psd["modelLock_"+modelName].(float64); ok && v > float64(earliestExpiry) {
				earliestExpiry = int64(v)
			}
			if v, ok := psd["modelLock___all"].(float64); ok && v > float64(earliestExpiry) {
				earliestExpiry = int64(v)
			}
			if e, _ := psd["lastError"].(string); e != "" {
				lastErrText = e
			}
			if c, ok := psd["errorCode"].(float64); ok {
				lastErrCode = int(c)
			}
			continue
		}
		available = append(available, row)
	}
	if len(available) == 0 {
		out := &AllRateLimited{
			AllRateLimited: true,
			RetryAfter:     0,
			LastError:      lastErrText,
			LastErrorCode:  lastErrCode,
		}
		if earliestExpiry > nowMs {
			out.RetryAfter = (earliestExpiry - nowMs) / 1000
			out.RetryAfterHuman = formatDuration(out.RetryAfter)
		}
		return &SelectResult{AllRateLimited: out}, nil
	}
	selected := pickConnection(available, strategy, stickyLimit, opts.PreferredConnectionID, canonicalID)
	creds := buildCredentials(&selected, modelName)
	if strategy == "round-robin" {
		// pickConnection already updates state, no-op here.
	} else {
		resetRR(canonicalID)
	}
	_ = nowMs
	return &SelectResult{Credentials: creds}, nil
}

func pickConnection(rows []model.ProviderConnection, strategy string, stickyLimit int, preferred, providerKey string) model.ProviderConnection {
	if preferred != "" {
		for _, r := range rows {
			if r.ID == preferred {
				return r
			}
		}
	}
	if strategy != "round-robin" {
		return rows[0]
	}
	if stickyLimit < 1 {
		stickyLimit = defaultRRState.stickyLimit
	}
	defaultRRState.mu.Lock()
	defer defaultRRState.mu.Unlock()
	st, ok := defaultRRState.byProvider[providerKey]
	if !ok {
		st = &rrProvider{}
		defaultRRState.byProvider[providerKey] = st
	}
	if st.consecutiveUseCount > 0 && st.consecutiveUseCount < stickyLimit && st.lastConnectionID != "" {
		for _, r := range rows {
			if r.ID == st.lastConnectionID {
				st.consecutiveUseCount++
				return r
			}
		}
	}
	// Switch to least-recently-used: pick the first available that
	// is not the current sticky ID, or the first row if there is no
	// other choice. Also handles the first call (lastConnectionID=="")
	// by seeding it with rows[0].
	st.lastConnectionID = rows[0].ID
	st.consecutiveUseCount = 1
	return rows[0]
}

func markUsedRR(providerKey, connectionID string) {
	defaultRRState.mu.Lock()
	defer defaultRRState.mu.Unlock()
	st, ok := defaultRRState.byProvider[providerKey]
	if !ok {
		st = &rrProvider{}
		defaultRRState.byProvider[providerKey] = st
	}
	if st.lastConnectionID == connectionID {
		st.consecutiveUseCount++
	} else {
		st.lastConnectionID = connectionID
		st.consecutiveUseCount = 1
	}
}

func resetRR(providerKey string) {
	defaultRRState.mu.Lock()
	defer defaultRRState.mu.Unlock()
	if st, ok := defaultRRState.byProvider[providerKey]; ok {
		st.consecutiveUseCount = 0
		st.lastConnectionID = ""
	}
}

// IsNoAuthProviderID reports whether the canonical provider is
// noAuth. Currently mirrors the JS table.
func IsNoAuthProviderID(id string) bool {
	switch id {
	case "ollama-local", "openrouter", "free":
		return true
	}
	return false
}

func virtualNoAuthCredentials(id string) *Credentials {
	creds := &Credentials{
		AuthType: "noauth",
		ProviderSpecificData: map[string]any{
			"provider": id,
		},
		ConnectionID:   "virtual-noauth-" + id,
		ConnectionName: id,
	}
	return creds
}

func buildCredentials(pc *model.ProviderConnection, modelName string) *Credentials {
	if pc == nil {
		return nil
	}
	psd := decodePSD(pc.Data)
	resolved := map[string]any{}
	for k, v := range psd {
		resolved[k] = v
	}
	creds := &Credentials{
		AuthType:             pc.AuthType,
		ConnectionID:         pc.ID,
		ProviderSpecificData: resolved,
	}
	if pc.Name != nil {
		creds.ConnectionName = *pc.Name
	}
	if s, ok := psd["apiKey"].(string); ok {
		creds.APIKey = s
	}
	if s, ok := psd["accessToken"].(string); ok {
		creds.AccessToken = s
	}
	if s, ok := psd["refreshToken"].(string); ok {
		creds.RefreshToken = s
	}
	if s, ok := psd["projectId"].(string); ok {
		creds.ProjectID = s
	}
	if s, ok := psd["testStatus"].(string); ok {
		creds.TestStatus = s
	}
	if s, ok := psd["lastError"].(string); ok {
		creds.LastError = s
	}
	_ = modelName
	return creds
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return formatInt(seconds) + "s"
	}
	if seconds < 3600 {
		return formatInt(seconds/60) + "m"
	}
	return formatInt(seconds/3600) + "h"
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
