package oauth

import (
	"fmt"
	"sync"
	"time"
)

// Voice is a TTS voice in the standard format.
type Voice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
}

// mediaVoicesResponse is the response shape.
type mediaVoicesResponse struct {
	Provider string  `json:"provider"`
	Voices   []Voice `json:"voices"`
}

// MediaVoicesFetcher fetches voices from a provider.
type MediaVoicesFetcher func() ([]Voice, error)

var (
	mediaFetcherMu     sync.RWMutex
	mediaFetcher       = map[string]MediaVoicesFetcher{}
	mediaVoicesCacheMu sync.RWMutex
	mediaVoicesCache   = map[string]mediaVoicesCacheEntry{}
)

type mediaVoicesCacheEntry struct {
	voices    []Voice
	expiresAt time.Time
}

// RegisterMediaVoicesFetcher registers a voices fetcher for a provider.
func RegisterMediaVoicesFetcher(provider string, fn MediaVoicesFetcher) {
	mediaFetcherMu.Lock()
	defer mediaFetcherMu.Unlock()
	mediaFetcher[provider] = fn
}

// ClearMediaVoicesCache clears the in-memory voices cache.
func ClearMediaVoicesCache() {
	mediaVoicesCacheMu.Lock()
	defer mediaVoicesCacheMu.Unlock()
	mediaVoicesCache = map[string]mediaVoicesCacheEntry{}
}

const mediaVoicesCacheTTL = 5 * time.Minute

// HandleMediaVoices implements GET /api/media-providers/tts/:provider/voices.
func HandleMediaVoices(c *Context) (any, error) {
	provider := c.Provider
	mediaFetcherMu.RLock()
	fetcher, ok := mediaFetcher[provider]
	mediaFetcherMu.RUnlock()
	if !ok {
		return nil, NewHandlerError("NOT_FOUND", fmt.Sprintf("Provider %q not supported", provider))
	}

	mediaVoicesCacheMu.RLock()
	entry, hit := mediaVoicesCache[provider]
	mediaVoicesCacheMu.RUnlock()
	if hit && time.Now().Before(entry.expiresAt) {
		return mediaVoicesResponse{Provider: provider, Voices: entry.voices}, nil
	}

	voices, err := fetcher()
	if err != nil {
		return nil, NewHandlerError("PROVIDER_ERROR", fmt.Sprintf("Failed to fetch voices: %v", err))
	}
	if voices == nil {
		voices = []Voice{}
	}

	mediaVoicesCacheMu.Lock()
	mediaVoicesCache[provider] = mediaVoicesCacheEntry{
		voices:    voices,
		expiresAt: time.Now().Add(mediaVoicesCacheTTL),
	}
	mediaVoicesCacheMu.Unlock()

	return mediaVoicesResponse{Provider: provider, Voices: voices}, nil
}
