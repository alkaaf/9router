package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/9router/9router/internal/model"
	"gorm.io/gorm"
)

var allowedNodeTypes = map[string]bool{
	"openai-compatible":     true,
	"anthropic-compatible": true,
	"custom-embedding":     true,
}

// ProviderNodesHandler returns an http.HandlerFunc for GET/POST
// /api/provider-nodes.
func ProviderNodesHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleProviderNodesGet(w, db)
		case http.MethodPost:
			handleProviderNodesPost(w, r, db)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handleProviderNodesGet(w http.ResponseWriter, db *gorm.DB) {
	var rows []model.ProviderNode
	if err := db.Table("providerNodes").Order("createdAt ASC").Find(&rows).Error; err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, n := range rows {
		out = append(out, providerNodeToView(n))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"nodes": out})
}

type providerNodePostBody struct {
	Name    string `json:"name"`
	Prefix  string `json:"prefix"`
	Type    string `json:"type"`
	BaseURL string `json:"baseUrl"`
	APIType string `json:"apiType"`
}

func handleProviderNodesPost(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var body providerNodePostBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "name is required")
		return
	}
	if strings.TrimSpace(body.Prefix) == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "prefix is required")
		return
	}
	if !allowedNodeTypes[body.Type] {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid type")
		return
	}
	if strings.TrimSpace(body.BaseURL) == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "baseUrl is required")
		return
	}

	sanitized := sanitiseBaseURL(body.BaseURL, body.Type)
	nodeType := body.Type
	nodeName := body.Name
	prefix := body.Prefix
	apiType := body.APIType
	dataPayload := map[string]any{
		"prefix":  prefix,
		"baseUrl": sanitized,
	}
	if apiType != "" {
		dataPayload["apiType"] = apiType
	}
	dataBuf, _ := json.Marshal(dataPayload)
	id := generateNodeID(body.Type)

	node := model.ProviderNode{
		ID:        id,
		Type:      &nodeType,
		Name:      &nodeName,
		Data:      string(dataBuf),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Table("providerNodes").Create(&node).Error; err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"node": providerNodeToView(node)})
}

func providerNodeToView(n model.ProviderNode) map[string]any {
	view := map[string]any{
		"id":   n.ID,
		"name": strDeref(n.Name),
		"data": n.Data,
	}
	if n.Type != nil {
		view["type"] = *n.Type
	}
	// Pull out prefix and baseUrl from Data for convenience.
	var d map[string]any
	if err := json.Unmarshal([]byte(n.Data), &d); err == nil {
		if p, ok := d["prefix"].(string); ok {
			view["prefix"] = p
		}
		if b, ok := d["baseUrl"].(string); ok {
			view["baseUrl"] = b
		}
	}
	return view
}

func sanitiseBaseURL(baseURL, nodeType string) string {
	out := strings.TrimRight(baseURL, "/")
	switch nodeType {
	case "anthropic-compatible":
		out = strings.TrimSuffix(out, "/messages")
	case "custom-embedding":
		out = strings.TrimSuffix(out, "/embeddings")
	}
	return out
}

func generateNodeID(nodeType string) string {
	var prefix string
	switch nodeType {
	case "openai-compatible":
		prefix = "openai-compatible"
	case "anthropic-compatible":
		prefix = "anthropic-compatible"
	case "custom-embedding":
		prefix = "custom-embedding"
	default:
		prefix = "node"
	}
	return prefix + "-" + randomToken(8)
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
