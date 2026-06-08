package auth

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
)

var (
	secretMu sync.RWMutex
	secret   []byte
)

const envSecretKey = "NINE_ROUTER_JWT_SECRET"

// SetSecret configures the package-wide signing secret. Use in tests.
func SetSecret(s []byte) {
	secretMu.Lock()
	defer secretMu.Unlock()
	if len(s) == 0 {
		secret = nil
		return
	}
	cp := make([]byte, len(s))
	copy(cp, s)
	secret = cp
}

// GetSecret returns the package-wide signing secret, generating a random
// one on first call if no secret has been configured and the
// NINE_ROUTER_JWT_SECRET env var is unset.
func GetSecret() []byte {
	secretMu.RLock()
	if len(secret) > 0 {
		s := secret
		secretMu.RUnlock()
		return s
	}
	secretMu.RUnlock()

	if v := os.Getenv(envSecretKey); v != "" {
		b, _ := hex.DecodeString(v)
		if len(b) == 0 {
			b = []byte(v)
		}
		secretMu.Lock()
		if len(secret) == 0 {
			secret = b
		}
		out := secret
		secretMu.Unlock()
		return out
	}

	gen, _ := generateSecret()
	secretMu.Lock()
	if len(secret) == 0 {
		secret = gen
	}
	out := secret
	secretMu.Unlock()
	return out
}

func generateSecret() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
