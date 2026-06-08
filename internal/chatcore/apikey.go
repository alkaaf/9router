package chatcore

import (
	"errors"
	"fmt"

	"github.com/9router/9router/internal/model"
	"gorm.io/gorm"
)

// apiKeyValidator checks whether an incoming chat request is allowed to
// proceed based on the requireApiKey setting and the caller's supplied
// API key. It mirrors the behaviour in
// src/sse/handlers/chat.js#handleChat (the requireApiKey + isValidApiKey
// branch).
//
// The function returns nil when the request is authorised. Otherwise it
// returns an error whose string form is the user-facing message (either
// "Missing API key" or "Invalid API key") so the HTTP layer can forward
// it verbatim to WriteError with status 401.
type apiKeyValidator struct {
	db *gorm.DB
}

// newAPIKeyValidator builds a validator bound to the supplied GORM
// database handle. The handle is stored by pointer; callers must not
// close it for the lifetime of the validator.
func newAPIKeyValidator(db *gorm.DB) *apiKeyValidator {
	return &apiKeyValidator{db: db}
}

// Validate returns nil when the request satisfies the API-key policy,
// or a non-nil error whose string form is the rejection message. The
// function does NOT translate the error to an HTTP response — that is
// the caller's responsibility.
func (v *apiKeyValidator) Validate(requireApiKey bool, apiKey string) error {
	if !requireApiKey {
		return nil
	}
	if apiKey == "" {
		return fmt.Errorf("%w: %s", errAPIKeyRejected, "Missing API key")
	}

	var key model.ApiKey
	// The ApiKey.Key column has a UNIQUE index, so First by key column
	// is an O(log n) index seek, not a full table scan.
	if err := v.db.Where("`key` = ?", apiKey).First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: %s", errAPIKeyRejected, "Invalid API key")
		}
		return fmt.Errorf("%w: %v", errAPIKeyRejected, err)
	}

	// A key that exists but has isActive=false (or isActive IS NULL,
	// which GORM decodes as the zero *bool) is treated as inactive.
	// The Node.js isValidApiKey -> validateApiKey flow does the same
	// via a WHERE isActive = 1 clause.
	if key.IsActive == nil || !*key.IsActive {
		return fmt.Errorf("%w: %s", errAPIKeyRejected, "Invalid API key")
	}

	return nil
}

// errAPIKeyRejected is the sentinel used to distinguish "API key
// rejected" from other errors (DB down, decode failures, etc.). The
// HTTP layer checks errors.Is(err, errAPIKeyRejected) to decide
// whether to return 401 or 500.
var errAPIKeyRejected = fmt.Errorf("api key rejected")
