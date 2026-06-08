package chatcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Strategy is the credential-selection strategy the caller wants
// applied. The string values match the Node.js settings
// (`fill-first`, `round-robin`) so JSON config files can be reused
// without translation.
type Strategy string

const (
	StrategyFillFirst  Strategy = "fill-first"
	StrategyRoundRobin Strategy = "round-robin"
)

// ConnectionData mirrors the per-row JSON column on
// providerConnections. It captures the credential fields the
// executor layer needs plus the runtime state used by the
// selection / lock / rotation logic (lastUsedAt,
// consecutiveUseCount, modelLock_*).
//
// Field semantics match open-sse/services/auth.js:
//
//   - AuthType / APIKey / AccessToken / RefreshToken / ProjectID
//     come straight from the DB row's `data` JSON column.
//   - ProviderSpecificData is the executor's per-provider payload
//     (e.g. copilotToken, antigravity project id).
//   - LastUsedAt / ConsecutiveUseCount are the round-robin
//     rotation state, written back on every successful selection.
//   - ModelLocks maps model name → earliest unlock time (RFC3339
//     string) for per-model rate-limit locks.
type ConnectionData struct {
	AuthType              string            `json:"authType"`
	APIKey                string            `json:"apiKey"`
	AccessToken           string            `json:"accessToken"`
	RefreshToken          string            `json:"refreshToken"`
	ProjectID             string            `json:"projectId"`
	ProviderSpecificData map[string]any     `json:"providerSpecificData,omitempty"`
	LastUsedAt            *time.Time        `json:"lastUsedAt,omitempty"`
	ConsecutiveUseCount   int               `json:"consecutiveUseCount,omitempty"`
	ModelLocks            map[string]string `json:"-"` // populated from modelLock_* keys
	// Catch-all for the dozens of additional fields stored in
	// the JSON column (lastError, errorCode, testStatus, ...).
	Extras map[string]any `json:"-"`
}

// Credentials is the resolved set of fields the executor needs to
// dispatch a request. ConnectionID is included so the caller can
// mark errors back to the same row.
type Credentials struct {
	ConnectionID          string
	ConnectionName        string
	AuthType              string
	APIKey                string
	AccessToken           string
	RefreshToken          string
	ProjectID             string
	ProviderSpecificData map[string]any
}

// RateLimitState is returned by SelectCredentials when every account
// is locked for the requested model. The caller is expected to
// translate this into a 429 / 503 with a Retry-After header.
type RateLimitState struct {
	AllRateLimited bool      `json:"allRateLimited"`
	RetryAfter     time.Time `json:"retryAfter"`
	RetryAfterHuman string   `json:"retryAfterHuman"`
	LastError      string    `json:"lastError,omitempty"`
	LastErrorCode  string    `json:"lastErrorCode,omitempty"`
}

// CandidateConnection is the minimal view of a providerConnections
// row the selection logic needs. The DB layer (PROV) will return
// these from a GORM query.
type CandidateConnection struct {
	ID       string
	Provider string
	Name     string
	Email    string
	Priority *int
	IsActive *bool
	Data     string // raw JSON column
	// Parsed cached view of Data. Set by LoadConnection; the
	// selector relies on it being non-nil.
	parsed *ConnectionData
}

// ConnectionsLoader abstracts the DB access. The signature matches
// what a real ProviderRepository will provide once PROV-003 is in
// place; for unit tests we pass a closure that returns an in-memory
// slice.
type ConnectionsLoader func(provider string) ([]CandidateConnection, error)

// ErrNoConnections is returned when no accounts exist for a
// provider. This is distinct from "all are rate-limited" — the
// caller should respond with a different error message.
var ErrNoConnections = errors.New("no provider connections")

// LoadConnection parses the raw JSON `data` column into a typed
// ConnectionData. It is exported so repository code can re-use the
// same shape when writing updates back. The function tolerates
// invalid JSON by returning an empty struct — broken rows must not
// block all selections.
//
// modelLock_<model> keys live in BOTH d.ModelLocks (for typed
// access) and d.Extras (so callers iterating over Extras still
// see them — this matches the Node.js code that surfaces the
// full row object).
func LoadConnection(raw string) (*ConnectionData, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return &ConnectionData{}, nil
	}
	var d ConnectionData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return &ConnectionData{}, nil //nolint:nilerr // broken row → empty, log elsewhere
	}
	var raw2 map[string]any
	if err := json.Unmarshal([]byte(raw), &raw2); err == nil {
		d.Extras = map[string]any{}
		for k, v := range raw2 {
			if strings.HasPrefix(k, "modelLock_") {
				if d.ModelLocks == nil {
					d.ModelLocks = map[string]string{}
				}
				if s, ok := v.(string); ok {
					d.ModelLocks[strings.TrimPrefix(k, "modelLock_")] = s
				}
			}
			// Already represented by a typed field — skip to avoid
			// duplication, but always keep the modelLock_* keys in
			// Extras too (see comment above).
			switch k {
			case "authType", "apiKey", "accessToken", "refreshToken", "projectId",
				"providerSpecificData", "lastUsedAt", "consecutiveUseCount":
				continue
			}
			d.Extras[k] = v
		}
	}
	return &d, nil
}

// parse populates the candidate's parsed view once and caches it.
func (c *CandidateConnection) parse() *ConnectionData {
	if c.parsed != nil {
		return c.parsed
	}
	d, _ := LoadConnection(c.Data)
	c.parsed = d
	return d
}

// isModelLocked returns the lock-expiry time for the supplied model
// if the row is currently locked, else the zero time. A row is
// considered locked if it has a modelLock_<model> key whose RFC3339
// timestamp is still in the future.
func (c *CandidateConnection) isModelLocked(model string) (time.Time, bool) {
	if model == "" {
		return time.Time{}, false
	}
	d := c.parse()
	if d == nil || d.ModelLocks == nil {
		return time.Time{}, false
	}
	raw, ok := d.ModelLocks[model]
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	if time.Now().After(t) {
		// Lock expired. Treat as unlocked.
		return time.Time{}, false
	}
	return t, true
}

// isActiveBool reads the row's isActive flag. The default for nil /
// unset is true (matches the existing schema default of 1).
func (c *CandidateConnection) isActiveBool() bool {
	if c.IsActive == nil {
		return true
	}
	return *c.IsActive
}

// SelectCredentials is the top-level entry point. It implements
// both fill-first and round-robin strategies, mirroring
// open-sse/services/auth.js#getProviderCredentials.
//
// excludeIDs is the set of connection IDs the caller has already
// tried (typically the same model in a previous combo attempt).
// model is the specific model the request is for — used to
// filter out per-model rate limits.
//
// The function returns one of:
//
//   - (Credentials, nil)        — a usable account
//   - (nil, RateLimitState)     — every account is locked; the
//     returned value (with .AllRateLimited == true) is also non-nil
//     and embeds the earliest retry timestamp.
//   - (nil, ErrNoConnections)   — no accounts at all for the provider.
//   - (nil, otherErr)           — DB or decode error.
//
// The CredentialsMutation callback, if non-nil, is invoked
// synchronously with the row update payload (lastUsedAt,
// consecutiveUseCount) so the caller can persist it. The function
// is pure: it does not touch the DB itself, it only reports what
// should be written.
func SelectCredentials(
	provider string,
	excludeIDs map[string]struct{},
	model string,
	strategy Strategy,
	stickyLimit int,
	loader ConnectionsLoader,
) (*Credentials, *RateLimitState, *ConnectionData, error) {
	if loader == nil {
		return nil, nil, nil, errors.New("connections loader is nil")
	}
	if stickyLimit <= 0 {
		stickyLimit = 1
	}

	all, err := loader(provider)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load connections: %w", err)
	}
	if len(all) == 0 {
		return nil, nil, nil, ErrNoConnections
	}

	// Step 1 — filter: isActive, not excluded, not model-locked.
	available := make([]CandidateConnection, 0, len(all))
	for _, c := range all {
		if !c.isActiveBool() {
			continue
		}
		if _, ex := excludeIDs[c.ID]; ex {
			continue
		}
		if _, locked := c.isModelLocked(model); locked {
			continue
		}
		available = append(available, c)
	}

	if len(available) == 0 {
		// All accounts are unavailable. Build the rate-limit
		// response with the earliest lock expiry.
		state := buildRateLimitState(all, model)
		if state != nil {
			return nil, state, nil, nil
		}
		return nil, nil, nil, nil
	}

	// Step 2 — sort: priority ASC, then ID for determinism.
	sort.SliceStable(available, func(i, j int) bool {
		pi := priorityOrMax(available[i].Priority)
		pj := priorityOrMax(available[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return available[i].ID < available[j].ID
	})

	// Step 3 — strategy-specific selection.
	var chosen *CandidateConnection
	var update *ConnectionData
	switch strategy {
	case StrategyRoundRobin:
		chosen, update = pickRoundRobin(available, stickyLimit)
	default:
		// fill-first (default)
		first := available[0]
		chosen = &first
		// No state to update on a fill-first pick — we just take
		// the top of the sorted list.
	}

	if chosen == nil {
		// Defensive: if round-robin selected nil, fall back to
		// the first available row.
		first := available[0]
		chosen = &first
	}

	creds := credentialsFromCandidate(chosen)
	return creds, nil, update, nil
}

// priorityOrMax returns the row's priority, defaulting to 999 (lowest
// possible) when nil. This matches the JS `c.priority || 999` idiom.
func priorityOrMax(p *int) int {
	if p == nil {
		return 999
	}
	return *p
}

// pickRoundRobin implements the JS round-robin branch from
// auth.js#getProviderCredentials. It reads the most-recently-used
// account (if any) and either stays on it (when its
// consecutiveUseCount < stickyLimit) or rotates to the
// least-recently-used candidate.
func pickRoundRobin(available []CandidateConnection, stickyLimit int) (*CandidateConnection, *ConnectionData) {
	// Sort: most-recently-used first (so byRecency[0] is "current").
	byRecency := append([]CandidateConnection(nil), available...)
	sort.SliceStable(byRecency, func(i, j int) bool {
		li, lj := byRecency[i].parse(), byRecency[j].parse()
		ai, bi := li.LastUsedAt, lj.LastUsedAt
		switch {
		case ai == nil && bi == nil:
			return priorityOrMax(byRecency[i].Priority) < priorityOrMax(byRecency[j].Priority)
		case ai == nil:
			return false // nil lastUsedAt sorts LAST (so it won't be the "current")
		case bi == nil:
			return true
		default:
			return ai.After(*bi)
		}
	})

	current := byRecency[0]
	currentData := current.parse()
	currentCount := 0
	if currentData != nil {
		currentCount = currentData.ConsecutiveUseCount
	}

	if currentData != nil && currentData.LastUsedAt != nil && currentCount < stickyLimit {
		// Stay on the current account. Increment its count and
		// refresh lastUsedAt.
		now := time.Now().UTC()
		upd := *currentData
		upd.LastUsedAt = &now
		upd.ConsecutiveUseCount = currentCount + 1
		return &current, &upd
	}

	// Rotate: pick the least-recently-used. The JS code excludes
	// the current account from the rotation candidates, but only
	// when there are ≥ 2 connections. We mirror that.
	byOldest := append([]CandidateConnection(nil), available...)
	sort.SliceStable(byOldest, func(i, j int) bool {
		li, lj := byOldest[i].parse(), byOldest[j].parse()
		ai, bi := li.LastUsedAt, lj.LastUsedAt
		switch {
		case ai == nil && bi == nil:
			return priorityOrMax(byOldest[i].Priority) < priorityOrMax(byOldest[j].Priority)
		case ai == nil:
			return true
		case bi == nil:
			return false
		default:
			return ai.Before(*bi)
		}
	})

	pick := byOldest[0]
	upd := *pick.parse()
	now := time.Now().UTC()
	upd.LastUsedAt = &now
	upd.ConsecutiveUseCount = 1
	return &pick, &upd
}

// buildRateLimitState inspects every account and computes the
// earliest per-model lock expiry. Returns nil if nothing is locked
// (i.e. all accounts were unavailable for a different reason).
func buildRateLimitState(all []CandidateConnection, model string) *RateLimitState {
	if model == "" {
		return nil
	}
	var earliest time.Time
	var lastErr, lastErrCode string
	for i := range all {
		t, locked := all[i].isModelLocked(model)
		if !locked {
			continue
		}
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
		// Capture error context from the first locked row, so the
		// caller can log it.
		if lastErr == "" {
			d := all[i].parse()
			if d != nil {
				if s, ok := d.Extras["lastError"].(string); ok {
					lastErr = s
				}
				if s, ok := d.Extras["errorCode"].(string); ok {
					lastErrCode = s
				}
			}
		}
	}
	if earliest.IsZero() {
		return nil
	}
	return &RateLimitState{
		AllRateLimited:  true,
		RetryAfter:      earliest,
		RetryAfterHuman: formatRetryAfter(earliest),
		LastError:       lastErr,
		LastErrorCode:   lastErrCode,
	}
}

// formatRetryAfter returns a human-friendly description of how long
// until the supplied time. Mirrors open-sse/services/accountFallback
// .js#formatRetryAfter for the values the Node.js layer emits.
func formatRetryAfter(when time.Time) string {
	d := time.Until(when)
	if d <= 0 {
		return "now"
	}
	// Round to whole seconds so the human format matches the JS
	// output even when the input timestamp falls a few ms short of
	// the expected value (e.g. 1m59.999s should still print as 1m).
	seconds := int((d + 500*time.Millisecond) / time.Second)
	if seconds <= 0 {
		return "now"
	}
	switch {
	case seconds < 60:
		return fmt.Sprintf("reset after %ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("reset after %dm", seconds/60)
	case seconds < 86400:
		h := seconds / 3600
		m := (seconds % 3600) / 60
		return fmt.Sprintf("reset after %dh%dm", h, m)
	default:
		return fmt.Sprintf("reset after %dd", seconds/86400)
	}
}

// credentialsFromCandidate projects a candidate row into the
// executor-facing Credentials struct.
func credentialsFromCandidate(c *CandidateConnection) *Credentials {
	if c == nil {
		return nil
	}
	d := c.parse()
	if d == nil {
		return &Credentials{ConnectionID: c.ID, ConnectionName: c.ID}
	}
	return &Credentials{
		ConnectionID:          c.ID,
		ConnectionName:        pickName(c, d),
		AuthType:              d.AuthType,
		APIKey:                d.APIKey,
		AccessToken:           d.AccessToken,
		RefreshToken:          d.RefreshToken,
		ProjectID:             d.ProjectID,
		ProviderSpecificData:  d.ProviderSpecificData,
	}
}

// pickName selects the best human-readable name for the connection
// (displayName > name > email > id), matching the JS preference
// order in auth.js#getProviderCredentials.
func pickName(c *CandidateConnection, d *ConnectionData) string {
	if s, ok := d.Extras["displayName"].(string); ok && s != "" {
		return s
	}
	if c.Name != "" {
		return c.Name
	}
	if c.Email != "" {
		return c.Email
	}
	return c.ID
}
