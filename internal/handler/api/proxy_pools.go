package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/9router/9router/internal/model"
	"gorm.io/gorm"
)

var allowedProxyPoolTypes = map[string]bool{
	"http":       true,
	"socks5":     true,
	"cloudflare": true,
	"vercel":     true,
	"deno":       true,
}

// ProxyPoolsHandler returns an http.HandlerFunc for GET/POST
// /api/proxy-pools.
func ProxyPoolsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleProxyPoolsGet(w, r, db)
		case http.MethodPost:
			handleProxyPoolsPost(w, r, db)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handleProxyPoolsGet(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var rows []model.ProxyPool
	q := db.Table("proxyPools").Order("createdAt ASC")

	if v := r.URL.Query().Get("isActive"); v != "" {
		active, err := strconv.ParseBool(v)
		if err == nil {
			q = q.Where("isActive = ?", active)
		}
	}
	if err := q.Find(&rows).Error; err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	includeUsage := r.URL.Query().Get("includeUsage") == "true"

	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		view := proxyPoolToView(p)
		if includeUsage {
			view["boundConnectionCount"] = countBoundConnections(db, p.ID)
		}
		out = append(out, view)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"proxyPools": out})
}

type proxyPoolPostBody struct {
	Name        string `json:"name"`
	ProxyURL    string `json:"proxyUrl"`
	Type        string `json:"type"`
	NoProxy     string `json:"noProxy"`
	IsActive    *bool  `json:"isActive"`
	StrictProxy *bool  `json:"strictProxy"`
}

func handleProxyPoolsPost(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var body proxyPoolPostBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "name is required")
		return
	}
	if strings.TrimSpace(body.ProxyURL) == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "proxyUrl is required")
		return
	}
	// Apply defaults
	ptype := body.Type
	if ptype == "" {
		ptype = "http"
	}
	if !allowedProxyPoolTypes[ptype] {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid type")
		return
	}
	active := true
	if body.IsActive != nil {
		active = *body.IsActive
	}
	strict := false
	if body.StrictProxy != nil {
		strict = *body.StrictProxy
	}
	noProxy := body.NoProxy

	data := map[string]any{
		"name":        body.Name,
		"proxyUrl":    body.ProxyURL,
		"type":        ptype,
		"noProxy":     noProxy,
		"strictProxy": strict,
	}
	buf, _ := json.Marshal(data)

	id := "pool-" + randomToken(8)
	pool := model.ProxyPool{
		ID:        id,
		IsActive:  &active,
		Data:      string(buf),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Table("proxyPools").Create(&pool).Error; err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"proxyPool": proxyPoolToView(pool)})
}

func proxyPoolToView(p model.ProxyPool) map[string]any {
	out := map[string]any{
		"id": p.ID,
	}
	if p.IsActive != nil {
		out["isActive"] = *p.IsActive
	}
	if p.TestStatus != nil {
		out["testStatus"] = *p.TestStatus
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(p.Data), &d); err == nil {
		for k, v := range d {
			out[k] = v
		}
	}
	return out
}

func countBoundConnections(db *gorm.DB, poolID string) int {
	var n int64
	db.Table("providerConnections").
		Where("data LIKE ?", "%\"proxyPoolId\":\""+poolID+"\"%").
		Count(&n)
	return int(n)
}
