package chatcore

import (
	"encoding/json"
	"strings"
)

// ComboInfo describes a single combo loaded from the DB. The Models
// field is the JSON-decoded array of model strings stored in the
// combos row.
type ComboInfo struct {
	ID     string
	Name   string
	Kind   string
	Models []string
}

// ComboLookup fetches a combo by name. It returns (nil, nil) when the
// combo does not exist — the caller treats both an empty lookup and
// an explicit "not found" identically.
type ComboLookup func(name string) (*ComboInfo, error)

// GetComboModels mirrors open-sse/services/combo.js#
// getComboModelsFromData. It detects whether the incoming modelStr
// is a combo name and, if so, returns the expanded list of models.
//
// The function returns (nil, nil) for three distinct cases:
//
//   - modelStr contains "/", so it is an explicit "provider/model"
//     reference and never a combo name.
//   - modelStr is a name that does not match any combo in the table.
//   - modelStr matches a combo row but the row's models list is empty.
//
// Callers MUST treat a nil return as "this is a single model" and
// proceed with normal alias resolution. Returning an explicit error
// is reserved for actual DB / decode failures.
func GetComboModels(modelStr string, lookup ComboLookup) ([]string, error) {
	// Step 1 — slash is the explicit provider/model form. The
	// Node.js code rejects this case at the top of the function so
	// combo names that happen to contain "/" (theoretically legal,
	// but never used in practice) cannot be misinterpreted.
	if strings.Contains(modelStr, "/") {
		return nil, nil
	}
	// Step 2 — early exit for empty / whitespace input.
	if strings.TrimSpace(modelStr) == "" {
		return nil, nil
	}
	if lookup == nil {
		return nil, nil
	}

	combo, err := lookup(modelStr)
	if err != nil {
		return nil, err
	}
	if combo == nil {
		return nil, nil
	}
	if len(combo.Models) == 0 {
		return nil, nil
	}
	return combo.Models, nil
}

// ParseComboModels is a helper for repository code that loads combo
// rows from the DB. The combos row's `models` column is a JSON array
// of model strings (or possibly a JSON object — the Node.js schema
// accepts both shapes in some older versions). The function returns
// the decoded list, or nil on a decode error / wrong type.
func ParseComboModels(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Try array form first (the canonical shape used since v2).
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}
	// Fall back to { models: [...] } shape.
	var obj struct {
		Models []string `json:"models"`
	}
	if err := json.Unmarshal([]byte(raw), &obj); err == nil && obj.Models != nil {
		return obj.Models
	}
	return nil
}
