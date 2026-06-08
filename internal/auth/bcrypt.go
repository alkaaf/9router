package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the cost factor used for password hashing.
const bcryptCost = 12

// bcryptMaxBytes is the 72-byte limit imposed by bcrypt. Inputs longer
// than this are silently truncated to match Node.js bcrypt behavior.
const bcryptMaxBytes = 72

// HashPassword returns a bcrypt-encoded hash of the plaintext password
// suitable for storage in the database.
func HashPassword(plaintext string) (string, error) {
	if len(plaintext) > bcryptMaxBytes {
		plaintext = plaintext[:bcryptMaxBytes]
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt.GenerateFromPassword: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword compares a plaintext password against a stored bcrypt
// hash. Returns true on match, false on any failure.
func VerifyPassword(plaintext, storedHash string) bool {
	if storedHash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(plaintext))
	return err == nil
}
