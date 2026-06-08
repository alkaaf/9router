package cloudflare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthConfig_Default(t *testing.T) {
	c := DefaultHealthConfig()
	if c.DNSTimeout != 2*time.Second {
		t.Fatalf("DNSTimeout = %v, want 2s", c.DNSTimeout)
	}
	if c.HTTPTimeout != 5*time.Second {
		t.Fatalf("HTTPTimeout = %v, want 5s", c.HTTPTimeout)
	}
	if c.TotalWait != 60*time.Second {
		t.Fatalf("TotalWait = %v, want 60s", c.TotalWait)
	}
	if c.Interval != 2*time.Second {
		t.Fatalf("Interval = %v, want 2s", c.Interval)
	}
	if len(c.Resolvers) != 2 {
		t.Fatalf("Resolvers length = %d, want 2", len(c.Resolvers))
	}
}

func TestHealthConfig_FillDefaults(t *testing.T) {
	c := HealthConfig{}.fillDefaults()
	d := DefaultHealthConfig()
	if c.DNSTimeout != d.DNSTimeout || c.HTTPTimeout != d.HTTPTimeout ||
		c.TotalWait != d.TotalWait || c.Interval != d.Interval {
		t.Fatalf("zero-valued config not filled: %+v", c)
	}
	if len(c.Resolvers) != len(d.Resolvers) {
		t.Fatalf("Resolvers not filled")
	}
}

func TestProbeURLAlive_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("path = %q, want /api/health", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ok, err := ProbeURLAlive(context.Background(), srv.URL, HealthConfig{HTTPTimeout: time.Second})
	if err != nil {
		t.Fatalf("ProbeURLAlive: %v", err)
	}
	if !ok {
		t.Fatalf("expected alive=true for 200 response")
	}
}

func TestProbeURLAlive_AppendsHealthPath(t *testing.T) {
	var hitPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := ProbeURLAlive(context.Background(), srv.URL+"/some/base", HealthConfig{HTTPTimeout: time.Second}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if hitPath != "/some/base/api/health" {
		t.Fatalf("hitPath = %q, want /some/base/api/health", hitPath)
	}
}

func TestProbeURLAlive_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	ok, err := ProbeURLAlive(context.Background(), srv.URL, HealthConfig{HTTPTimeout: time.Second})
	if err != nil {
		t.Fatalf("ProbeURLAlive: %v", err)
	}
	if ok {
		t.Fatalf("expected alive=false for 503")
	}
}

func TestProbeURLAlive_Timeout(t *testing.T) {
	// Listener that accepts but never responds.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ok, err := ProbeURLAlive(context.Background(), srv.URL, HealthConfig{HTTPTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("ProbeURLAlive: %v", err)
	}
	if ok {
		t.Fatalf("expected alive=false on timeout")
	}
}

func TestProbeURLAlive_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	ok, err := ProbeURLAlive(ctx, srv.URL, HealthConfig{HTTPTimeout: 5 * time.Second})
	if err == nil {
		t.Fatalf("expected error from cancelled ctx")
	}
	if ok {
		t.Fatalf("expected alive=false")
	}
}

func TestWaitForHealth_ImmediateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	err := WaitForHealth(context.Background(), srv.URL, HealthConfig{
		HTTPTimeout: time.Second, TotalWait: 5 * time.Second, Interval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WaitForHealth: %v", err)
	}
}

func TestWaitForHealth_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := WaitForHealth(ctx, srv.URL, HealthConfig{
		HTTPTimeout: 100 * time.Millisecond, TotalWait: 5 * time.Second, Interval: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatalf("expected error from cancelled context")
	}
}

func TestWaitForHealth_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := WaitForHealth(context.Background(), srv.URL, HealthConfig{
		HTTPTimeout: 100 * time.Millisecond, TotalWait: 200 * time.Millisecond, Interval: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestWaitForHealth_EventuallyHealthy(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := WaitForHealth(context.Background(), srv.URL, HealthConfig{
		HTTPTimeout: 500 * time.Millisecond, TotalWait: 5 * time.Second, Interval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WaitForHealth should succeed once server returns 200: %v", err)
	}
}

func TestResolveHostname_NoResolvers(t *testing.T) {
	// Should still work with default resolvers on a real name.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ips, err := ResolveHostname(ctx, "localhost", nil)
	if err != nil {
		t.Skipf("ResolveHostname against localhost failed (likely no DNS): %v", err)
	}
	if len(ips) == 0 {
		t.Fatalf("expected at least one IP for localhost")
	}
}

func TestResolveHostname_Fallback(t *testing.T) {
	// 127.0.0.1:1 is a closed port — every resolver should fail.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := ResolveHostname(ctx, "example.invalid", []string{"127.0.0.1:1", "127.0.0.1:2"})
	if err == nil {
		t.Fatalf("expected resolution error")
	}
}
