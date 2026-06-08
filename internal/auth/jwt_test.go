package auth

import (
	"encoding/json"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestDashboardClaims_Marshal(t *testing.T) {
	claims := DashboardClaims{
		Authenticated:   true,
		RegisteredClaims: jwt.RegisteredClaims{Subject: "admin"},
	}
	data, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if m["sub"] != "admin" {
		t.Errorf("expected sub=admin, got %v", m["sub"])
	}
	if m["authenticated"] != true {
		t.Errorf("expected authenticated=true, got %v", m["authenticated"])
	}
}

func TestLoginResponse_Marshal_Success(t *testing.T) {
	resp := LoginResponse{Success: true}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"success":true,"remainingBeforeLock":0}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestAuthStatusResponse_Marshal(t *testing.T) {
	resp := AuthStatusResponse{Authenticated: true, AuthMode: "password"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"authenticated":true,"authMode":"password"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestLoginErrorResponse_Marshal(t *testing.T) {
	resp := LoginErrorResponse{Error: "invalid password", RemainingBeforeLock: 3}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"error":"invalid password","remainingBeforeLock":3}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}
