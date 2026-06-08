package oauth

import (
	"encoding/json"
	"fmt"
	"sync"
)

// CLITool is a single tool's metadata + encrypted config.
type CLITool struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"`
}

// cliToolsListResponse is the response shape for /api/cli-tools GET.
type cliToolsListResponse struct {
	Tools []CLITool `json:"tools"`
}

// cliToolsUpdateRequest is the request body for /api/cli-tools PUT.
type cliToolsUpdateRequest struct {
	Tools []CLITool `json:"tools"`
}

// cliToolsUpdateResponse is the response shape for /api/cli-tools PUT.
type cliToolsUpdateResponse struct {
	Success bool `json:"success"`
	Updated int  `json:"updated"`
}

// cliToolsKVKey returns the canonical KV key for a given tool id.
func cliToolsKVKey(toolID string) string {
	return "tool:" + toolID
}

// KVListedEntry is a single key/value pair from a KV listing.
type KVListedEntry struct {
	Key   string
	Value string
}

// KVListing is the optional interface that KVRepo can implement to
// support listing all keys in a scope.
type KVListing interface {
	List(ctx Context) []KVListedEntry
}

var (
	kvListMu sync.RWMutex
	kvList   KVListing
)

// SetKVListing sets the KV listing helper.
func SetKVListing(r KVListing) {
	kvListMu.Lock()
	defer kvListMu.Unlock()
	kvList = r
}

func currentKVList() KVListing {
	kvListMu.RLock()
	defer kvListMu.RUnlock()
	return kvList
}

// decryptValue is the value decryptor. Tests use the default (identity);
// production overrides via SetValueDecryptor.
var decryptValue = func(v string) string { return v }

// SetValueDecryptor overrides the decryptor. Pass nil to restore identity.
func SetValueDecryptor(fn func(string) string) {
	if fn == nil {
		decryptValue = func(v string) string { return v }
		return
	}
	decryptValue = fn
}

// parseStoredCLITool reads a stored entry and decodes it into a CLITool.
func parseStoredCLITool(key, value string) (CLITool, error) {
	if len(key) < len("tool:") {
		return CLITool{}, fmt.Errorf("invalid tool key %q", key)
	}
	toolID := key[len("tool:"):]
	decoded := decryptValue(value)
	var raw struct {
		Name    string          `json:"name"`
		Enabled bool            `json:"enabled"`
		Config  json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal([]byte(decoded), &raw); err != nil {
		return CLITool{}, fmt.Errorf("tool %q: invalid stored config: %w", toolID, err)
	}
	return CLITool{
		ID:      toolID,
		Name:    raw.Name,
		Enabled: raw.Enabled,
		Config:  raw.Config,
	}, nil
}

// HandleCLIToolsList implements GET /api/cli-tools.
func HandleCLIToolsList(c *Context) (any, error) {
	kv := currentKVRepo()
	if kv == nil {
		return cliToolsListResponse{Tools: []CLITool{}}, nil
	}
	listing := currentKVList()
	if listing == nil {
		return cliToolsListResponse{Tools: []CLITool{}}, nil
	}

	entries := listing.List(*c)
	tools := make([]CLITool, 0, len(entries))
	for _, e := range entries {
		tool, err := parseStoredCLITool(e.Key, e.Value)
		if err != nil {
			continue
		}
		tools = append(tools, tool)
	}
	return cliToolsListResponse{Tools: tools}, nil
}

// HandleCLIToolsUpdate implements PUT /api/cli-tools.
func HandleCLIToolsUpdate(c *Context) (any, error) {
	if len(c.Body) == 0 {
		return nil, NewHandlerError("BAD_REQUEST", "Request body is required")
	}
	var req cliToolsUpdateRequest
	if err := json.Unmarshal(c.Body, &req); err != nil {
		return nil, NewHandlerError("BAD_REQUEST", "Invalid JSON body")
	}
	for i, t := range req.Tools {
		if t.ID == "" {
			return nil, NewHandlerError("BAD_REQUEST", fmt.Sprintf("tool[%d].id is required", i))
		}
	}

	kv := currentKVRepo()
	if kv == nil {
		return cliToolsUpdateResponse{Success: true, Updated: len(req.Tools)}, nil
	}

	for _, t := range req.Tools {
		stored := struct {
			Name    string          `json:"name"`
			Enabled bool            `json:"enabled"`
			Config  json.RawMessage `json:"config"`
		}{
			Name:    t.Name,
			Enabled: t.Enabled,
			Config:  t.Config,
		}
		data, err := json.Marshal(stored)
		if err != nil {
			return nil, NewHandlerError("BAD_REQUEST", fmt.Sprintf("tool %q: marshal failed: %v", t.ID, err))
		}
		encrypted := encryptJSONValue(string(data))
		if err := kv.Set(c.Ctx, "cli-tools", cliToolsKVKey(t.ID), encrypted, nil); err != nil {
			return nil, NewHandlerError("DB_ERROR", fmt.Sprintf("tool %q: save failed: %v", t.ID, err))
		}
	}
	return cliToolsUpdateResponse{Success: true, Updated: len(req.Tools)}, nil
}
