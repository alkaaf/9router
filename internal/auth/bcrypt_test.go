package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_Cost12Prefix(t *testing.T) {
	hash, err := HashPassword("123456")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Errorf("expected $2a$12$ prefix, got %s", hash[:7])
	}
	if !VerifyPassword("123456", hash) {
		t.Error("expected verify to succeed for matching password")
	}
	if VerifyPassword("wrong", hash) {
		t.Error("expected verify to fail for wrong password")
	}
}

func TestHashPassword_EmptyString(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash even for empty string")
	}
}

func TestHashPassword_TwoCallsDiffer(t *testing.T) {
	h1, err := HashPassword("123456")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	h2, err := HashPassword("123456")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if h1 == h2 {
		t.Error("expected different hashes due to random salt")
	}
	if !VerifyPassword("123456", h1) {
		t.Error("expected first hash to verify")
	}
	if !VerifyPassword("123456", h2) {
		t.Error("expected second hash to verify")
	}
}

func TestVerifyPassword_EmptyHash(t *testing.T) {
	if VerifyPassword("anything", "") {
		t.Error("expected false for empty stored hash")
	}
}

func TestVerifyPassword_LongPassword(t *testing.T) {
	long := strings.Repeat("a", 80)
	hash, err := HashPassword(long)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !VerifyPassword(long, hash) {
		t.Error("expected verify to succeed for long password")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("123456")
	if VerifyPassword("wrong-password", hash) {
		t.Error("expected false for wrong password")
	}
}

func TestHashPassword_LongPassword72Plus(t *testing.T) {
	long := strings.Repeat("a", 100)
	hash, err := HashPassword(long)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !VerifyPassword(long, hash) {
		t.Error("expected verify to succeed for 72+ char password")
	}
}
