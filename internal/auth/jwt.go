package auth

import "github.com/golang-jwt/jwt/v5"

// DashboardClaims represents the JWT claims for dashboard authentication.
// Embeds jwt.RegisteredClaims for standard fields (exp, iat, nbf, sub).
type DashboardClaims struct {
	jwt.RegisteredClaims
	Authenticated bool `json:"authenticated"`
}

// LoginResponse is the response body for POST /api/auth/login.
type LoginResponse struct {
	Success             bool `json:"success"`
	RemainingBeforeLock int  `json:"remainingBeforeLock"`
}

// LoginErrorResponse is the error response for login failures.
type LoginErrorResponse struct {
	Error               string `json:"error"`
	RemainingBeforeLock int    `json:"remainingBeforeLock"`
}

// AuthStatusResponse is the response body for GET /api/auth/status.
type AuthStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	AuthMode      string `json:"authMode"`
}
