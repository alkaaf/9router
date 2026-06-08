package chatcore

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// ptr is a small helper to build *int / *bool literals inline.
func ptr[T any](v T) *T { return &v }

// candidates builds CandidateConnection rows from a compact form
// (id, priority, isActive, data) to keep the test setup readable.
type cand struct {
	ID       string
	Priority *int
	Active   *bool
	Data     string
}

func toCandidates(in []cand) []CandidateConnection {
	out := make([]CandidateConnection, 0, len(in))
	for _, c := range in {
		row := CandidateConnection{
			ID:       c.ID,
			Provider: "openai",
			Name:     c.ID,
			Priority: c.Priority,
			IsActive: c.Active,
			Data:     c.Data,
		}
		out = append(out, row)
	}
	return out
}

// loaderFor wraps a slice into a ConnectionsLoader.
func loaderFor(rows []CandidateConnection) ConnectionsLoader {
	return func(provider string) ([]CandidateConnection, error) {
		return rows, nil
	}
}

// dataJSON is a tiny helper that returns a JSON-encoded
// providerConnections.data value. The variadic fields are written
// verbatim into the object.
func dataJSON(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	return string(b)
}

// TestFillFirst_SingleAccount — basic happy path.
func TestFillFirst_SingleAccount(t *testing.T) {
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-a"})},
	})
	creds, rl, _, err := SelectCredentials("openai", nil, "", StrategyFillFirst, 1, loaderFor(rows))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if rl != nil {
		t.Fatalf("unexpected rate-limit state: %+v", rl)
	}
	if creds == nil || creds.ConnectionID != "a" {
		t.Errorf("got %+v, want connection a", creds)
	}
}

// TestFillFirst_PicksHighestPriority — AC-001: lowest priority wins.
func TestFillFirst_PicksHighestPriority(t *testing.T) {
	rows := toCandidates([]cand{
		{ID: "low", Priority: ptr(99), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-low"})},
		{ID: "mid", Priority: ptr(50), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-mid"})},
		{ID: "high", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-high"})},
	})
	creds, _, _, _ := SelectCredentials("openai", nil, "", StrategyFillFirst, 1, loaderFor(rows))
	if creds.ConnectionID != "high" {
		t.Errorf("got %s, want high (priority 1)", creds.ConnectionID)
	}
}

// TestFillFirst_RespectsExcludeIDs — AC-002: excluded IDs are skipped.
func TestFillFirst_RespectsExcludeIDs(t *testing.T) {
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-a"})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, _, _ := SelectCredentials("openai", map[string]struct{}{"a": {}}, "", StrategyFillFirst, 1, loaderFor(rows))
	if creds.ConnectionID != "b" {
		t.Errorf("got %s, want b (a excluded)", creds.ConnectionID)
	}
}

// TestFillFirst_FiltersModelLocked — AC-003: model-locked accounts
// are excluded.
func TestFillFirst_FiltersModelLocked(t *testing.T) {
	locked := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{
			"authType":       "apikey",
			"apiKey":         "sk-a",
			"modelLock_gpt4": locked,
		})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, _, _ := SelectCredentials("openai", nil, "gpt4", StrategyFillFirst, 1, loaderFor(rows))
	if creds.ConnectionID != "b" {
		t.Errorf("got %s, want b (a is model-locked)", creds.ConnectionID)
	}
}

// TestFillFirst_NoAccounts — AC-004: empty provider list.
func TestFillFirst_NoAccounts(t *testing.T) {
	_, _, _, err := SelectCredentials("openai", nil, "", StrategyFillFirst, 1, loaderFor(nil))
	if !errors.Is(err, ErrNoConnections) {
		t.Errorf("got err=%v, want ErrNoConnections", err)
	}
}

// TestFillFirst_AllExcluded — variant of AC-005.
func TestFillFirst_AllExcluded(t *testing.T) {
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-a"})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, _, err := SelectCredentials("openai", map[string]struct{}{"a": {}, "b": {}}, "", StrategyFillFirst, 1, loaderFor(rows))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if creds != nil {
		t.Errorf("all excluded should return nil creds, got %+v", creds)
	}
}

// TestFillFirst_AllRateLimited — AC-005: every account locked for
// the model → returns RateLimitState with the earliest expiry.
func TestFillFirst_AllRateLimited(t *testing.T) {
	earlier := time.Now().Add(1 * time.Minute).UTC().Format(time.RFC3339)
	later := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{
			"authType":       "apikey",
			"apiKey":         "sk-a",
			"modelLock_gpt4": earlier,
		})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{
			"authType":       "apikey",
			"apiKey":         "sk-b",
			"modelLock_gpt4": later,
		})},
	})
	creds, rl, _, _ := SelectCredentials("openai", nil, "gpt4", StrategyFillFirst, 1, loaderFor(rows))
	if creds != nil {
		t.Errorf("all locked should return nil creds, got %+v", creds)
	}
	if rl == nil {
		t.Fatal("expected RateLimitState, got nil")
	}
	if !rl.AllRateLimited {
		t.Errorf("AllRateLimited = false, want true")
	}
	// Earliest expiry is the "earlier" timestamp.
	if !rl.RetryAfter.Equal(parseTime(t, earlier)) {
		t.Errorf("RetryAfter = %v, want %v", rl.RetryAfter, earlier)
	}
}

// TestFillFirst_SamePriority_StableID — AC variant: ties broken by
// ID for determinism.
func TestFillFirst_SamePriority_StableID(t *testing.T) {
	rows := toCandidates([]cand{
		{ID: "z", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-z"})},
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-a"})},
		{ID: "m", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-m"})},
	})
	creds, _, _, _ := SelectCredentials("openai", nil, "", StrategyFillFirst, 1, loaderFor(rows))
	if creds.ConnectionID != "a" {
		t.Errorf("got %s, want a (lowest ID on tie)", creds.ConnectionID)
	}
}

// TestFillFirst_InactiveSkipped — defensive: isActive=false row skipped.
func TestFillFirst_InactiveSkipped(t *testing.T) {
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(false), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-a"})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, _, _ := SelectCredentials("openai", nil, "", StrategyFillFirst, 1, loaderFor(rows))
	if creds.ConnectionID != "b" {
		t.Errorf("got %s, want b (a is inactive)", creds.ConnectionID)
	}
}

// TestRoundRobin_StaysUnderLimit — AC: when current account's
// consecutiveUseCount < stickyLimit, the same account is returned
// and the count is incremented.
func TestRoundRobin_StaysUnderLimit(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{
			"authType":            "apikey",
			"apiKey":              "sk-a",
			"lastUsedAt":          past,
			"consecutiveUseCount": 2,
		})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, upd, _ := SelectCredentials("openai", nil, "", StrategyRoundRobin, 5, loaderFor(rows))
	if creds.ConnectionID != "a" {
		t.Errorf("got %s, want a (under sticky limit)", creds.ConnectionID)
	}
	if upd == nil || upd.ConsecutiveUseCount != 3 {
		t.Errorf("update count = %v, want 3", upd)
	}
}

// TestRoundRobin_RotatesAtLimit — when count == stickyLimit, the
// selector rotates to the next account and resets the count to 1.
func TestRoundRobin_RotatesAtLimit(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{
			"authType":            "apikey",
			"apiKey":              "sk-a",
			"lastUsedAt":          past,
			"consecutiveUseCount": 5,
		})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, upd, _ := SelectCredentials("openai", nil, "", StrategyRoundRobin, 5, loaderFor(rows))
	if creds.ConnectionID != "b" {
		t.Errorf("got %s, want b (rotated off a)", creds.ConnectionID)
	}
	if upd == nil || upd.ConsecutiveUseCount != 1 {
		t.Errorf("update count = %v, want 1 (reset)", upd)
	}
}

// TestRoundRobin_NoLastUsed_PicksByPriority — AC-001: when nothing
// has been used, fall back to priority order.
func TestRoundRobin_NoLastUsed_PicksByPriority(t *testing.T) {
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-a"})},
		{ID: "b", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, _, _ := SelectCredentials("openai", nil, "", StrategyRoundRobin, 3, loaderFor(rows))
	if creds.ConnectionID != "b" {
		t.Errorf("got %s, want b (priority 1)", creds.ConnectionID)
	}
}

// TestRoundRobin_ExcludesModelLocked — AC variant: model locks
// are honoured in round-robin too.
func TestRoundRobin_ExcludesModelLocked(t *testing.T) {
	locked := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	past := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{
			"authType":            "apikey",
			"apiKey":              "sk-a",
			"lastUsedAt":          past,
			"consecutiveUseCount": 1,
			"modelLock_gpt4":      locked,
		})},
		{ID: "b", Priority: ptr(2), Active: ptr(true), Data: dataJSON(map[string]any{"authType": "apikey", "apiKey": "sk-b"})},
	})
	creds, _, _, _ := SelectCredentials("openai", nil, "gpt4", StrategyRoundRobin, 5, loaderFor(rows))
	if creds.ConnectionID != "b" {
		t.Errorf("got %s, want b (a is model-locked)", creds.ConnectionID)
	}
}

// TestRoundRobin_DBPersistsUpdates — the returned update payload
// contains a fresh lastUsedAt and the right count.
func TestRoundRobin_DBPersistsUpdates(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	rows := toCandidates([]cand{
		{ID: "a", Priority: ptr(1), Active: ptr(true), Data: dataJSON(map[string]any{
			"authType":            "apikey",
			"apiKey":              "sk-a",
			"lastUsedAt":          past,
			"consecutiveUseCount": 4,
		})},
	})
	before := time.Now()
	creds, _, upd, _ := SelectCredentials("openai", nil, "", StrategyRoundRobin, 5, loaderFor(rows))
	after := time.Now()

	if creds.ConnectionID != "a" {
		t.Fatalf("got %s, want a", creds.ConnectionID)
	}
	if upd == nil || upd.LastUsedAt == nil {
		t.Fatal("expected non-nil lastUsedAt in update")
	}
	ts := *upd.LastUsedAt
	if ts.Before(before.Add(-time.Second)) || ts.After(after.Add(time.Second)) {
		t.Errorf("lastUsedAt = %v, not in window [%v, %v]", ts, before, after)
	}
	if upd.ConsecutiveUseCount != 5 {
		t.Errorf("count = %d, want 5", upd.ConsecutiveUseCount)
	}
}

// TestLoadConnection_ModelLockDecoding — confirms the modelLock_*
// extraction in LoadConnection.
func TestLoadConnection_ModelLockDecoding(t *testing.T) {
	ts := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	d, err := LoadConnection(dataJSON(map[string]any{
		"authType":            "oauth",
		"accessToken":         "tok",
		"modelLock_gpt-4":     ts,
		"modelLock_claude-3":  ts,
		"lastError":           "429 rate limited",
		"consecutiveUseCount": 7,
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if d.AuthType != "oauth" {
		t.Errorf("authType = %q", d.AuthType)
	}
	if len(d.ModelLocks) != 2 {
		t.Errorf("ModelLocks = %v, want 2 entries", d.ModelLocks)
	}
	if d.Extras["lastError"] != "429 rate limited" {
		t.Errorf("extras.lastError = %v", d.Extras["lastError"])
	}
	if d.ConsecutiveUseCount != 7 {
		t.Errorf("count = %d, want 7", d.ConsecutiveUseCount)
	}
}

// TestLoadConnection_InvalidJSON — broken JSON returns an empty
// struct, not an error. The selector then sees no authType and
// returns a Credentials with empty fields (caller decides what to
// do).
func TestLoadConnection_InvalidJSON(t *testing.T) {
	d, err := LoadConnection("not json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil struct")
	}
}

// TestLoadConnection_Empty — empty string returns zero struct.
func TestLoadConnection_Empty(t *testing.T) {
	d, err := LoadConnection("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.AuthType != "" {
		t.Errorf("authType = %q, want empty", d.AuthType)
	}
}

// TestFormatRetryAfter — table-driven for the human-friendly format.
func TestFormatRetryAfter(t *testing.T) {
	now := time.Now()
	cases := []struct {
		when time.Time
		want string
	}{
		{now.Add(30 * time.Second), "reset after 30s"},
		{now.Add(2 * time.Minute), "reset after 2m"},
		{now.Add(time.Hour + 5*time.Minute), "reset after 1h5m"},
		{now.Add(26 * time.Hour), "reset after 1d"},
		{now.Add(-time.Second), "now"},
	}
	for _, c := range cases {
		if got := formatRetryAfter(c.when); got != c.want {
			t.Errorf("formatRetryAfter(%v) = %q, want %q", c.when, got, c.want)
		}
	}
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return ts
}
