package repository

import (
	"encoding/json"
	"errors"

	"github.com/9router/9router/internal/model"
	"gorm.io/gorm"
)

// KVRepo extends the basic KVRepository with typed Get/Set helpers
// for JSON-encoded values (used by pricing, model aliases, etc.).
type KVRepo struct {
	*KVRepository
}

func NewKVRepo(db *gorm.DB) *KVRepo {
	return &KVRepo{KVRepository: NewKVRepository(db)}
}

// GetJSON reads a JSON-encoded value from the kv store. Returns
// (nil, nil) if the key does not exist.
func (r *KVRepo) GetJSON(scope, key string) (any, error) {
	var kv model.KV
	err := r.db.Where("scope = ? AND key = ?", scope, key).Take(&kv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var out any
	if err := json.Unmarshal([]byte(kv.Value), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetJSON writes a JSON-encoded value to the kv store, replacing any
// existing row with the same (scope, key) pair.
func (r *KVRepo) SetJSON(scope, key string, value any) error {
	buf, err := json.Marshal(value)
	if err != nil {
		return err
	}
	kv := model.KV{Scope: scope, Key: key, Value: string(buf)}
	return r.db.Save(&kv).Error
}

// Delete removes a key from the kv store. Returns nil if the key did
// not exist.
func (r *KVRepo) Delete(scope, key string) error {
	return r.db.Where("scope = ? AND key = ?", scope, key).Delete(&model.KV{}).Error
}
