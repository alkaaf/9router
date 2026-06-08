package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenTTL is the lifetime of a dashboard JWT.
const TokenTTL = 24 * time.Hour

// SignJWT creates and signs a dashboard JWT using HS256 with the package
// secret. The claims include the username as the subject, an
// "authenticated": true flag, and exp 24h in the future. Extra claims
// provided are merged into the registered claims map.
func SignJWT(username string, extraClaims map[string]interface{}) (string, error) {
	now := time.Now()
	// Use MapClaims so that extra claims are merged naturally without
	// round-tripping through a typed struct (which would drop unknown
	// keys on unmarshal).
	claims := jwt.MapClaims{
		"sub":           username,
		"authenticated": true,
		"iat":           now.Unix(),
		"exp":           now.Add(TokenTTL).Unix(),
		"iss":           "9router",
	}
	for k, v := range extraClaims {
		claims[k] = v
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(GetSecret())
}
