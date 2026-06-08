package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// JSON-RPC 2.0 error codes (per JSON-RPC 2.0 spec).
const (
	jsonRPCParseError     = -32700
	jsonRPCInvalidRequest = -32600
	jsonRPCMethodNotFound = -32601
	jsonRPCInternalError  = -32603
)

// jsonRPCRequest is the incoming JSON-RPC 2.0 request body.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

// jsonRPCResponse is the outgoing JSON-RPC 2.0 response body.
type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
	ID      any           `json:"id"`
}

// jsonRPCError is the JSON-RPC 2.0 error structure.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// MCPPluginHandler is the handler signature for a plugin's message handler.
type MCPPluginHandler func(method string, params json.RawMessage) (any, error)

// MCPPluginRegistry is the interface for looking up registered plugins.
type MCPPluginRegistry interface {
	GetHandler(pluginName string) MCPPluginHandler
}

var (
	mcpPluginRegistryMu sync.RWMutex
	mcpPluginRegistry   MCPPluginRegistry
)

// SetMCPPluginRegistry sets the global MCP plugin registry.
func SetMCPPluginRegistry(r MCPPluginRegistry) {
	mcpPluginRegistryMu.Lock()
	defer mcpPluginRegistryMu.Unlock()
	mcpPluginRegistry = r
}

func currentMCPPluginRegistry() MCPPluginRegistry {
	mcpPluginRegistryMu.RLock()
	defer mcpPluginRegistryMu.RUnlock()
	return mcpPluginRegistry
}

// DefaultMCPPluginRegistry is a simple in-memory plugin registry.
type DefaultMCPPluginRegistry struct {
	plugins map[string]MCPPluginHandler
}

// NewDefaultMCPPluginRegistry creates a new empty registry.
func NewDefaultMCPPluginRegistry() *DefaultMCPPluginRegistry {
	return &DefaultMCPPluginRegistry{plugins: make(map[string]MCPPluginHandler)}
}

// Register adds a plugin handler.
func (r *DefaultMCPPluginRegistry) Register(name string, handler MCPPluginHandler) {
	r.plugins[name] = handler
}

// GetHandler implements MCPPluginRegistry.
func (r *DefaultMCPPluginRegistry) GetHandler(name string) MCPPluginHandler {
	return r.plugins[name]
}

// HandleMCPMessage implements POST /api/mcp/:plugin/message.
func HandleMCPMessage(c *Context) (any, error) {
	if len(c.Body) == 0 {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error: &jsonRPCError{
				Code:    jsonRPCParseError,
				Message: "Parse error: empty request body",
			},
			ID: nil,
		}, nil
	}

	var req jsonRPCRequest
	if err := json.Unmarshal(c.Body, &req); err != nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error: &jsonRPCError{
				Code:    jsonRPCParseError,
				Message: fmt.Sprintf("Parse error: %v", err),
			},
			ID: nil,
		}, nil
	}

	if req.JSONRPC != "2.0" {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error: &jsonRPCError{
				Code:    jsonRPCInvalidRequest,
				Message: "Invalid Request: jsonrpc must be \"2.0\"",
			},
			ID: req.ID,
		}, nil
	}

	if req.Method == "" {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error: &jsonRPCError{
				Code:    jsonRPCInvalidRequest,
				Message: "Invalid Request: method is required",
			},
			ID: req.ID,
		}, nil
	}

	registry := currentMCPPluginRegistry()
	if registry == nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error: &jsonRPCError{
				Code:    jsonRPCInternalError,
				Message: "Internal error: plugin registry not configured",
			},
			ID: req.ID,
		}, nil
	}

	handler := registry.GetHandler(c.Provider)
	if handler == nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error: &jsonRPCError{
				Code:    jsonRPCMethodNotFound,
				Message: fmt.Sprintf("Method not found: plugin %q not registered", c.Provider),
			},
			ID: req.ID,
		}, nil
	}

	result, err := handler(req.Method, req.Params)
	if err != nil {
		var he *HandlerError
		if errors.As(err, &he) && he.Code != "" {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				Error: &jsonRPCError{
					Code:    jsonRPCInternalError,
					Message: he.Message,
				},
				ID: req.ID,
			}, nil
		}
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error: &jsonRPCError{
				Code:    jsonRPCInternalError,
				Message: err.Error(),
			},
			ID: req.ID,
		}, nil
	}

	return jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}, nil
}
