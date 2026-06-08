package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-32-bytes-long-xxxxxx"

func TestSignJWT_Basic(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)

	tok, err := SignJWT("admin", nil)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	if tok == "" {
		t.Fatal("expected non-empty token")
	}

	parsed, err := jwt.ParseWithClaims(tok, &DashboardClaims{}, func(t *jwt.Token) (interface{}, error) {
		return GetSecret(), nil
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	claims, ok := parsed.Claims.(*DashboardClaims)
	if !ok {
		t.Fatalf("expected DashboardClaims, got %T", parsed.Claims)
	}
	if claims.Subject != "admin" {
		t.Errorf("expected sub=admin, got %q", claims.Subject)
	}
	if !claims.Authenticated {
		t.Error("expected authenticated=true")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected exp claim")
	}
	delta := claims.ExpiresAt.Time.Sub(time.Now())
	if delta < 23*time.Hour || delta > 25*time.Hour {
		t.Errorf("exp delta out of range: %v", delta)
	}
}

func TestSignJWT_WithExtras(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)

	extras := map[string]interface{}{"role": "superadmin"}
	tok, err := SignJWT("admin", extras)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	parsed, err := jwt.ParseWithClaims(tok, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		return GetSecret(), nil
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	parts, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("expected MapClaims, got %T", parsed.Claims)
	}
	role, ok := parts["role"].(string)
	if !ok || role != "superadmin" {
		t.Errorf("expected role=superadmin in extras, got %v", parts["role"])
	}
}

func TestSignJWT_WrongSecret(t *testing.T) {
	SetSecret([]byte(testSecret))
	tok, err := SignJWT("admin", nil)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	SetSecret([]byte("different-secret-32-bytes-xxxx"))
	defer SetSecret(nil)

	_, err = jwt.ParseWithClaims(tok, &DashboardClaims{}, func(t *jwt.Token) (interface{}, error) {
		return GetSecret(), nil
	})
	if err == nil {
		t.Fatal("expected verification failure with wrong secret")
	}
}
