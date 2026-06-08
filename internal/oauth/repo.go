package oauth

import (
	"context"
	"time"
)

// ProviderConnection is the data needed to upsert a provider credential.
type ProviderConnection struct {
	ID        string
	Provider  string
	AuthType  string
	Email     *string
	Name      *string
	Data      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProviderConnectionQuery filters for list/search operations.
type ProviderConnectionQuery struct {
	Provider string
	AuthType string
	Limit    int
	Offset   int
}

// ProviderConnectionRepo abstracts provider credential storage.
type ProviderConnectionRepo interface {
	Upsert(ctx context.Context, pc *ProviderConnection) (*ProviderConnection, error)
	GetByID(ctx context.Context, id string) (*ProviderConnection, error)
	GetByProviderAndType(ctx context.Context, provider, authType string) (*ProviderConnection, error)
	List(ctx context.Context, q ProviderConnectionQuery) ([]ProviderConnection, error)
	Delete(ctx context.Context, id string) error
}

// Job represents a background job started by an OAuth endpoint.
type Job struct {
	ID        string
	Status    string
	Result    map[string]any
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// JobRepo abstracts background job storage.
type JobRepo interface {
	Create(ctx context.Context, j *Job) (*Job, error)
	Get(ctx context.Context, id string) (*Job, error)
	Update(ctx context.Context, j *Job) (*Job, error)
}

// KVRepo abstracts the OAuth state KV store.
type KVRepo interface {
	Set(ctx context.Context, scope, key, value string, ttl *time.Time) error
	Get(ctx context.Context, scope, key string) (string, error)
	Delete(ctx context.Context, scope, key string) error
}
