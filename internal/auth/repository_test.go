package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newApiKeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AuthApiKey{}))
	return db
}

func TestAuthApiKey_TableName(t *testing.T) {
	a := AuthApiKey{}
	assert.Equal(t, "api_keys", a.TableName())
}

func TestAuthApiKey_CreateAndFind(t *testing.T) {
	db := newApiKeyTestDB(t)
	repo := NewApiKeyRepository(db)
	ctx := context.Background()

	rec, err := repo.CreateApiKey(ctx, "my-secret-key", "CI")
	require.NoError(t, err)
	assert.Equal(t, "CI", rec.Name)
	assert.NotEmpty(t, rec.KeyHash)
	assert.True(t, rec.ID > 0)

	found, err := repo.FindValidApiKey(ctx, "my-secret-key")
	require.NoError(t, err)
	assert.Equal(t, "CI", found.Name)

	_, err = repo.FindValidApiKey(ctx, "wrong-key")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestAuthApiKey_ListAndDelete(t *testing.T) {
	db := newApiKeyTestDB(t)
	repo := NewApiKeyRepository(db)
	ctx := context.Background()

	_, _ = repo.CreateApiKey(ctx, "key-1", "CI")
	_, _ = repo.CreateApiKey(ctx, "key-2", "CLI")

	keys, err := repo.ListApiKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	for _, k := range keys {
		assert.Empty(t, k.KeyHash, "KeyHash must be zeroed in list")
	}

	require.NoError(t, repo.DeleteApiKey(ctx, keys[0].ID))
	keys2, err := repo.ListApiKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, keys2, 1)
}

func TestAuthApiKey_UpdateLastUsed(t *testing.T) {
	db := newApiKeyTestDB(t)
	repo := NewApiKeyRepository(db)
	ctx := context.Background()

	rec, _ := repo.CreateApiKey(ctx, "key-1", "Test")
	before := rec.CreatedAt

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.UpdateLastUsed(ctx, rec.ID))

	var updated AuthApiKey
	require.NoError(t, db.First(&updated, rec.ID).Error)
	assert.True(t, updated.UpdatedAt.After(before), "UpdatedAt should change")
}
