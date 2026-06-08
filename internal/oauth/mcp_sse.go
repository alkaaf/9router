package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// MCPSSEStreamer streams events for a plugin.
type MCPSSEStreamer interface {
	StreamEvents(ctx context.Context) (<-chan json.RawMessage, error)
}

// MCPSSEStreamerRegistry looks up SSE streamers by plugin name.
type MCPSSEStreamerRegistry interface {
	GetStreamer(pluginName string) MCPSSEStreamer
}

var (
	mcpSSEStreamerRegistryMu sync.RWMutex
	mcpSSEStreamerRegistry   MCPSSEStreamerRegistry
)

// SetMCPSSEStreamerRegistry sets the SSE streamer registry.
func SetMCPSSEStreamerRegistry(r MCPSSEStreamerRegistry) {
	mcpSSEStreamerRegistryMu.Lock()
	defer mcpSSEStreamerRegistryMu.Unlock()
	mcpSSEStreamerRegistry = r
}

func currentMCPSSEStreamerRegistry() MCPSSEStreamerRegistry {
	mcpSSEStreamerRegistryMu.RLock()
	defer mcpSSEStreamerRegistryMu.RUnlock()
	return mcpSSEStreamerRegistry
}

// DefaultMCPSSEStreamerRegistry is a simple in-memory streamer registry.
type DefaultMCPSSEStreamerRegistry struct {
	streamers map[string]MCPSSEStreamer
}

// NewDefaultMCPSSEStreamerRegistry creates a new empty registry.
func NewDefaultMCPSSEStreamerRegistry() *DefaultMCPSSEStreamerRegistry {
	return &DefaultMCPSSEStreamerRegistry{streamers: make(map[string]MCPSSEStreamer)}
}

// Register adds a streamer.
func (r *DefaultMCPSSEStreamerRegistry) Register(name string, s MCPSSEStreamer) {
	r.streamers[name] = s
}

// GetStreamer implements MCPSSEStreamerRegistry.
func (r *DefaultMCPSSEStreamerRegistry) GetStreamer(name string) MCPSSEStreamer {
	return r.streamers[name]
}

// mcpSSEStartResponse is what HandleMCPSSE returns.
type mcpSSEStartResponse struct {
	Type   string `json:"type"`
	Plugin string `json:"plugin"`
}

// HandleMCPSSE implements GET /api/mcp/:plugin/sse.
func HandleMCPSSE(c *Context) (any, error) {
	registry := currentMCPSSEStreamerRegistry()
	if registry == nil {
		return nil, NewHandlerError("INTERNAL_ERROR", "SSE streamer registry not configured")
	}
	streamer := registry.GetStreamer(c.Provider)
	if streamer == nil {
		return nil, NewHandlerError("NOT_FOUND", fmt.Sprintf("Plugin %q not found", c.Provider))
	}
	return mcpSSEStartResponse{Type: "sse-start", Plugin: c.Provider}, nil
}
