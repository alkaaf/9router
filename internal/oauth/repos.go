package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
)

// pcRepo is the global ProviderConnectionRepo.
var (
	pcRepoMu sync.RWMutex
	pcRepo   ProviderConnectionRepo
)

// SetProviderConnectionRepo swaps in a new repo.
func SetProviderConnectionRepo(r ProviderConnectionRepo) {
	pcRepoMu.Lock()
	defer pcRepoMu.Unlock()
	pcRepo = r
}

func currentProviderConnectionRepo() ProviderConnectionRepo {
	pcRepoMu.RLock()
	defer pcRepoMu.RUnlock()
	return pcRepo
}

// jobRepo is the global JobRepo.
var (
	jobRepoMu sync.RWMutex
	jobRepo   JobRepo
)

// SetJobRepo swaps in a new JobRepo.
func SetJobRepo(r JobRepo) {
	jobRepoMu.Lock()
	defer jobRepoMu.Unlock()
	jobRepo = r
}

func currentJobRepo() JobRepo {
	jobRepoMu.RLock()
	defer jobRepoMu.RUnlock()
	return jobRepo
}

// kvRepo is the global KVRepo.
var (
	kvRepoMu sync.RWMutex
	kvRepo   KVRepo
)

// SetKVRepo swaps in a new KVRepo.
func SetKVRepo(r KVRepo) {
	kvRepoMu.Lock()
	defer kvRepoMu.Unlock()
	kvRepo = r
}

func currentKVRepo() KVRepo {
	kvRepoMu.RLock()
	defer kvRepoMu.RUnlock()
	return kvRepo
}

// encryptJSONValue serialises v as JSON. Production overrides
// SetEncryptor to apply real encryption.
var encryptMu sync.Mutex
var encryptJSONValue = func(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// SetEncryptor overrides the JSON encryptor.
func SetEncryptor(fn func(v any) string) {
	encryptMu.Lock()
	defer encryptMu.Unlock()
	if fn == nil {
		encryptJSONValue = func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		}
		return
	}
	encryptJSONValue = fn
}

// upsertProviderConnection routes the upsert through the global pcRepo.
var upsertProviderConnection = func(ctx context.Context, in ProviderConnection) (*ProviderConnection, error) {
	repo := currentProviderConnectionRepo()
	if repo == nil {
		if in.ID == "" {
			in.ID = generateID()
		}
		return &in, nil
	}
	existing, _ := repo.GetByProviderAndType(ctx, in.Provider, in.AuthType)
	if existing != nil {
		existing.Data = in.Data
		existing.Email = in.Email
		existing.Name = in.Name
		return repo.Upsert(ctx, existing)
	}
	if in.ID == "" {
		in.ID = generateID()
	}
	return repo.Upsert(ctx, &in)
}

// generateID returns a random UUID v4-like identifier.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// shortToken returns a short hex token for simulation purposes.
func shortToken(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)[:n]
}
