package auth

import (
	"time"

	"gorm.io/gorm"
)

// AuthApiKey stores API keys as bcrypt hashes for the auth middleware.
type AuthApiKey struct {
	ID         uint           `gorm:"primaryKey;column:id"`
	KeyHash    string         `gorm:"not null;type:text;column:key_hash"`
	Name       string         `gorm:"not null;column:name"`
	LastUsedAt *time.Time     `gorm:"column:last_used_at"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName pins the table name to "api_keys".
func (AuthApiKey) TableName() string {
	return "api_keys"
}
