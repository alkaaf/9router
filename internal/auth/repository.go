package auth

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ApiKeyRepository is the persistence layer for AuthApiKey records.
type ApiKeyRepository struct {
	db *gorm.DB
}

// NewApiKeyRepository returns a repository bound to the given GORM DB.
func NewApiKeyRepository(db *gorm.DB) *ApiKeyRepository {
	return &ApiKeyRepository{db: db}
}

// CreateApiKey hashes the raw key and persists the record.
func (r *ApiKeyRepository) CreateApiKey(ctx context.Context, rawKey, name string) (*AuthApiKey, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcryptCost)
	if err != nil {
		return nil, err
	}
	rec := &AuthApiKey{
		KeyHash: string(hash),
		Name:    name,
	}
	if err := r.db.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// FindValidApiKey iterates all non-deleted records and bcrypt-compares
// the raw key against each KeyHash.
func (r *ApiKeyRepository) FindValidApiKey(ctx context.Context, rawKey string) (*AuthApiKey, error) {
	var keys []AuthApiKey
	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Find(&keys).Error; err != nil {
		return nil, err
	}
	for _, k := range keys {
		if bcrypt.CompareHashAndPassword([]byte(k.KeyHash), []byte(rawKey)) == nil {
			return &k, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// ListApiKeys returns all non-deleted records with KeyHash zeroed.
func (r *ApiKeyRepository) ListApiKeys(ctx context.Context) ([]AuthApiKey, error) {
	var out []AuthApiKey
	if err := r.db.WithContext(ctx).
		Model(&AuthApiKey{}).
		Where("deleted_at IS NULL").
		Find(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		out[i].KeyHash = ""
	}
	return out, nil
}

// UpdateLastUsed sets LastUsedAt to now.
func (r *ApiKeyRepository) UpdateLastUsed(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).
		Model(&AuthApiKey{}).
		Where("id = ?", id).
		Update("last_used_at", time.Now()).
		Error
}

// DeleteApiKey soft-deletes by ID.
func (r *ApiKeyRepository) DeleteApiKey(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&AuthApiKey{}, id).Error
}
