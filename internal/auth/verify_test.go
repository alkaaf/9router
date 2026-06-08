package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyJWT_ValidToken(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)

	tok, err := SignJWT("admin", nil)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	claims, ok := VerifyJWT(tok)
	if !ok || claims == nil {
		t.Fatal("expected valid token")
	}
	if claims.Subject != "admin" {
		t.Errorf("expected sub=admin, got %q", claims.Subject)
	}
}

func TestVerifyJWT_ExpiredToken(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)

	claims := jwt.MapClaims{
		"sub":           "admin",
		"authenticated": true,
		"iat":           time.Now().Add(-25 * time.Hour).Unix(),
		"exp":           time.Now().Add(-1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	_, ok := VerifyJWT(signed)
	if ok {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestVerifyJWT_TamperedToken(t *testing.T) {
	SetSecret([]byte(testSecret))
	defer SetSecret(nil)

	tok, err := SignJWT("admin", nil)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	tampered := tok[:len(tok)-4] + "xxxx"
	_, ok := VerifyJWT(tampered)
	if ok {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestVerifyJWT_EmptyString(t *testing.T) {
	_, ok := VerifyJWT("")
	if ok {
		t.Fatal("expected empty token to be rejected")
	}
}

func TestVerifyJWT_MalformedToken(t *testing.T) {
	_, ok := VerifyJWT("not-a-jwt")
	if ok {
		t.Fatal("expected malformed token to be rejected")
	}
}

func TestVerifyJWT_WrongSecret(t *testing.T) {
	SetSecret([]byte(testSecret))
	tok, err := SignJWT("admin", nil)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	SetSecret([]byte("different-secret-32-bytes-xxxx"))
	defer SetSecret(nil)
	_, ok := VerifyJWT(tok)
	if ok {
		t.Fatal("expected wrong-secret verification to fail")
	}
}
