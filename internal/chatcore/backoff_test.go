package chatcore

import (
	"testing"
	"time"
)

// TestCheckFallbackError_429 — AC-001: 429 returns shouldFallback=true
// and a non-zero cooldown.
func TestCheckFallbackError_429(t *testing.T) {
	d := CheckFallbackError(429, "rate limit exceeded", 0)
	if !d.ShouldFallback {
		t.Errorf("ShouldFallback = false, want true")
	}
	if d.CooldownMs <= 0 {
		t.Errorf("CooldownMs = %d, want > 0", d.CooldownMs)
	}
	if d.NewBackoffLevel != 1 {
		t.Errorf("NewBackoffLevel = %d, want 1 (first 429)", d.NewBackoffLevel)
	}
}

// TestCheckFallbackError_429_LevelIncreases — repeated 429s grow the
// cooldown exponentially.
func TestCheckFallbackError_429_LevelIncreases(t *testing.T) {
	d1 := CheckFallbackError(429, "rate limit", 0) // level 0 → +1
	d2 := CheckFallbackError(429, "rate limit", 1) // level 1 → +1
	d3 := CheckFallbackError(429, "rate limit", 2) // level 2 → +1
	if d2.CooldownMs <= d1.CooldownMs {
		t.Errorf("expected growth: d1=%d d2=%d", d1.CooldownMs, d2.CooldownMs)
	}
	if d3.CooldownMs <= d2.CooldownMs {
		t.Errorf("expected growth: d2=%d d3=%d", d2.CooldownMs, d3.CooldownMs)
	}
	if d2.NewBackoffLevel != 2 || d3.NewBackoffLevel != 3 {
		t.Errorf("levels: d2=%d (want 2), d3=%d (want 3)", d2.NewBackoffLevel, d3.NewBackoffLevel)
	}
}

// TestCheckFallbackError_429_LevelCapped — MaxLevel stops growth.
func TestCheckFallbackError_429_LevelCapped(t *testing.T) {
	d := CheckFallbackError(429, "rate limit", DefaultBackoff.MaxLevel)
	if d.NewBackoffLevel != DefaultBackoff.MaxLevel {
		t.Errorf("level = %d, want capped at %d", d.NewBackoffLevel, DefaultBackoff.MaxLevel)
	}
}

// TestCheckFallbackError_400 — AC: 400 returns shouldFallback=false.
func TestCheckFallbackError_400(t *testing.T) {
	d := CheckFallbackError(400, "bad request body", 5)
	if d.ShouldFallback {
		t.Errorf("400 should NOT fall back, got %+v", d)
	}
	if d.CooldownMs != 0 {
		t.Errorf("400 cooldown = %d, want 0", d.CooldownMs)
	}
}

// TestCheckFallbackError_404 — 404 (model not found) is a client
// error and must not fall back.
func TestCheckFallbackError_404(t *testing.T) {
	d := CheckFallbackError(404, "model not found", 5)
	if d.ShouldFallback {
		t.Errorf("404 should NOT fall back, got %+v", d)
	}
}

// TestCheckFallbackError_500 — AC: 5xx returns shouldFallback=true.
func TestCheckFallbackError_500(t *testing.T) {
	for _, s := range []int{500, 502, 503, 504} {
		d := CheckFallbackError(s, "upstream down", 0)
		if !d.ShouldFallback {
			t.Errorf("status %d: ShouldFallback = false, want true", s)
		}
	}
}

// TestCheckFallbackError_401_403 — auth errors fall back (so the
// caller rotates to a non-broken account) but with a transient
// cooldown.
func TestCheckFallbackError_401_403(t *testing.T) {
	for _, s := range []int{401, 403} {
		d := CheckFallbackError(s, "auth failed", 0)
		if !d.ShouldFallback {
			t.Errorf("status %d: ShouldFallback = false, want true (auth broken — try next account)", s)
		}
		if d.CooldownMs != TransientCooldownMs {
			t.Errorf("status %d: cooldown = %d, want %d (transient)", s, d.CooldownMs, TransientCooldownMs)
		}
	}
}

// TestCheckFallbackError_TextRule — explicit "rate limit" in the
// body wins even when the status is unusual (some providers return
// 200/4xx with a rate-limit body).
func TestCheckFallbackError_TextRule(t *testing.T) {
	d := CheckFallbackError(418, "Rate Limit hit, try later", 0)
	if !d.ShouldFallback {
		t.Errorf("text rule: ShouldFallback = false, want true")
	}
	if d.NewBackoffLevel != 1 {
		t.Errorf("text rule: level = %d, want 1", d.NewBackoffLevel)
	}
}

// TestCheckFallbackError_DefaultTransient — unknown status with no
// matching text → transient cooldown.
func TestCheckFallbackError_DefaultTransient(t *testing.T) {
	d := CheckFallbackError(418, "I'm a teapot", 0)
	if !d.ShouldFallback {
		t.Errorf("default: ShouldFallback = false, want true")
	}
	if d.CooldownMs != TransientCooldownMs {
		t.Errorf("default: cooldown = %d, want %d", d.CooldownMs, TransientCooldownMs)
	}
}

// TestBackoffCooldown_Growth — sanity check the exponential table.
func TestBackoffCooldown_Growth(t *testing.T) {
	cases := []struct {
		level int
		want  time.Duration
	}{
		{0, 2 * time.Second},
		{1, 2 * time.Second}, // base * 2^0
		{2, 4 * time.Second}, // base * 2^1
		{3, 8 * time.Second},
		{4, 16 * time.Second},
	}
	for _, c := range cases {
		got := backoffCooldown(c.level)
		if got != c.want {
			t.Errorf("level %d = %v, want %v", c.level, got, c.want)
		}
	}
}

// TestBackoffCooldown_Cap — large levels are capped at Max.
func TestBackoffCooldown_Cap(t *testing.T) {
	got := backoffCooldown(50)
	if got != DefaultBackoff.Max {
		t.Errorf("level 50 = %v, want max %v", got, DefaultBackoff.Max)
	}
}

// TestComputeLockExpiry_Precise — AC: resetsAtMs in the future
// yields that delta (capped at MaxRateLimitCooldownMs).
func TestComputeLockExpiry_Precise(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	reset := nowMs + 60_000 // 60s
	expiry := ComputeLockExpiry(reset, 0)
	want := time.UnixMilli(nowMs + 60_000).UTC()
	if !expiry.Equal(want) {
		t.Errorf("expiry = %v, want %v", expiry, want)
	}
}

// TestComputeLockExpiry_Cap — resetsAtMs > MaxRateLimitCooldownMs in
// the future is capped.
func TestComputeLockExpiry_Cap(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	reset := nowMs + int64(MaxRateLimitCooldownMs) + 60*60*1000 // way over cap
	expiry := ComputeLockExpiry(reset, 0)
	want := time.UnixMilli(nowMs + int64(MaxRateLimitCooldownMs)).UTC()
	if !expiry.Equal(want) {
		t.Errorf("expiry = %v, want %v (capped at MaxRateLimitCooldownMs)", expiry, want)
	}
}

// TestComputeLockExpiry_Fallback — when resetsAtMs is in the past,
// the supplied cooldownMs is used.
func TestComputeLockExpiry_Fallback(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	expiry := ComputeLockExpiry(nowMs-1000, 5000)
	want := time.UnixMilli(nowMs + 5000).UTC()
	if !expiry.Equal(want) {
		t.Errorf("expiry = %v, want %v (used fallback cooldown)", expiry, want)
	}
}

// TestBuildModelLockKey — the canonical "modelLock_<model>" form.
func TestBuildModelLockKey(t *testing.T) {
	if got := BuildModelLockKey("gpt-4"); got != "modelLock_gpt-4" {
		t.Errorf("key = %q, want modelLock_gpt-4", got)
	}
	if got := BuildModelLockKey(""); got != "modelLock___all" {
		t.Errorf("empty model key = %q, want modelLock___all", got)
	}
}

// TestMarkUnavailable_NoAuth — noAuth connection is a no-op.
func TestMarkUnavailable_NoAuth(t *testing.T) {
	conn := &CandidateConnection{ID: "noauth", Data: "{}"}
	out := MarkUnavailable(conn, 429, "rate limit", "openai", "gpt-4", 0)
	if out.ShouldFallback {
		t.Errorf("noAuth should be no-op, got %+v", out)
	}
	if out.Updated != nil {
		t.Errorf("noAuth should not return updated data, got %+v", out.Updated)
	}
}

// TestMarkUnavailable_429 — 429 writes the model lock and returns
// the cooldown.
func TestMarkUnavailable_429(t *testing.T) {
	conn := &CandidateConnection{
		ID:       "conn-1",
		Provider: "openai",
		Data:     `{"authType":"apikey","apiKey":"sk-1"}`,
	}
	out := MarkUnavailable(conn, 429, "rate limit", "openai", "gpt-4", 0)
	if !out.ShouldFallback {
		t.Errorf("ShouldFallback = false, want true")
	}
	if out.CooldownMs <= 0 {
		t.Errorf("CooldownMs = %d, want > 0", out.CooldownMs)
	}
	if out.Updated == nil {
		t.Fatal("Updated is nil")
	}
	if out.Updated.Extras["testStatus"] != "unavailable" {
		t.Errorf("testStatus = %v, want unavailable", out.Updated.Extras["testStatus"])
	}
	if out.Updated.Extras["lastError"] != "rate limit" {
		t.Errorf("lastError = %v", out.Updated.Extras["lastError"])
	}
	lockKey := BuildModelLockKey("gpt-4")
	if _, ok := out.Updated.Extras[lockKey].(string); !ok {
		t.Errorf("expected %q to be set, got %v", lockKey, out.Updated.Extras[lockKey])
	}
	if out.LockKey != lockKey {
		t.Errorf("LockKey = %q, want %q", out.LockKey, lockKey)
	}
}

// TestMarkUnavailable_ResetsAtMs — provider-supplied resetsAtMs
// wins over heuristic backoff.
func TestMarkUnavailable_ResetsAtMs(t *testing.T) {
	conn := &CandidateConnection{ID: "conn-1", Data: `{"authType":"apikey","apiKey":"sk-1"}`}
	future := time.Now().Add(90 * time.Second).UnixMilli()
	out := MarkUnavailable(conn, 429, "limit hit", "openai", "gpt-4", future)
	if !out.ShouldFallback {
		t.Errorf("ShouldFallback = false, want true")
	}
	// 90s is under MaxRateLimitCooldownMs, so the cooldown should
	// reflect the precise reset.
	if out.CooldownMs < 80_000 || out.CooldownMs > 100_000 {
		t.Errorf("CooldownMs = %d, want ~90000 (precise reset)", out.CooldownMs)
	}
	if out.Updated == nil {
		t.Fatal("Updated is nil")
	}
	if v, ok := out.Updated.Extras["backoffLevel"].(int); !ok || v != 0 {
		// The JS code resets backoffLevel to 0 when precise
		// resetsAtMs is provided.
		t.Errorf("backoffLevel = %v, want 0 (reset by precise reset)", out.Updated.Extras["backoffLevel"])
	}
}

// TestMarkUnavailable_400 — client error: no fallback, no write.
func TestMarkUnavailable_400(t *testing.T) {
	conn := &CandidateConnection{ID: "conn-1", Data: `{"authType":"apikey","apiKey":"sk-1"}`}
	out := MarkUnavailable(conn, 400, "bad model", "openai", "gpt-4", 0)
	if out.ShouldFallback {
		t.Errorf("400 should not fall back, got %+v", out)
	}
	if out.Updated != nil {
		t.Errorf("400 should not produce an update, got %+v", out.Updated)
	}
}

// TestClearAccountError_SingleLock — the current model's lock is
// removed and error state is reset (no other locks remain).
func TestClearAccountError_SingleLock(t *testing.T) {
	future := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	conn := &CandidateConnection{
		ID: "conn-1",
		Data: dataJSON(map[string]any{
			"authType":         "apikey",
			"apiKey":           "sk-1",
			"modelLock_gpt-4":  future,
			"testStatus":       "unavailable",
			"lastError":        "rate limit",
			"backoffLevel":     3,
		}),
	}
	upd := ClearAccountError(conn, "gpt-4")
	if upd == nil {
		t.Fatal("expected update, got nil")
	}
	if _, present := upd.Extras["modelLock_gpt-4"]; present {
		// It is set to nil, but the key should be present with a nil value.
		if v, ok := upd.Extras["modelLock_gpt-4"]; ok && v != nil {
			t.Errorf("modelLock_gpt-4 = %v, want cleared", v)
		}
	}
	if upd.Extras["testStatus"] != "active" {
		t.Errorf("testStatus = %v, want active", upd.Extras["testStatus"])
	}
	if _, ok := upd.Extras["lastError"]; ok {
		if v := upd.Extras["lastError"]; v != nil {
			t.Errorf("lastError = %v, want cleared", v)
		}
	}
	if v, ok := upd.Extras["backoffLevel"].(int); !ok || v != 0 {
		t.Errorf("backoffLevel = %v, want 0", upd.Extras["backoffLevel"])
	}
}

// TestClearAccountError_MultipleLocks — only the current model's lock
// is removed; another active lock survives and the error state is
// preserved.
func TestClearAccountError_MultipleLocks(t *testing.T) {
	future := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	conn := &CandidateConnection{
		ID: "conn-1",
		Data: dataJSON(map[string]any{
			"authType":           "apikey",
			"apiKey":             "sk-1",
			"modelLock_gpt-4":    future,
			"modelLock_claude-3": future,
			"testStatus":         "unavailable",
			"lastError":          "rate limit",
		}),
	}
	upd := ClearAccountError(conn, "gpt-4")
	if upd == nil {
		t.Fatal("expected update, got nil")
	}
	// gpt-4 lock cleared.
	if v, ok := upd.Extras["modelLock_gpt-4"]; ok && v != nil {
		t.Errorf("modelLock_gpt-4 = %v, want cleared", v)
	}
	// claude-3 lock survives.
	if _, ok := upd.Extras["modelLock_claude-3"].(string); !ok {
		t.Errorf("modelLock_claude-3 = %v, want present", upd.Extras["modelLock_claude-3"])
	}
	// testStatus must remain "unavailable" — there is still an
	// active lock.
	if upd.Extras["testStatus"] != "unavailable" {
		t.Errorf("testStatus = %v, want unavailable (active lock remains)", upd.Extras["testStatus"])
	}
}

// TestClearAccountError_ExpiredLockLazyClean — locks that are in
// the past are removed even if the caller did not name them
// explicitly.
func TestClearAccountError_ExpiredLockLazyClean(t *testing.T) {
	expired := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	future := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	conn := &CandidateConnection{
		ID: "conn-1",
		Data: dataJSON(map[string]any{
			"authType":           "apikey",
			"apiKey":             "sk-1",
			"modelLock_old":      expired,
			"modelLock_gpt-4":    future,
			"testStatus":         "unavailable",
			"lastError":          "rate limit",
		}),
	}
	upd := ClearAccountError(conn, "")
	if upd == nil {
		t.Fatal("expected update, got nil")
	}
	// Old lock cleared (expired).
	if v, ok := upd.Extras["modelLock_old"]; ok && v != nil {
		t.Errorf("modelLock_old = %v, want cleared (expired)", v)
	}
	// Active lock remains.
	if _, ok := upd.Extras["modelLock_gpt-4"].(string); !ok {
		t.Errorf("modelLock_gpt-4 = %v, want present", upd.Extras["modelLock_gpt-4"])
	}
}

// TestClearAccountError_NoAuth — no-op for noauth.
func TestClearAccountError_NoAuth(t *testing.T) {
	conn := &CandidateConnection{ID: "noauth", Data: "{}"}
	if upd := ClearAccountError(conn, "gpt-4"); upd != nil {
		t.Errorf("noAuth should be no-op, got %+v", upd)
	}
}

// TestClearAccountError_AlreadyClean — short-circuits when there
// is nothing to clear.
func TestClearAccountError_AlreadyClean(t *testing.T) {
	conn := &CandidateConnection{ID: "conn-1", Data: `{"authType":"apikey","apiKey":"sk-1"}`}
	if upd := ClearAccountError(conn, "gpt-4"); upd != nil {
		t.Errorf("clean row should short-circuit, got %+v", upd)
	}
}
