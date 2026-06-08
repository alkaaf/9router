package auth

import (
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// VerifyJWT parses and validates a dashboard JWT. Returns the
// RegisteredClaims on success or nil/false on any failure (missing,
// malformed, expired, wrong signature). All errors are logged at debug
// level; callers only receive the boolean result.
func VerifyJWT(tokenString string) (claims *jwt.RegisteredClaims, ok bool) {
	if tokenString == "" {
		log.Printf("[auth] VerifyJWT: empty token string")
		return nil, false
	}
	c := &DashboardClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %T", t.Method)
		}
		return GetSecret(), nil
	})
	if err != nil {
		log.Printf("[auth] VerifyJWT: parse error: %v", err)
		return nil, false
	}
	if !tok.Valid {
		log.Printf("[auth] VerifyJWT: token not valid")
		return nil, false
	}
	if c.ExpiresAt != nil && c.ExpiresAt.Time.Before(time.Now()) {
		log.Printf("[auth] VerifyJWT: token expired at %v", c.ExpiresAt.Time)
		return nil, false
	}
	return &c.RegisteredClaims, true
}
