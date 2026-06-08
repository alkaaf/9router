package chatcore

import (
	"fmt"
	"strings"
	"time"
)

// BackoffConfig mirrors open-sse/config/errorConfig.js#BACKOFF_CONFIG.
// Exponential levels: 2s, 4s, 8s, ... capped at 5 minutes and
// maxLevel = 15.
//
// In the JS implementation getQuotaCooldown is called with
// `newLevel` (after incrementing), so a first-time 429 yields
// cooldown = base * 2^(1-1) = 2s. We mirror that.
type BackoffConfig struct {
	Base     time.Duration // 2s
	Max      time.Duration // 5m
	MaxLevel int           // 15
}

// DefaultBackoff is the configuration the production code uses.
// It is a package-level var (not const) so tests can override it.
var DefaultBackoff = BackoffConfig{
	Base:     2 * time.Second,
	Max:      5 * time.Minute,
	MaxLevel: 15,
}

// TransientCooldownMs is the default cooldown for unrecognised
// errors (matches open-sse/config/errorConfig.js#TRANSIENT_COOLDOWN_MS).
const TransientCooldownMs = 30 * 1000

// MaxRateLimitCooldownMs caps provider-reported precise cooldowns
// (e.g. codex resets_at = 5-6h). Matches the JS export.
const MaxRateLimitCooldownMs = 30 * 60 * 1000

// FallbackDecision is the result of evaluating an upstream error.
// shouldFallback is true when the caller should rotate to the next
// account. cooldownMs is the per-model lock duration; the caller
// writes modelLock_<model> = now + cooldownMs.
type FallbackDecision struct {
	ShouldFallback  bool
	CooldownMs      int
	NewBackoffLevel int
}

// backoffCooldown returns the cooldown duration for the supplied
// (already-incremented) backoff level. Mirrors
// getQuotaCooldown: cooldown = base * 2^(level-1), capped at max.
func backoffCooldown(level int) time.Duration {
	if level <= 0 {
		return DefaultBackoff.Base
	}
	lvlShift := level - 1
	if lvlShift < 0 {
		lvlShift = 0
	}
	d := DefaultBackoff.Base
	for i := 0; i < lvlShift; i++ {
		d *= 2
		if d >= DefaultBackoff.Max {
			return DefaultBackoff.Max
		}
	}
	return d
}

// CheckFallbackError is the Go equivalent of
// open-sse/services/accountFallback.js#checkFallbackError. It is
// config-driven: ERROR_RULES (text rules first, then status rules)
// are walked top-to-bottom and the first match wins. Unrecognised
// errors fall through to the transient cooldown.
//
// We inline a small subset of the JS rules — the most common
// status codes — so the package is self-contained. The full table
// can be added later without breaking callers.
//
// The function is pure: it does not touch the DB. The caller is
// responsible for persisting the resulting cooldown and the
// (possibly updated) backoffLevel.
func CheckFallbackError(status int, errorText string, backoffLevel int) FallbackDecision {
	lower := strings.ToLower(errorText)

	// Rule 1 — explicit "rate limit" message wins (some providers
	// return 200/4xx with a rate-limit body).
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate_limit") {
		newLevel := minInt(backoffLevel+1, DefaultBackoff.MaxLevel)
		ms := int(backoffCooldown(newLevel) / time.Millisecond)
		return FallbackDecision{ShouldFallback: true, CooldownMs: ms, NewBackoffLevel: newLevel}
	}

	// Rule 2 — status-based rules.
	switch status {
	case 429:
		newLevel := minInt(backoffLevel+1, DefaultBackoff.MaxLevel)
		ms := int(backoffCooldown(newLevel) / time.Millisecond)
		return FallbackDecision{ShouldFallback: true, CooldownMs: ms, NewBackoffLevel: newLevel}
	case 500, 502, 503, 504:
		// Transient server error — short cooldown, no level bump.
		return FallbackDecision{ShouldFallback: true, CooldownMs: TransientCooldownMs}
	case 400, 404, 422:
		// Client error — do NOT fall back (the model request is
		// malformed; switching accounts will not help).
		return FallbackDecision{ShouldFallback: false, CooldownMs: 0}
	case 401, 403:
		// Auth / permission error. The account is broken; do not
		// retry this model on it. We do fall back to another
		// account but with no lock so the auth-broken account
		// is not needlessly penalised.
		return FallbackDecision{ShouldFallback: true, CooldownMs: TransientCooldownMs}
	}

	// Default — treat as transient, fall back.
	return FallbackDecision{ShouldFallback: true, CooldownMs: TransientCooldownMs}
}

// ComputeLockExpiry returns the absolute UTC time at which a model
// lock should expire. It prefers the precise resetsAtMs (capped at
// MaxRateLimitCooldownMs) when supplied and in the future, and
// falls back to the supplied cooldownMs otherwise.
func ComputeLockExpiry(resetsAtMs int64, cooldownMs int) time.Time {
	nowMs := time.Now().UnixMilli()
	if resetsAtMs > nowMs {
		delta := resetsAtMs - nowMs
		if delta > MaxRateLimitCooldownMs {
			delta = MaxRateLimitCooldownMs
		}
		return time.UnixMilli(nowMs + delta).UTC()
	}
	return time.UnixMilli(nowMs + int64(cooldownMs)).UTC()
}

// BuildModelLockKey returns the JSON key used to persist a per-model
// lock. Mirrors the JS `modelLock_${model}` naming. The "__all"
// sentinel is preserved for account-level locks.
func BuildModelLockKey(model string) string {
	if model == "" {
		return "modelLock___all"
	}
	return "modelLock_" + model
}

// ModelLockKeyRegexp is exported for callers that need to find all
// modelLock_* keys (used by clearAccountError). The check is
// deliberately case-sensitive to match the JS implementation.
func ModelLockKeyRegexp() func(string) bool {
	return func(k string) bool { return strings.HasPrefix(k, "modelLock_") }
}

// MarkUnavailableOutcome is the per-call return value of
// MarkUnavailable. It mirrors the {shouldFallback, cooldownMs} shape
// from open-sse/services/auth.js#markAccountUnavailable.
type MarkUnavailableOutcome struct {
	ShouldFallback bool
	CooldownMs     int
	// Updated is the data-column JSON the caller should write
	// back to the connection row. It includes the new
	// modelLock_<model>, the new backoffLevel, the lastError,
	// and any other side effects. The field is nil when no
	// write is required.
	Updated *ConnectionData
	// LockKey is the JSON column key that was set
	// (e.g. "modelLock_gpt-4"). Empty when no write happened.
	LockKey string
}

// MarkUnavailable mirrors open-sse/services/auth.js#
// markAccountUnavailable. It is pure: it does not touch the DB
// itself, it just computes the new state and returns it for the
// caller to persist.
//
// Inputs:
//
//   - conn:    the current connection (parsed + raw Data).
//   - status:  the upstream HTTP status code.
//   - errText: the upstream error message.
//   - resetsAtMs: precise cooldown timestamp (e.g. from codex
//     resets_at). Pass 0 to use the heuristic.
//
// Returned Updated is a deep copy of the connection data with the
// new modelLock_* and error fields applied. The caller writes it
// back to the DB.
func MarkUnavailable(conn *CandidateConnection, status int, errText string, provider, model string, resetsAtMs int64) MarkUnavailableOutcome {
	if conn == nil {
		return MarkUnavailableOutcome{}
	}
	cid := conn.ID
	if cid == "" || cid == "noauth" {
		// NoAuth virtual connection — nothing to persist.
		return MarkUnavailableOutcome{}
	}

	cur := conn.parse()
	backoffLevel := 0
	if cur != nil {
		backoffLevel = cur.ConsecutiveUseCount // see comment below
	}
	// The JS code uses `conn.backoffLevel`, but the Go model
	// surfaces it under a different field; we accept either.
	if cur != nil {
		if v, ok := cur.Extras["backoffLevel"].(float64); ok {
			backoffLevel = int(v)
		}
	}

	var decision FallbackDecision
	nowMs := time.Now().UnixMilli()
	if resetsAtMs > nowMs {
		delta := resetsAtMs - nowMs
		if delta > MaxRateLimitCooldownMs {
			delta = MaxRateLimitCooldownMs
		}
		decision = FallbackDecision{
			ShouldFallback:  true,
			CooldownMs:      int(delta),
			NewBackoffLevel: 0,
		}
	} else {
		decision = CheckFallbackError(status, errText, backoffLevel)
	}
	if !decision.ShouldFallback {
		return MarkUnavailableOutcome{ShouldFallback: false, CooldownMs: 0}
	}

	expiry := ComputeLockExpiry(resetsAtMs, decision.CooldownMs)
	lockKey := BuildModelLockKey(model)
	reason := errText
	if len(reason) > 100 {
		reason = reason[:100]
	}

	// Build the updated ConnectionData.
	updated := ConnectionData{}
	if cur != nil {
		updated = *cur
	}
	updated.LastUsedAt = cur.LastUsedAt
	updated.ConsecutiveUseCount = 0
	if updated.Extras == nil {
		updated.Extras = map[string]any{}
	}
	updated.Extras[lockKey] = expiry.Format(time.RFC3339)
	updated.Extras["backoffLevel"] = decision.NewBackoffLevel
	updated.Extras["testStatus"] = "unavailable"
	updated.Extras["lastError"] = reason
	updated.Extras["errorCode"] = status
	updated.Extras["lastErrorAt"] = time.Now().UTC().Format(time.RFC3339)

	return MarkUnavailableOutcome{
		ShouldFallback: true,
		CooldownMs:     decision.CooldownMs,
		Updated:        &updated,
		LockKey:        lockKey,
	}
}

// ClearAccountError mirrors open-sse/services/auth.js#
// clearAccountError. It removes the lock for the supplied model,
// lazy-cleans any other expired locks, and resets the row's error
// state to active IF no active locks remain.
//
// The function is pure: it returns the updated ConnectionData for
// the caller to persist.
func ClearAccountError(conn *CandidateConnection, model string) *ConnectionData {
	if conn == nil {
		return nil
	}
	if conn.ID == "" || conn.ID == "noauth" {
		return nil
	}
	cur := conn.parse()
	if cur == nil {
		return nil
	}

	// Quick exit: nothing to clear.
	hasState := false
	if s, ok := cur.Extras["testStatus"].(string); ok && s != "" {
		hasState = true
	}
	if s, ok := cur.Extras["lastError"].(string); ok && s != "" {
		hasState = true
	}
	if !hasState {
		// Look for any modelLock_* keys in Extras.
		isLockKey := ModelLockKeyRegexp()
		for k := range cur.Extras {
			if isLockKey(k) {
				hasState = true
				break
			}
		}
	}
	if !hasState {
		return nil
	}

	updated := *cur
	updated.Extras = make(map[string]any, len(cur.Extras))
	for k, v := range cur.Extras {
		updated.Extras[k] = v
	}
	now := time.Now()

	// Compute the keys to clear: the supplied model, the
	// account-level lock, and any expired locks.
	isLockKey := ModelLockKeyRegexp()
	keysToClear := []string{}
	remainingActive := []string{}
	for k, v := range cur.Extras {
		if !isLockKey(k) {
			continue
		}
		// Parse the expiry.
		var expiry time.Time
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				expiry = t
			}
		}
		expired := !expiry.IsZero() && !expiry.After(now)
		isCurrentModel := model != "" && k == BuildModelLockKey(model)
		isAllLock := k == "modelLock___all"

		if expired || isCurrentModel || isAllLock {
			keysToClear = append(keysToClear, k)
			continue
		}
		if !expiry.IsZero() {
			remainingActive = append(remainingActive, k)
		}
	}

	for _, k := range keysToClear {
		updated.Extras[k] = nil
	}

	// If no active locks remain, reset error state.
	if len(remainingActive) == 0 {
		updated.Extras["testStatus"] = "active"
		updated.Extras["lastError"] = nil
		updated.Extras["lastErrorAt"] = nil
		updated.Extras["backoffLevel"] = 0
	}
	return &updated
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatCooldown is a debug helper, used in tests to render a
// cooldown value as "5s" / "30s" / "1m" / "1h".
func formatCooldown(ms int) string {
	switch {
	case ms >= 3600*1000:
		return fmt.Sprintf("%dh", ms/3600000)
	case ms >= 60*1000:
		return fmt.Sprintf("%dm", ms/60000)
	default:
		return fmt.Sprintf("%ds", ms/1000)
	}
}
