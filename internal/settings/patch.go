package settings

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// SettingsReader abstracts reading settings (GET handler).
type SettingsReader interface {
	GetSettings() (map[string]any, error)
}

// SettingsWriter extends SettingsReader with write support (PATCH handler).
type SettingsWriter interface {
	SettingsReader
	UpdateSettings(updates map[string]any) (map[string]any, error)
}

// SideEffects captures the runtime hooks the patch handler triggers when
// outbound proxy or combo strategy fields change. Both callbacks are
// nil-safe — the handler will simply skip the call if nil.
type SideEffects struct {
	ApplyOutboundProxyEnv func(settings map[string]any)
	ResetComboRotation    func()
}

// SettingsPatchHandler returns an http.HandlerFunc for PATCH /api/settings.
// It performs a merge-patch update of the singleton settings row, hashes
// newPassword with bcrypt, strips empty oidcClientSecret, and triggers
// the registered side effects on relevant field changes.
func SettingsPatchHandler(store SettingsWriter, side SideEffects) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.Header().Set("Allow", http.MethodPatch)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Could not read request body")
			return
		}
		patch := map[string]any{}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &patch); err != nil {
				writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
				return
			}
		}

		// Determine which fields the client actually sent.
		hasKey := func(key string) bool {
			_, ok := patch[key]
			return ok
		}

		// Track which side effects to invoke.
		triggerProxy := hasKey("outboundProxyEnabled") || hasKey("outboundProxyUrl") || hasKey("outboundNoProxy")
		triggerCombo := hasKey("comboStrategy") || hasKey("comboStickyRoundRobinLimit") || hasKey("comboStrategies")

		// Build the writeable patch (strip newPassword/currentPassword — handled below).
		updates := map[string]any{}
		for k, v := range patch {
			if k == "newPassword" || k == "currentPassword" {
				continue
			}
			updates[k] = v
		}

		// OIDC secret clearing: empty/whitespace deletes the field.
		if hasKey("oidcClientSecret") {
			s, _ := patch["oidcClientSecret"].(string)
			if strings.TrimSpace(s) == "" {
				updates["oidcClientSecret"] = nil // sentinel: delete key
			}
		}

		// Password handling.
		if hasKey("newPassword") {
			newPwd, _ := patch["newPassword"].(string)
			if newPwd == "" {
				writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "newPassword must not be empty")
				return
			}
			current, err := store.GetSettings()
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
				return
			}
			existing, _ := current["password"].(string)
			if err := verifyCurrentPassword(store, existing, patch); err != nil {
				writeJSONError(w, statusForPasswordError(err), "INVALID_PASSWORD", err.Error())
				return
			}
			hash, err := bcryptHash(newPwd)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
				return
			}
			updates["password"] = hash
		}

		// Persist the patch.
		merged, err := store.UpdateSettings(updates)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}

		// Side effects run after the write succeeds.
		if triggerProxy && side.ApplyOutboundProxyEnv != nil {
			side.ApplyOutboundProxyEnv(merged)
		}
		if triggerCombo && side.ResetComboRotation != nil {
			side.ResetComboRotation()
		}

		// Build the same redacted response as GET.
		oidcSecret, _ := merged["oidcClientSecret"].(string)
		password, _ := merged["password"].(string)

		safe := redactSettings(merged)
		safe["oidcConfigured"] = oidcConfigured(safe, oidcSecret)
		safe["hasPassword"] = password != ""
		safe["enableRequestLogs"] = envBool("ENABLE_REQUEST_LOGS")
		safe["enableTranslator"] = envBool("ENABLE_TRANSLATOR")

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(safe)
	}
}

// verifyCurrentPassword validates currentPassword against the stored hash
// using the first-time and existing-password rules.
func verifyCurrentPassword(store SettingsReader, existingHash string, patch map[string]any) error {
	supplied, _ := patch["currentPassword"].(string)
	if existingHash != "" {
		// Password exists: currentPassword is required and must match.
		if supplied == "" {
			return errMissingCurrentPassword
		}
		if !bcryptCompare(supplied, existingHash) {
			return errWrongCurrentPassword
		}
		return nil
	}
	// First-time setup: allow empty or "123456".
	if supplied == "" || supplied == "123456" {
		return nil
	}
	return errWrongCurrentPassword
}

var (
	errMissingCurrentPassword = errors.New("current password required")
	errWrongCurrentPassword   = errors.New("invalid current password")
)

func statusForPasswordError(err error) int {
	if errors.Is(err, errMissingCurrentPassword) {
		return http.StatusBadRequest
	}
	return http.StatusUnauthorized
}

// bcryptHash wraps bcrypt.GenerateFromPassword with a stable cost.
func bcryptHash(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), 10)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func bcryptCompare(pw, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
