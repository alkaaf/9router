package model

import "encoding/json"

// Setting represents the single-row `settings` table that holds the entire
// application configuration as a JSON-encoded string.
//
// Mirrors the `settings` table in the existing Node.js schema:
//   - id   INTEGER PRIMARY KEY CHECK (id = 1)   -- enforces single-row
//   - data TEXT NOT NULL                        -- JSON-encoded config blob
//
// The repository is responsible for upserting this row
// and for parsing `Data` into a typed structure on read.
type Setting struct {
	ID   uint   `gorm:"primaryKey;check:id = 1;autoIncrement;column:id"`
	Data string `gorm:"not null;type:text;column:data"`
}

// TableName pins the table name to `settings`.
func (Setting) TableName() string {
	return "settings"
}

// GetData parses the JSON-encoded Data field into a generic map.
func (s *Setting) GetData() (map[string]any, error) {
	out := map[string]any{}
	if err := json.Unmarshal([]byte(s.Data), &out); err != nil {
		return nil, err
	}
	return out, nil
}
