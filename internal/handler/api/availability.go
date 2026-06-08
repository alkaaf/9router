package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/9router/9router/internal/repository"
	"gorm.io/gorm"
)

// AvailabilityHandler returns an http.HandlerFunc for GET/POST
// /api/models/availability.
func AvailabilityHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleAvailabilityGet(w, db)
		case http.MethodPost:
			handleAvailabilityPost(w, r, db)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

type availabilityEntry struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Status         string `json:"status"`
	Until          int64  `json:"until,omitempty"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName,omitempty"`
	LastError      string `json:"lastError,omitempty"`
}

func handleAvailabilityGet(w http.ResponseWriter, db *gorm.DB) {
	pcRepo := repository.NewProviderRepository(db)
	conns, err := pcRepo.ListAll()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	now := time.Now()
	out := make([]availabilityEntry, 0)
	unavailableCount := 0

	for _, c := range conns {
		// Check if the connection has any modelLock_* fields with future timestamps.
		// These are stored in the Data JSON field; we look for keys starting with
		// "modelLock_" and parse the value as a unix-ms timestamp.
		locks, lastErr := extractModelLocks(c.Data, now)
		if lastErr != "" && c.IsActive != nil && *c.IsActive {
			// connection is marked active but has a last error — surface it
		}
		for modelName, until := range locks {
			out = append(out, availabilityEntry{
				Provider:       c.Provider,
				Model:          modelName,
				Status:         "cooldown",
				Until:          until,
				ConnectionID:   c.ID,
				ConnectionName: strDeref(c.Name),
				LastError:      lastErr,
			})
		}
		if c.IsActive != nil && !*c.IsActive {
			out = append(out, availabilityEntry{
				Provider:       c.Provider,
				Model:          "__all",
				Status:         "unavailable",
				ConnectionID:   c.ID,
				ConnectionName: strDeref(c.Name),
				LastError:      lastErr,
			})
			unavailableCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"models":           out,
		"unavailableCount": unavailableCount,
	})
}

func handleAvailabilityPost(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var body struct {
		Action   string `json:"action"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if body.Action != "clearCooldown" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "unknown action")
		return
	}
	if body.Provider == "" || body.Model == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "provider and model required")
		return
	}
	pcRepo := repository.NewProviderRepository(db)
	conns, err := pcRepo.ListAll()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	cleared := false
	for _, c := range conns {
		if c.Provider != body.Provider {
			continue
		}
		updated, ok := clearModelLock(c.Data, body.Model)
		if !ok {
			continue
		}
		active := true
		c.Data = updated
		c.IsActive = &active
		if err := db.Save(&c).Error; err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		cleared = true
	}
	if !cleared {
		// Not necessarily an error: the cooldown may have already expired.
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// extractModelLocks returns a map of modelName -> until(unix-ms) for all
// modelLock_* keys whose until is in the future. Also returns the
// lastError string from the connection data.
func extractModelLocks(data string, now time.Time) (map[string]int64, string) {
	out := map[string]int64{}
	if data == "" {
		return out, ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return out, ""
	}
	lastErr, _ := m["lastError"].(string)
	for k, v := range m {
		if !strings.HasPrefix(k, "modelLock_") {
			continue
		}
		modelName := strings.TrimPrefix(k, "modelLock_")
		n, ok := v.(float64)
		if !ok {
			continue
		}
		until := int64(n)
		if until <= now.UnixMilli() {
			continue
		}
		out[modelName] = until
	}
	return out, lastErr
}

// clearModelLock removes the modelLock_<model> entry from the connection
// data JSON. Returns the new data and whether a lock was actually cleared.
func clearModelLock(data, model string) (string, bool) {
	if data == "" {
		return data, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return data, false
	}
	key := "modelLock_" + model
	if _, ok := m[key]; !ok {
		return data, false
	}
	delete(m, key)
	buf, _ := json.Marshal(m)
	return string(buf), true
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
