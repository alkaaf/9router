package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTunnelService struct {
	enableRes TunnelResult
	enableErr error
	disableErr error
	enableCalls int
	disableCalls int
}

func (f *fakeTunnelService) Enable(ctx context.Context) (TunnelResult, error) {
	f.enableCalls++
	return f.enableRes, f.enableErr
}

func (f *fakeTunnelService) Disable(ctx context.Context) error {
	f.disableCalls++
	return f.disableErr
}

func (f *fakeTunnelService) GetStatus(ctx context.Context) (TunnelStatus, error) {
	return TunnelStatus{}, nil
}

func TestEnableHandler_Success(t *testing.T) {
	svc := &fakeTunnelService{
		enableRes: TunnelResult{
			TunnelURL: "https://abc.trycloudflare.com",
			ShortID:   "abc",
			PublicURL: "https://rabc.abc-tunnel.us",
		},
	}
	h := &EnableHandler{Service: svc}
	status, body := h.Handle(context.Background())
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	var res EnableResult
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !res.Success || res.TunnelURL != "https://abc.trycloudflare.com" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if svc.enableCalls != 1 {
		t.Fatalf("expected 1 enable call, got %d", svc.enableCalls)
	}
}

func TestEnableHandler_AlreadyRunning(t *testing.T) {
	svc := &fakeTunnelService{
		enableRes: TunnelResult{
			TunnelURL:      "https://abc.trycloudflare.com",
			ShortID:        "abc",
			PublicURL:      "https://rabc.abc-tunnel.us",
			AlreadyRunning: true,
		},
	}
	h := &EnableHandler{Service: svc}
	status, body := h.Handle(context.Background())
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	var res EnableResult
	_ = json.Unmarshal(body, &res)
	if !res.AlreadyRunning {
		t.Fatalf("expected alreadyRunning=true, got %+v", res)
	}
}

func TestEnableHandler_ServiceError(t *testing.T) {
	svc := &fakeTunnelService{enableErr: errors.New("spawn failed")}
	h := &EnableHandler{Service: svc}
	status, body := h.Handle(context.Background())
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
	var err HTTPError
	if jerr := json.Unmarshal(body, &err); jerr != nil {
		t.Fatalf("unmarshal: %v", jerr)
	}
	if err.Error != "spawn failed" {
		t.Fatalf("error msg = %q, want %q", err.Error, "spawn failed")
	}
}

func TestEnableHandler_NilService(t *testing.T) {
	h := &EnableHandler{}
	status, _ := h.Handle(context.Background())
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
}

func TestEnableHandler_ContextCancelPropagates(t *testing.T) {
	svc := &fakeTunnelService{enableErr: context.Canceled}
	h := &EnableHandler{Service: svc}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, body := h.Handle(ctx)
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
	var err HTTPError
	_ = json.Unmarshal(body, &err)
	if err.Error != context.Canceled.Error() {
		t.Fatalf("error msg = %q, want %q", err.Error, context.Canceled.Error())
	}
}

func TestDisableHandler_Success(t *testing.T) {
	svc := &fakeTunnelService{}
	h := &DisableHandler{Service: svc}
	status, body := h.Handle(context.Background())
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	var res DisableResult
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success=true")
	}
	if svc.disableCalls != 1 {
		t.Fatalf("expected 1 disable call, got %d", svc.disableCalls)
	}
}

func TestDisableHandler_ServiceError(t *testing.T) {
	svc := &fakeTunnelService{disableErr: errors.New("kill failed")}
	h := &DisableHandler{Service: svc}
	status, body := h.Handle(context.Background())
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
	var err HTTPError
	_ = json.Unmarshal(body, &err)
	if err.Error != "kill failed" {
		t.Fatalf("error msg = %q, want %q", err.Error, "kill failed")
	}
}

func TestDisableHandler_Idempotent(t *testing.T) {
	// Disable when nothing is running returns nil (no error).
	svc := &fakeTunnelService{}
	h := &DisableHandler{Service: svc}
	status, body := h.Handle(context.Background())
	if status != 200 {
		t.Fatalf("expected 200 even with no tunnel, got %d (body=%s)", status, body)
	}
}

func TestDisableHandler_NilService(t *testing.T) {
	h := &DisableHandler{}
	status, _ := h.Handle(context.Background())
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
}
