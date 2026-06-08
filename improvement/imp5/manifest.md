# 9Router Backend Rewrite — Manifest

**Version:** 1.1
**Date:** 2026-06-04
**Goal:** Rewrite Node.js backend to Go while keeping Next.js frontend
**Scope:** AI routing gateway, MITM proxy, provider management, usage tracking

---

## 1. Executive Summary

9Router adalah AI infrastructure management tool yang berfungsi sebagai:
- **Single endpoint** untuk semua AI provider (OpenAI, Anthropic, Google, Azure, dll)
- **MITM proxy** yang intercepts HTTPS traffic dari AI IDE (Antigravity, Copilot, Kiro, Cursor)
- **Load balancer** dengan combo strategies (fallback, round-robin, sticky)
- **Usage tracker** dengan cost aggregation per provider/model

**Technical Stack Current:**
- Backend: Next.js (Express + custom handlers) + Node.js MITM
- Frontend: Next.js 16 + React 19 + Tailwind CSS 4
- Database: SQLite (sql.js) / PostgreSQL (pg)
- ORM: Raw SQL queries manual
- State: Zustand

**Technical Stack Target:**
- Backend: Go (Fiber framework)
- Frontend: Next.js 16 + React 19 (unchanged)
- Database: GORM (`gorm.io/gorm`) + SQLite (`gorm.io/driver/sqlite`) + PostgreSQL (`gorm.io/driver/postgres`)
- Protocol: REST API + SSE streaming

---

## 2. Scope Definition

### 2.1 In Scope

#### A. REST API Endpoints (126 routes)
```
Dashboard API (/api/*) — 44 routes
├── /api/auth/*              → login, logout, OIDC, status
├── /api/providers/*         → CRUD, test, models, validate
├── /api/combos/*            → CRUD
├── /api/keys/*              → API key management
├── /api/settings/*          → App settings
├── /api/provider-nodes/*    → Node configuration
├── /api/proxy-pools/*      → Proxy pool management
├── /api/pricing/*           → Pricing config
├── /api/tags/*              → Tags
├── /api/usage/*            → Usage tracking & charts
├── /api/tunnel/*            → Cloudflare tunnel management
├── /api/translator/*        → Translator UI backend
├── /api/mcp/*               → MCP server integration
├── /api/media-providers/*   → TTS/STT voices
├── /api/cli-tools/*         → CLI tool settings
├── /api/oauth/*             → OAuth flows (GitLab, Kiro, Cursor, iFlow, Codex)
├── /api/locale              → i18n
├── /api/shutdown            → Server shutdown
└── /api/health, /api/init, /api/version

Public LLM API (/v1/*) — 14 routes
├── POST /v1/chat/completions  → Core chat endpoint (MOST CRITICAL)
├── POST /v1/messages          → Anthropic format
├── POST /v1/embeddings        → Embeddings
├── POST /v1/responses        → OpenAI Responses API
├── POST /v1/search            → Web search
├── POST /v1/web/fetch         → URL content fetch
├── POST /v1/audio/speech      → TTS
├── POST /v1/audio/transcriptions → STT
├── GET  /v1/models            → Model list
├── GET  /v1/models/[kind]     → Model list by kind
├── POST /v1/images/generations → Image generation
├── POST /v1beta/models        → Beta compatibility
└── POST /v1/api/chat          → Internal chat
```

#### B. AI Proxy Engine (open-sse/)
```
Executors (19 providers):
├── antigravity      → Gemini format
├── azure            → Microsoft Azure OpenAI
├── gemini-cli       → Google Gemini CLI
├── github           → GitHub Models
├── kiro             → Kiro CodeWhisperer
├── codex            → OpenAI Codex
├── cursor           → Cursor (placeholder)
├── vertex           → Google Vertex AI
├── vertex-partner   → Vertex partner
├── qwen             → Alibaba Qwen
├── opencode         → OpenCode
├── opencode-go      → OpenCode Go
├── grok-web         → xAI Grok
├── perplexity-web   → Perplexity
├── ollama-local     → Local Ollama
├── commandcode      → CommandCode
├── iflow            → iFlow
├── qoder            → Qoder AI
└── default          → Generic OpenAI-compatible

Core Services:
├── chatCore          → Request routing, credential management
├── embeddingsCore    → Embedding generation
├── imageGenerationCore → Image generation
├── fetchCore         → Web fetch (Perplexity)
├── ttsCore           → Text-to-speech
├── sttCore           → Speech-to-text
└── comboService      → Multi-model fallback/round-robin

Translators:
├── anthropic_to_openai  → Claude → OpenAI format
├── gemini_to_openai     → Gemini → OpenAI format
├── openai_to_anthropic  → OpenAI → Claude format
└── kiro_eventstream     → AWS EventStream binary conversion

RTK (Response Transform Kit):
├── caveman          → Context compression
├── autodetect       → Auto-detect format
└── filters/*        → Token-saving transforms
```

#### C. Database Layer
```
Tables (10):
├── _meta             → Key-value metadata
├── settings          → App settings (single row)
├── providerConnections → AI provider credentials
├── providerNodes      → Provider configurations
├── proxyPools         → Proxy pool definitions
├── apiKeys            → User API keys
├── combos             → Model combos
├── kv                 → Key-value store
├── usageHistory       → Per-request usage logs
├── usageDaily         → Daily aggregates
└── requestDetails     → Request metadata

ORM Layer:
├── GORM model definitions dengan struct tags
├── Auto-migration via db.AutoMigrate(&Model{})
├── SQLite driver (gorm.io/driver/sqlite) — single-user
├── PostgreSQL driver (gorm.io/driver/postgres) — production
└── Connection string dari environment variable
```

#### D. MITM Proxy (Node.js — keep as-is, adapt target)
```
Features:
├── HTTPS server dengan dynamic SNI
├── HTTP/2 passthrough dengan ALPN negotiation
├── Certificate generation per domain
├── Tool-specific handlers:
│   ├── antigravity.js → Gemini IDE
│   ├── copilot.js → GitHub Copilot
│   ├── kiro.js → Kiro IDE (CodeWhisperer)
│   └── cursor.js → Cursor (placeholder)
└── DNS-based routing

Target: Change ROUTER_BASE from Next.js to Go server
```

#### E. State Management
```
Zustand Stores (7):
├── themeStore        → Theme preference
├── userStore         → User state
├── providerStore     → Provider list + caching
├── notificationStore → Notifications
├── settingsStore     → Settings
├── headerSearchStore → Search state
└── CLIENT_STORE_TTL_MS → Cache TTL (5 min default)
```

### 2.2 Out of Scope

- Frontend rewrite (Next.js stays)
- MITM server rewrite (keep in Node.js, change target)
- CLI rewrite (terminal UI, update base URL only)
- Build/deploy scripts
- Docker/K8s configuration

---

## 3. Architecture

### 3.1 Target Architecture

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                      Client                            │
                    │   (AI IDE: Antigravity, Copilot, Kiro, Cursor, etc)   │
                    └─────────────────────────────────────────────────────────┘
                                         │
                                         │ HTTPS (MITM intercepts)
                                         ▼
                    ┌─────────────────────────────────────────────────────────┐
                    │                   MITM Server (Node.js)                │
                    │            Port 443, dynamic SNI certs                 │
                    │              Tool-specific handlers                    │
                    └─────────────────────────────────────────────────────────┘
                                         │ HTTP
                                         │ fetchRouter → http://localhost:8090
                                         ▼
                    ┌─────────────────────────────────────────────────────────┐
                    │                Go API Server (Fiber)                  │
                    │                      Port 8090                         │
                    ├─────────────────────────────────────────────────────────┤
                    │  Middleware: CORS, Auth (JWT/CLI), Logging, Metrics   │
                    │  Recovery, RequestID                                   │
                    ├─────────────────────────────────────────────────────────┤
                    │  Routes:                                                │
                    │    /v1/*  → LLM Proxy (streaming SSE)                 │
                    │    /api/* → Dashboard CRUD                            │
                    │    /health, /metrics                                   │
                    ├─────────────────────────────────────────────────────────┤
                    │  Services:                                             │
                    │    ├── chatHandler     → /v1/chat/completions          │
                    │    ├── embeddingsHandler → /v1/embeddings             │
                    │    ├── providersService → Provider management         │
                    │    ├── combosService    → Combo strategies             │
                    │    ├── authService     → JWT + API key validation     │
                    │    ├── usageService    → Usage tracking               │
                    │    └── tunnelService   → Cloudflare tunnel            │
                    ├─────────────────────────────────────────────────────────┤
                    │  Executors (19 providers):                              │
                    │    ├── antigravity.go   → Gemini                       │
                    │    ├── azure.go        → Azure OpenAI                 │
                    │    ├── anthropic.go    → Claude                       │
                    │    ├── gemini.go       → Gemini                        │
                    │    ├── openai.go       → OpenAI                       │
                    │    └── ...                                                │
                    ├─────────────────────────────────────────────────────────┤
                    │  GORM Database Layer:                                  │
                    │    ├── models/ (struct definitions)                    │
                    │    ├── repository/ (CRUD operations)                   │
                    │    └── db.go (AutoMigrate + connection)                │
                    └─────────────────────────────────────────────────────────┘
                                         │
                    ┌────────────────────┴────────────────────┐
                    │                                     │
                    ▼                                     ▼
         ┌─────────────────────┐            ┌─────────────────────┐
         │    SQLite (local)    │            │   PostgreSQL (prod) │
         │   9router.db        │            │   Connection pool   │
         └─────────────────────┘            └─────────────────────┘
```

### 3.2 Directory Structure (Go)

```
9router/
├── cmd/
│   └── server/
│       └── main.go                 # Entry point: init GORM, setup Fiber, start
├── internal/
│   ├── server/
│   │   ├── router.go              # Fiber router setup
│   │   ├── middleware/
│   │   │   ├── auth.go            # JWT/CLI/API key middleware
│   │   │   ├── cors.go            # CORS config
│   │   │   ├── logger.go          # Request logging
│   │   │   ├── recovery.go        # Panic recovery
│   │   │   └── requestid.go       # Request ID injection
│   │   └── graceful.go            # Graceful shutdown
│   ├── handler/
│   │   ├── api/
│   │   │   ├── auth.go            # /api/auth/*
│   │   │   ├── providers.go       # /api/providers/*
│   │   │   ├── combos.go          # /api/combos/*
│   │   │   ├── keys.go            # /api/keys/*
│   │   │   ├── settings.go        # /api/settings/*
│   │   │   ├── usage.go           # /api/usage/*
│   │   │   ├── tunnel.go          # /api/tunnel/*
│   │   │   ├── translator.go      # /api/translator/*
│   │   │   ├── mcp.go             # /api/mcp/*
│   │   │   └── cli-tools.go       # /api/cli-tools/*
│   │   ├── v1/
│   │   │   ├── chat.go            # /v1/chat/completions
│   │   │   ├── messages.go        # /v1/messages
│   │   │   ├── embeddings.go      # /v1/embeddings
│   │   │   ├── responses.go       # /v1/responses
│   │   │   ├── search.go          # /v1/search
│   │   │   ├── webfetch.go        # /v1/web/fetch
│   │   │   ├── audio.go           # /v1/audio/*
│   │   │   ├── images.go          # /v1/images/generations
│   │   │   └── models.go          # /v1/models
│   │   └── health.go              # /health, /metrics
│   ├── executor/
│   │   ├── registry.go            # Executor factory
│   │   ├── base.go               # Executor interface + base impl
│   │   ├── openai.go             # OpenAI-compatible
│   │   ├── azure.go              # Azure OpenAI
│   │   ├── anthropic.go          # Anthropic Claude
│   │   ├── gemini.go             # Google Gemini
│   │   ├── vertex.go             # Google Vertex
│   │   ├── github.go             # GitHub Models
│   │   ├── kiro.go               # Kiro CodeWhisperer
│   │   ├── grok.go               # xAI Grok
│   │   ├── qwen.go               # Alibaba Qwen
│   │   ├── perplexity.go         # Perplexity
│   │   ├── ollama.go             # Local Ollama
│   │   └── ...
│   ├── translator/
│   │   ├── anthropic.go          # Anthropic ↔ OpenAI
│   │   ├── gemini.go             # Gemini ↔ OpenAI
│   │   ├── responses.go          # Responses API ↔ Chat
│   │   └── kiro.go               # AWS EventStream handling
│   ├── rtk/
│   │   ├── caveman.go            # Context compression
│   │   └── filters.go            # Token-saving filters
│   ├── combo/
│   │   ├── strategy.go           # Strategy interface
│   │   ├── fallback.go           # Fallback strategy
│   │   ├── roundrobin.go         # Round-robin strategy
│   │   └── sticky.go             # Sticky session strategy
│   ├── repository/
│   │   ├── db.go                 # GORM connection & auto-migrate
│   │   ├── provider.go           # Provider repo
│   │   ├── proto.go              # ProviderNode repo
│   │   ├── proxypool.go          # ProxyPool repo
│   │   ├── combo.go              # Combo repo
│   │   ├── settings.go           # Settings repo
│   │   ├── apikey.go             # API key repo
│   │   ├── usage.go              # Usage tracking repo
│   │   ├── kv.go                 # Key-value repo
│   │   └── meta.go               # Metadata repo
│   ├── model/
│   │   ├── provider.go           # GORM model
│   │   ├── provider_node.go      # GORM model
│   │   ├── proxy_pool.go         # GORM model
│   │   ├── api_key.go            # GORM model
│   │   ├── combo.go              # GORM model
│   │   ├── setting.go            # GORM model
│   │   ├── usage_history.go      # GORM model
│   │   ├── usage_daily.go        # GORM model
│   │   ├── request_detail.go     # GORM model
│   │   ├── kv.go                 # GORM model
│   │   └── meta.go               # GORM model
│   ├── auth/
│   │   ├── jwt.go                # JWT validation (golang-jwt)
│   │   ├── apikey.go             # API key validation
│   │   ├── cli.go                # CLI token validation
│   │   └── oauth.go              # OAuth flows
│   ├── proxy/
│   │   └── tunnel.go             # Cloudflare tunnel
│   └── config/
│       └── config.go             # Env-based configuration
├── pkg/
│   ├── response/
│   │   ├── sse.go                # SSE streaming helpers
│   │   └── error.go              # Standardized error response
│   └── util/
│       └── crypto.go             # Shared crypto helpers
├── go.mod
├── go.sum
└── Makefile
```

### 3.3 Technology Decision — Fiber

```
Alasan memilih Fiber dibanding Gin:
  - API lebih ringkas dan ekspresif (mirip Express.js)
  - Performa lebih tinggi (faster routing via fasthttp)
  - Lebih mudah dipahami developer yang familiar Express
  - Middleware built-in (recovery, requestID, cors)
  - Context methods lebih lengkap (c.JSON, c.SendStream, c.Next)
  - Community plugins untuk session, caching, helmet

Contoh route setup:
├── app := fiber.New()
├── app.Use(middleware.Recovery())
├── app.Use(middleware.RequestID())
├── app.Use(middleware.Logger())
│
├── api := app.Group("/api")
├── api.Post("/auth/login", handler.Login)
├── api.Get("/providers", middleware.Auth, handler.ListProviders)
│
├── v1 := app.Group("/v1")
├── v1.Post("/chat/completions", handler.ChatCompletions)
│
└── app.Get("/health", handler.Health)
```

### 3.4 Technology Decision — GORM

```
Alasan memilih GORM:
  - Auto-migration: model struct → create/alter table otomatis
  - Dual-driver: SQLite + PostgreSQL via driver yang sama
  - Query builder: Where, Joins, Preload, Scopes
  - Hook system: BeforeCreate, AfterFind, dll
  - Connection pooling built-in
  - Migrations: AutoMigrate + Migrator interface
  - Relation management: BelongsTo, HasMany, ManyToMany

GORM AutoMigrate:
├── db, _ := gorm.Open(driver, dsn)
├── db.AutoMigrate(
│   ├── &model.ProviderConnection{},
│   ├── &model.ProviderNode{},
│   ├── &model.ProxyPool{},
│   ├── &model.ApiKey{},
│   ├── &model.Combo{},
│   ├── &model.Setting{},
│   ├── &model.UsageHistory{},
│   ├── &model.UsageDaily{},
│   ├── &model.RequestDetail{},
│   ├── &model.KV{},
│   └── &model.Meta{},
│ )

Driver Selection:
├── SQLite:   gorm.Open(sqlite.Open("data/9router.db"))
└── Postgres: gorm.Open(postgres.Open(connString))
```

---

## 4. Database Layer Detail

### 4.1 GORM Model Definitions

```go
// model/provider.go
type ProviderConnection struct {
    ID         string `gorm:"primaryKey;type:text"`
    Provider   string `gorm:"not null;index:idx_pc_provider;index:idx_pc_provider_active,priority:1;index:idx_pc_priority,priority:1"`
    AuthType   string `gorm:"not null"`
    Name       *string
    Email      *string
    Priority   *int
    IsActive   *bool  `gorm:"default:1"`
    Data       string `gorm:"not null;type:text"`  // JSON string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

// model/combo.go
type Combo struct {
    ID        string `gorm:"primaryKey;type:text"`
    Name      string `gorm:"uniqueIndex;not null;type:text"`
    Kind      *string
    Models    string `gorm:"not null;type:text"`  // JSON array
    CreatedAt time.Time
    UpdatedAt time.Time
}

// model/usage_history.go
type UsageHistory struct {
    ID               uint      `gorm:"primaryKey;autoIncrement"`
    Timestamp        time.Time `gorm:"not null;index:idx_uh_ts;index:idx_uh_provider_ts,priority:1;index:idx_uh_model_ts,priority:1;index:idx_uh_conn_ts,priority:1;index:idx_uh_apiKey_ts,priority:1"`
    Provider         *string   `gorm:"index:idx_uh_provider;index:idx_uh_provider_ts,priority:2"`
    Model            *string   `gorm:"index:idx_uh_model;index:idx_uh_model_ts,priority:2"`
    ConnectionID     *string   `gorm:"index:idx_uh_conn;index:idx_uh_conn_ts,priority:2"`
    ApiKey           *string   `gorm:"index:idx_uh_apiKey;index:idx_uh_apiKey_ts,priority:2"`
    Endpoint         *string
    PromptTokens     int       `gorm:"default:0"`
    CompletionTokens int       `gorm:"default:0"`
    Cost             float64   `gorm:"default:0"`
    Status           *string
    Tokens           *string   `gorm:"type:text"`   // JSON
    Meta             *string   `gorm:"type:text"`   // JSON
}

// model/setting.go
type Setting struct {
    ID   uint   `gorm:"primaryKey;check:id = 1"`
    Data string `gorm:"not null;type:text"`  // JSON
}

// model/meta.go
type Meta struct {
    Key   string `gorm:"primaryKey;type:text"`
    Value string `gorm:"not null;type:text"`
}

// model/kv.go
type KV struct {
    Scope string `gorm:"primaryKey;type:text"`
    Key   string `gorm:"primaryKey;type:text"`
    Value string `gorm:"not null;type:text"`
}

// model/api_key.go
type ApiKey struct {
    ID        string    `gorm:"primaryKey;type:text"`
    Key       string    `gorm:"uniqueIndex;not null;type:text"`
    Name      *string
    MachineID *string
    IsActive  *bool     `gorm:"default:1"`
    CreatedAt time.Time
}

// model/provider_node.go
type ProviderNode struct {
    ID        string    `gorm:"primaryKey;type:text"`
    Type      *string   `gorm:"index"`
    Name      *string
    Data      string    `gorm:"not null;type:text"`
    CreatedAt time.Time
    UpdatedAt time.Time
}

// model/proxy_pool.go
type ProxyPool struct {
    ID         string    `gorm:"primaryKey;type:text"`
    IsActive   *bool     `gorm:"default:1;index"`
    TestStatus *string   `gorm:"index"`
    Data       string    `gorm:"not null;type:text"`
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

// model/usage_daily.go
type UsageDaily struct {
    DateKey string `gorm:"primaryKey;type:text"`
    Data    string `gorm:"not null;type:text"`
}

// model/request_detail.go
type RequestDetail struct {
    ID           string    `gorm:"primaryKey;type:text"`
    Timestamp    time.Time `gorm:"not null;index:idx_rd_ts"`
    Provider     *string   `gorm:"index:idx_rd_provider"`
    Model        *string   `gorm:"index:idx_rd_model"`
    ConnectionID *string   `gorm:"index:idx_rd_conn"`
    Status       *string
    Data         string    `gorm:"not null;type:text"`
}
```

### 4.2 Repository Layer

Setiap tabel memiliki repository struct dengan operasi CRUD via GORM:

```go
// repository/provider.go
type ProviderRepository struct {
    db *gorm.DB
}

func (r *ProviderRepository) FindAll() ([]model.ProviderConnection, error) {
    var providers []model.ProviderConnection
    result := r.db.Order("priority ASC").Find(&providers)
    return providers, result.Error
}

func (r *ProviderRepository) FindByID(id string) (*model.ProviderConnection, error) {
    var provider model.ProviderConnection
    result := r.db.First(&provider, "id = ?", id)
    if errors.Is(result.Error, gorm.ErrRecordNotFound) {
        return nil, nil
    }
    return &provider, result.Error
}

func (r *ProviderRepository) Create(provider *model.ProviderConnection) error {
    return r.db.Create(provider).Error
}
```

### 4.3 JSON Data Columns

Kolom `data` pada tabel tertentu menyimpan JSON string. Strategi:

```go
// Model struct field: string (raw JSON)
// Helper untuk parse/get:
type ProviderConnectionData struct {
    AccessToken     string `json:"accessToken"`
    RefreshToken    string `json:"refreshToken"`
    ExpiresAt       int64  `json:"expiresAt"`
    ProjectID       string `json:"projectId"`
    ProviderData    map[string]any `json:"providerSpecificData"`
}

func (m *ProviderConnection) GetData() (*ProviderConnectionData, error) {
    var d ProviderConnectionData
    err := json.Unmarshal([]byte(m.Data), &d)
    return &d, err
}

func (m *ProviderConnection) SetData(d *ProviderConnectionData) error {
    bytes, err := json.Marshal(d)
    if err != nil {
        return err
    }
    m.Data = string(bytes)
    return nil
}
```

---

## 5. Phases

### Phase 0: Reverse Engineering & Spec (10-14 days)

**Goal:** Document current behavior before writing code.

#### 0.1 API Contract Spec (3 days)
```
Tasks:
├── [ ] Document all 126 API endpoints (method, path, request, response)
├── [ ] Create OpenAPI 3.0 spec
├── [ ] Document error formats per status code
├── [ ] Document auth requirements per endpoint
└── [ ] Validate with existing frontend

Output: openapi.yaml
```

#### 0.2 Database Schema Spec (2 days)
```
Tasks:
├── [ ] Extract schema from src/lib/db/schema.js
├── [ ] Document all 10 tables (columns, indexes, constraints)
├── [ ] Document JSON format in data columns
├── [ ] Map SQLite → PostgreSQL differences
└── [ ] Convert to GORM model definitions

Output: model definitions in Go structs
```

#### 0.3 Model Routing Spec (2 days)
```
Tasks:
├── [ ] Document model → provider resolution logic
├── [ ] Document combo expansion
├── [ ] Document combo strategies (fallback, round-robin, sticky)
├── [ ] Document account priority & rate-limit handling
└── [ ] Create flow diagrams

Output: routing.md with diagrams
```

#### 0.4 Format Translation Spec (2-3 days)
```
Tasks:
├── [ ] Document OpenAI ↔ Anthropic ↔ Gemini format conversion
├── [ ] Document SSE streaming format
├── [ ] Document tool_calls streaming format
├── [ ] Document Kiro AWS EventStream format
└── [ ] Create translation matrix

Output: translator.md
```

#### 0.5 Credential Management Spec (2 days)
```
Tasks:
├── [ ] Document auth types per provider (api-key, oauth, cookie, pat)
├── [ ] Document token refresh flows per provider
├── [ ] Document account state tracking (available, rate-limited, error)
├── [ ] Document cooldown/exponential backoff calculation
└── [ ] Document callback patterns (onCredentialsRefreshed, onRequestSuccess)

Output: credentials.md
```

---

### Phase 1: Core Infrastructure (2-3 weeks)

**Goal:** Setup Go project, GORM database layer, auth middleware.

#### 1.1 Project Setup (2 days)
```
Tasks:
├── [ ] Initialize Go module: go mod init github.com/9router/backend
├── [ ] Setup directory structure (per Architecture section)
├── [ ] Install dependencies:
│   ├── go get github.com/gofiber/fiber/v2
│   ├── go get gorm.io/gorm
│   ├── go get gorm.io/driver/sqlite
│   ├── go get gorm.io/driver/postgres
│   ├── go get github.com/golang-jwt/jwt/v5
│   ├── go get golang.org/x/crypto (bcrypt)
│   └── go get github.com/joho/godotenv
├── [ ] Configure Makefile (dev, build, test, lint)
├── [ ] Setup golangci-lint configuration
├── [ ] Add .env.example with all environment variables
└── [ ] Add git hooks (pre-commit lint)

Deliverables:
- go.mod, go.sum
- Makefile
- .golangci.yml
- .env.example

Go Dependencies:
┌────────────────────────┬────────────────────────────────────┐
│ Package                │ Kegunaan                            │
├────────────────────────┼────────────────────────────────────┤
│ gofiber/fiber/v2       │ Web framework (router, middleware) │
│ gorm.io/gorm           │ ORM (database abstraction)         │
│ gorm.io/driver/sqlite  │ SQLite driver                      │
│ gorm.io/driver/postgres│ PostgreSQL driver                   │
│ golang-jwt/jwt/v5     │ JWT create & verify                 │
│ golang.org/x/crypto    │ bcrypt password hashing             │
│ joho/godotenv          │ .env file loader                    │
│ go-playground/validator│ Struct validation                   │
└────────────────────────┴────────────────────────────────────┘
```

#### 1.2 Configuration (1 day)
```
Tasks:
├── [ ] Create config package (env vars → struct)
├── [ ] Support modes: dev, production
├── [ ] Configure logging (zerolog or standard log)
└── [ ] Add health check endpoint

Deliverables:
- internal/config/config.go

Config struct:
├── type Config struct {
│   ├── Port       string  // default: "8090"
│   ├── DBDriver   string  // "sqlite" | "postgres"
│   ├── DBPath     string  // SQLite: "data/9router.db"
│   ├── DatabaseURL string // PostgreSQL conn string
│   ├── JWTSecret  string
│   ├── LogLevel   string
│   ├── Environment string // "dev" | "production"
│   └── RouterBaseURL string // for MITM redirect
│ }
```

#### 1.3 Database Layer with GORM (3-4 days)
```
Tasks:
├── [ ] Define all GORM models di internal/model/
├── [ ] Create GORM connection helper (db.go)
├── [ ] Auto-migration setup (AutoMigrate all models)
├── [ ] Create repository layer untuk setiap tabel
├── [ ] Implement JSON data column helpers
├── [ ] Add GORM hooks (BeforeCreate untuk UUID generation)
├── [ ] Connection pooling config

Deliverables:
- internal/model/*.go (11 models)
- internal/repository/db.go (GORM init + AutoMigrate)
- internal/repository/provider.go
- internal/repository/combo.go
- internal/repository/apikey.go
- internal/repository/settings.go
- internal/repository/usage.go
- internal/repository/kv.go
- internal/repository/meta.go

GORM Connection:
├── func NewGormDB(cfg *config.Config) (*gorm.DB, error) {
│   ├── switch cfg.DBDriver {
│   │   case "sqlite":
│   │       return gorm.Open(sqlite.Open(cfg.DBPath))
│   │   case "postgres":
│   │       return gorm.Open(postgres.Open(cfg.DatabaseURL))
│   │   }
│   ├── db.AutoMigrate(all models...)
│   ├── db.Logger = logger.Default.LogMode(logger.Warn)
│   └── return db, nil
│ }
```

#### 1.4 Auth Middleware (2-3 days)
```
Tasks:
├── [ ] JWT validation middleware (Bearer token)
├── [ ] API key validation
├── [ ] CLI token validation (x-9r-cli-token header)
├── [ ] Public endpoint allowlist
├── [ ] Dashboard route protection
└── [ ] Fiber middleware integration

Deliverables:
- internal/middleware/auth.go

Auth Rules:
├── /v1/*         → API key OR JWT OR local (no auth)
├── /api/*        → JWT OR CLI token
├── /api/public   → no auth
├── /api/cli-tools/* → CLI token only
├── /health       → no auth
└── /dashboard/*  → JWT (redirect to /login)
```

---

### Phase 2: REST API Implementation (2-3 weeks)

**Goal:** Implement all dashboard CRUD endpoints with Fiber + GORM.

#### 2.1 Core CRUD Handlers (2 weeks)
```
Tasks:
├── [ ] /api/providers CRUD
├── [ ] /api/combos CRUD
├── [ ] /api/keys CRUD
├── [ ] /api/settings (GET/PATCH)
├── [ ] /api/provider-nodes CRUD
├── [ ] /api/proxy-pools CRUD
├── [ ] /api/pricing CRUD
└── [ ] /api/tags CRUD

Validation:
- Request body JSON schema (go-playground/validator)
- Error responses (400, 401, 404, 500)
- Success responses (200, 201)

Fiber Handler Pattern:
├── func ListProviders(c *fiber.Ctx) error {
│   ├── providers, err := providerRepo.FindAll()
│   ├── if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
│   └── return c.JSON(fiber.Map{"connections": providers})
│ }
```

#### 2.2 Auth Handlers (3 days)
```
Tasks:
├── [ ] POST /api/auth/login (bcrypt compare + JWT issue)
├── [ ] POST /api/auth/logout (clear cookie)
├── [ ] GET  /api/auth/status
├── [ ] OIDC flow endpoints
└── [ ] JWT cookie management (httpOnly, secure, sameSite)

Auth Modes:
- "password" → bcrypt hash comparison
- "oidc" → OIDC provider redirect
```

#### 2.3 Usage & Stats (3 days)
```
Tasks:
├── [ ] /api/usage/history (paginated, filtered)
├── [ ] /api/usage/stats (aggregate)
├── [ ] /api/usage/chart (time-series data)
├── [ ] /api/usage/per-key/[keyId]/*
├── [ ] /api/usage/providers
└── [ ] /api/usage/stream (SSE real-time via Fiber)

GORM Queries:
├── // Aggregate per provider
├── db.Model(&UsageHistory{}).
│   Select("provider, SUM(prompt_tokens) as prompt_tokens, SUM(cost) as cost").
│   Where("timestamp > ?", since).
│   Group("provider").
│   Scan(&results)
```

#### 2.4 Tunnel & System (3 days)
```
Tasks:
├── [ ] /api/tunnel/status (Cloudflare tunnel state)
├── [ ] /api/tunnel/enable
├── [ ] /api/tunnel/disable
├── [ ] /api/tunnel/tailscale-*
├── [ ] /api/shutdown (Fiber: app.Shutdown())
├── [ ] /api/init (app initialization state)
└── [ ] /api/version
```

#### 2.5 OAuth & Integration (3 days)
```
Tasks:
├── [ ] OAuth endpoints per provider (gitlab, kiro, cursor, iflow, codex)
├── [ ] MCP server integration routes
├── [ ] Media providers voices lookup
└── [ ] CLI tools settings proxy
```

---

### Phase 3: AI Proxy Engine (3-4 weeks)

**Goal:** Implement /v1/* streaming endpoints with all provider executors.

#### 3.1 Base Executor Interface (2 days)
```
Tasks:
├── [ ] Define Executor interface
│   └── type Executor interface {
│         Execute(ctx context.Context, req *Request) (*Response, error)
│         ExecuteStream(ctx context.Context, req *Request) (<-chan *Chunk, error)
│         GetProvider() string
│       }
├── [ ] Create base implementation
├── [ ] Add error mapping (provider error → HTTP status)
└── [ ] Add request timeout handling with context

Provider Error Mapping:
├── 400 → Bad request (invalid model, missing params)
├── 401 → Auth failed (invalid token, expired)
├── 403 → Forbidden (quota exceeded, region blocked)
├── 429 → Rate limited (retry after from header)
├── 500 → Internal error (provider down)
└── 503 → Service unavailable (all accounts failed)
```

#### 3.2 OpenAI Executor (2 days) — Reference Implementation
```
Tasks:
├── [ ] Implement OpenAI executor
├── [ ] Handle streaming (SSE)
├── [ ] Handle tool_calls streaming
├── [ ] Handle errors and retry
└── [ ] Add usage tracking (prompt/completion tokens)
```

#### 3.3 Anthropic Executor (2 days)
```
Tasks:
├── [ ] Implement Anthropic executor
├── [ ] Translate OpenAI → Anthropic format
├── [ ] Handle streaming
├── [ ] Handle thinking (extended thinking)
├── [ ] Handle tool_use
└── [ ] Map Anthropic errors → standard format
```

#### 3.4 Gemini Executor (2 days)
```
Tasks:
├── [ ] Implement Gemini executor
├── [ ] Translate OpenAI → Gemini format (contents, parts)
├── [ ] Handle streaming (server side)
├── [ ] Handle function declarations
└── [ ] Map Gemini errors → standard format
```

#### 3.5 Azure Executor (1 day)
```
Tasks:
├── [ ] Implement Azure OpenAI executor
├── [ ] Handle Azure-specific auth (API key in header)
├── [ ] Map deployment name → endpoint
└── [ ] Handle Azure errors
```

#### 3.6 Remaining Executors (5-7 days)
```
Tasks:
├── [ ] Vertex (2 days)
├── [ ] GitHub Models (1 day)
├── [ ] Kiro CodeWhisperer (2 days)
├── [ ] Grok (1 day)
├── [ ] Qwen (1 day)
├── [ ] Perplexity (1 day)
├── [ ] Ollama Local (1 day)
├── [ ] Antigravity (2 days)
├── [ ] OpenCode (1 day)
└── [ ] Others as needed

Note: Some executors may share common patterns. Group by:
- OpenAI-compatible → simple wrapper
- OAuth-based → token refresh pattern
- Custom format → individual implementation
```

#### 3.7 Format Translators (2-3 days)
```
Tasks:
├── [ ] Anthropic → OpenAI translator
├── [ ] Gemini → OpenAI translator
├── [ ] OpenAI → Anthropic translator (for /v1/messages)
├── [ ] Responses API → Chat completions translator
├── [ ] Kiro AWS EventStream translator
└── [ ] Error format normalizer
```

#### 3.8 Combo Strategies (2 days)
```
Tasks:
├── [ ] Define Strategy interface
├── [ ] Implement Fallback strategy (try until success)
├── [ ] Implement Round-robin strategy (distribute load)
├── [ ] Implement Sticky strategy (sticky per session)
├── [ ] Add sticky round-robin limit
├── [ ] Add combo-specific overrides

Strategy Configuration (stored via GORM in settings table):
{
  "comboStrategy": "fallback",
  "comboStickyRoundRobinLimit": 1,
  "comboStrategies": {
    "my-combo": { "fallbackStrategy": "round-robin" }
  }
}
```

#### 3.9 RTK (Response Transform Kit) (2 days)
```
Tasks:
├── [ ] Implement caveman filter (context compression)
├── [ ] Implement autodetect format
├── [ ] Add per-request RTK override
└── [ ] Document RTK behavior
```

---

### Phase 4: Streaming SSE (1-2 weeks)

**Goal:** Implement streaming responses with proper goroutine management via Fiber.

#### 4.1 SSE Base (3 days)
```
Tasks:
├── [ ] Create SSE response helper (Fiber c.Context streaming)
│   └── fiber.Ctx: c.Set("Content-Type", "text/event-stream")
│   └── c.Set("Cache-Control", "no-cache")
│   └── c.Set("Connection", "keep-alive")
│   └── c.Context().SetBodyStreamWriter(func(w *bufio.Writer) { ... })
├── [ ] Handle context cancellation (client disconnect → abort upstream)
├── [ ] Handle connection close gracefully
└── [ ] Handle CORS preflight

Fiber SSE Pattern:
├── app.Post("/v1/chat/completions", func(c *fiber.Ctx) error {
│   ├── c.Set("Content-Type", "text/event-stream")
│   ├── c.Set("Cache-Control", "no-cache")
│   │
│   ├── c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
│   │   ├── for chunk := range streamChan {
│   │   │   ├── fmt.Fprintf(w, "data: %s\n\n", chunk.JSON())
│   │   │   ├── w.Flush()
│   │   │   }
│   │   ├── fmt.Fprintf(w, "data: [DONE]\n\n")
│   │   ├── w.Flush()
│   │   }
│   └── })
│   └── return nil
│ })
```

#### 4.2 Streaming Chat (2-3 days)
```
Tasks:
├── [ ] Implement streaming in chat handler
├── [ ] Handle delta chunks (content, tool_calls)
├── [ ] Handle finish_reason
├── [ ] Handle [DONE] marker
├── [ ] Handle errors mid-stream (send error event, close)
└── [ ] Add usage tracking for streaming

Stream Format:
data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":"Hello"},"index":0}]}

data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":" world"},"index":0}]}

data: {"id":"chatcmpl-xxx","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_xxx","type":"function","function":{"name":"get_weather","arguments":"{\"city\""}}}]},"index":0}]}

data: {"id":"chatcmpl-xxx","choices":[{"finish_reason":"stop","index":0}]}

data: [DONE]
```

#### 4.3 Tool Calls Streaming (2 days)
```
Tasks:
├── [ ] Accumulate tool call parts (id, name, arguments)
├── [ ] Stream partial arguments
├── [ ] Send complete tool_call on finish_reason=tool_calls
├── [ ] Handle streaming tool_call chunks
└── [ ] Test with multi-turn conversations
```

---

### Phase 5: Migration Strategy (2 weeks)

**Goal:** Zero-downtime migration from Node.js to Go.

#### 5.1 Parallel Run (1 week)
```
Tasks:
├── [ ] Deploy Go server on different port (e.g., 8090)
├── [ ] Configure Nginx/Caddy to route:
│   - /v1/* → Go server (primary)
│   - /api/* → Go server (primary)
│   - /dashboard/* → Next.js (unchanged)
├── [ ] MITM → Go (change ROUTER_BASE in src/mitm/handlers/base.js)
├── [ ] CLI → Go (change base URL in cli/src/cli/api/client.js)
├── [ ] Monitor for errors
└── [ ] Compare responses byte-by-byte

Migration Config:
location /v1 {
    proxy_pass http://localhost:8090;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
}

location /api {
    proxy_pass http://localhost:8090;
}

location /dashboard {
    proxy_pass http://localhost:20128;
}
```

#### 5.2 Gradual Cutover (3-4 days)
```
Tasks:
├── [ ] Start with low-traffic endpoints (combos, keys)
├── [ ] Move /v1/chat/completions
├── [ ] Move /v1/embeddings
├── [ ] Move all /api/* endpoints
├── [ ] Remove Next.js API routes one by one
└── [ ] Final: remove Nginx proxy (Go serves directly on 20128)

Rollback Plan:
├── Keep Next.js API routes active during migration
├── Revert Nginx config to point to Next.js
└── MITM ROUTER_BASE back to localhost:20128
```

#### 5.3 Cleanup (2-3 days)
```
Tasks:
├── [ ] Remove Next.js API routes (src/app/api/*)
├── [ ] Update package.json (remove unneeded dependencies)
├── [ ] Update documentation
├── [ ] Update deployment scripts
└── [ ] Archive old code (move to /legacy)

Dependencies to remove from package.json:
├── express
├── pg
├── better-sqlite3
├── sql.js
├── node-forge
└── selfsigned
```

---

### Phase 6: Testing & Quality Assurance (2-3 weeks, ongoing)

**Goal:** Comprehensive test coverage with unit, integration, and E2E tests.

---

#### 6.0 Testing Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         TEST PYRAMID                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│                         ┌───────────────┐                               │
│                         │   E2E Tests  │  ← 10% (critical flows)       │
│                         │   (Playwright)│      ~50 test cases          │
│                         └───────┬───────┘                               │
│                                 │                                        │
│                    ┌────────────┴────────────┐                         │
│                    │   Integration Tests     │  ← 30%                  │
│                    │   (real DB + mocked APIs)│     ~150 test cases     │
│                    └────────────┬────────────┘                         │
│                                 │                                        │
│                    ┌────────────┴────────────┐                         │
│                    │     Unit Tests          │  ← 60%                  │
│                    │   (pure functions, GORM) │     ~300 test cases     │
│                    └─────────────────────────┘                         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

Coverage Target: 85% overall (80% minimum per package)

┌────────────────────────────────────────────────────────────────────────┐
│ TEST CATEGORIES:                                                        │
│                                                                        │
│ 1. Unit Tests — internal/repository, internal/model, internal/auth     │
│ 2. Integration Tests — handlers with real DB, mocked HTTP clients     │
│ 3. E2E Tests — full flow dari API call sampai response                │
│ 4. Contract Tests — verify Go API match Node.js spec                  │
│ 5. Performance Tests — benchmark critical paths                        │
│ 6. Chaos Tests — graceful shutdown, connection pool exhaustion         │
└────────────────────────────────────────────────────────────────────────┘
```

---

#### 6.1 Unit Tests (Week 1)

**Framework:** `testing/testing` + `stretchr/testify` + `gocheck`

**Strategy:**
- Pure functions tanpa I/O → langsung test
- GORM operations → use `gorm.Open(sqlite.Open(":memory:"))` untuk test DB
- Repository layer → interface untuk easy mocking

**Directory Structure:**
```
internal/
├── model/
│   └── model_test.go
├── repository/
│   ├── provider_test.go
│   ├── combo_test.go
│   ├── apikey_test.go
│   ├── settings_test.go
│   └── usage_test.go
├── auth/
│   ├── jwt_test.go
│   └── apikey_test.go
├── executor/
│   ├── executor_test.go
│   └── openai_test.go
├── translator/
│   ├── anthropic_test.go
│   └── gemini_test.go
├── combo/
│   ├── fallback_test.go
│   ├── roundrobin_test.go
│   └── sticky_test.go
├── rtk/
│   └── caveman_test.go
└── handler/
    └── handler_test.go (table-driven tests)
```

**Example: Repository Test with In-Memory SQLite**
```go
func TestProviderRepository_FindAll(t *testing.T) {
    db, _ := gorm.Open(sqlite.Open(":memory:"))
    db.AutoMigrate(&model.ProviderConnection{})

    repo := NewProviderRepository(db)

    // Seed data
    repo.Create(&model.ProviderConnection{ID: "1", Provider: "openai", Data: "{}"})

    providers, err := repo.FindAll()
    assert.NoError(t, err)
    assert.Len(t, providers, 1)
    assert.Equal(t, "openai", providers[0].Provider)
}

func TestProviderRepository_UpdatePriority(t *testing.T) {
    db, _ := gorm.Open(sqlite.Open(":memory:"))
    db.AutoMigrate(&model.ProviderConnection{})

    repo := NewProviderRepository(db)
    repo.Create(&model.ProviderConnection{ID: "1", Provider: "openai", Priority: intPtr(1)})

    err := repo.UpdatePriority("1", 2)
    assert.NoError(t, err)

    p, _ := repo.FindByID("1")
    assert.Equal(t, 2, *p.Priority)
}
```

**Coverage Targets per Package:**

| Package | Target | Critical Paths |
|---------|--------|----------------|
| `internal/model/` | 95% | JSON parse/set, validation |
| `internal/repository/` | 95% | All CRUD + complex queries |
| `internal/auth/` | 98% | JWT create/verify, API key validate |
| `internal/executor/` | 85% | Request building, error handling |
| `internal/translator/` | 90% | Format conversions, edge cases |
| `internal/combo/` | 95% | Strategy logic, failover |
| `internal/rtk/` | 85% | Filter application |
| `internal/handler/` | 80% | Request parsing, response building |

**Total Unit Tests Target: 300+ test cases**

---

#### 6.2 Integration Tests (Week 1-2)

**Framework:** `stretchr/testify` + `httptest`

**Strategy:**
- Start real Fiber app dengan test database
- Use `httptest.NewRequest` untuk call handlers
- Mock external API calls (OpenAI, Anthropic, etc) dengan `nhooyr.io/websocket`
- Test full request/response cycle

**Categories:**

```go
// Integration test categories:

// 1. Handler Integration Tests
func TestProvidersHandler_List(t *testing.T) {
    app := fiber.New()
    setupTestDB(app)

    req := httptest.NewRequest("GET", "/api/providers", nil)
    req.Header.Set("Authorization", "Bearer "+testJWT)

    resp, err := app.Test(req)
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
}

// 2. Auth Flow Integration Tests
func TestAuthFlow_PasswordLogin(t *testing.T) {
    app := fiber.New()
    setupTestDB(app)
    seedUser(app, "admin", hashPassword("test123"))

    req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"test123"}`))
    req.Header.Set("Content-Type", "application/json")

    resp, err := app.Test(req)
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)

    // Verify JWT cookie set
    cookies := resp.Cookies()
    assert.True(t, hasCookie(cookies, "auth_token"))
}

// 3. Streaming Integration Tests
func TestChatCompletions_Streaming(t *testing.T) {
    // Mock provider response
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(200)

        // Stream chunks
        for _, chunk := range []string{`{"choices":[{"delta":{"content":"Hello"}}]}`, `[DONE]`} {
            fmt.Fprintf(w, "data: %s\n\n", chunk)
            w.(http.Flusher).Flush()
        }
    }))
    defer mockServer.Close()

    // Test streaming handler
    app := fiber.New()
    setupStreamingHandler(app, mockServer.URL)

    req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}]}`))
    req.Header.Set("Content-Type", "application/json")

    resp, err := app.Test(req)
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
    assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
}

// 4. Combo Strategy Integration Tests
func TestCombo_FallbackStrategy(t *testing.T) {
    // Test with multiple providers, first fails, second succeeds
    app := fiber.New()
    setupComboHandler(app)

    req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"my-combo"}`))
    resp, _ := app.Test(req)

    // Verify fallback attempted and succeeded
    assert.Equal(t, 200, resp.StatusCode)
    assertStreamContains(resp, "Hello from fallback")
}
```

**Integration Test Coverage:**

| Category | Test Cases | Target |
|----------|------------|--------|
| CRUD Operations | 50 | All /api/* endpoints |
| Auth Flows | 30 | login, logout, JWT, OIDC |
| Streaming | 20 | chat completions, SSE |
| Combo Strategies | 30 | fallback, roundrobin, sticky |
| Error Handling | 20 | 400, 401, 403, 429, 500, 503 |

**Total Integration Tests Target: 150+ test cases**

---

#### 6.3 E2E Tests with Playwright (Week 2-3)

**Framework:** Playwright + Go test runner (`github.com/playwright-community/playwright-go`)

**Strategy:**
- Test complete user flows dari frontend ke backend
- Verify end-to-end behavior, not just unit logic
- Use real browser for UI testing

**Test Scenarios:**

```go
// e2e_test.go

func TestE2E_ProviderManagement(t *testing.T) {
    pw, _ := playwright.Run()
    defer pw.Close()

    browser, _ := pw.Chromium.Launch()
    page, _ := browser.NewPage()

    // 1. Login flow
    page.Goto("http://localhost:8090/login")
    page.Fill("#password", "123456")
    page.Click("button[type=submit]")

    // Wait for dashboard
    page.WaitForURL("**/dashboard")
    assert.True(t, page.Locator("text=9Router").IsVisible())

    // 2. Add provider
    page.Click("text=Providers")
    page.Click("text=Add Provider")
    page.Fill("input[name=name]", "Test OpenAI")
    page.Fill("input[name=apiKey]", "sk-test-123")
    page.Click("button:has-text('Save')")

    // Verify provider added
    page.WaitForSelector("text=Test OpenAI")
    assert.True(t, page.Locator("text=Test OpenAI").IsVisible())

    browser.Close()
}

func TestE2E_ChatCompletions(t *testing.T) {
    pw, _ := playwright.Run()
    browser, _ := pw.Chromium.Launch()
    page, _ := browser.NewPage()

    // Setup provider via API first
    setupTestProvider("openai", "sk-test-key")

    // Test chat API via curl equivalent
    resp, _ := http.Post("http://localhost:8090/v1/chat/completions", "application/json",
        strings.NewReader(`{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`))

    assert.Equal(t, 200, resp.StatusCode)

    // Verify streaming response
    body, _ := io.ReadAll(resp.Body)
    assert.Contains(t, string(body), "data:")
    assert.Contains(t, string(body), "Hello")

    browser.Close()
}

func TestE2E_ComboWithFallback(t *testing.T) {
    // Setup combo with two models, first will fail
    setupCombo("test-combo", []string{"failing-model", "working-model"})

    // Call API
    resp, _ := http.Post("http://localhost:8090/v1/chat/completions", "application/json",
        strings.NewReader(`{"model":"test-combo","messages":[{"role":"user","content":"Hi"}]}`))

    // Should succeed via fallback
    assert.Equal(t, 200, resp.StatusCode)
    body, _ := io.ReadAll(resp.Body)
    assert.NotContains(t, string(body), "error")
}

func TestE2E_MITMIntegration(t *testing.T) {
    // Start MITM server (Node.js) pointing to Go backend
    mitm := startMITMWithTarget("http://localhost:8090")

    // Configure system proxy
    setSystemProxy("localhost", mitm.Port)

    // Make request via proxy
    resp, _ := http.Get("https://api.openai.com/v1/chat/completions")

    // Verify request went through MITM to Go
    assert.Equal(t, 200, resp.StatusCode)
    verifyMITMLogContains("openai")
}

func TestE2E_SettingsUpdate(t *testing.T) {
    pw, _ := playwright.Run()
    browser, _ := pw.Chromium.Launch()
    page, _ := browser.NewPage()

    page.Goto("http://localhost:8090/dashboard")
    page.Login("admin", "123456")

    // Update settings
    page.Click("text=Settings")
    page.Click("text=RTK")
    page.Click("button:has-text('Save')")

    // Verify via API
    resp, _ := http.Get("http://localhost:8090/api/settings")
    body, _ := io.ReadAll(resp.Body)
    assert.Contains(t, string(body), `"rtkEnabled":true`)

    browser.Close()
}
```

**E2E Test Coverage:**

| Flow | Test Cases | Priority |
|------|------------|----------|
| Auth (login/logout/JWT) | 5 | P0 |
| Provider CRUD | 10 | P0 |
| Chat Completions (streaming) | 10 | P0 |
| Combo Strategies | 8 | P0 |
| Usage Dashboard | 5 | P1 |
| Settings | 5 | P1 |
| MITM Integration | 5 | P1 |
| Error Scenarios | 10 | P2 |

**Total E2E Tests Target: 50+ test cases**

---

#### 6.4 Contract Tests (Week 2-3)

**Goal:** Verify Go implementation matches Node.js behavior byte-by-byte.

**Strategy:**
- Record Node.js responses for known inputs
- Compare Go responses against recorded responses
- Focus on streaming format, error format, header order

```go
// contract_test.go

func TestContract_ChatCompletions(t *testing.T) {
    testCases := []struct {
        name     string
        request  ChatRequest
        expected string // path to expected response JSON
    }{
        {"simple message", simpleRequest, "testdata/chat/simple.json"},
        {"with system prompt", systemRequest, "testdata/chat/system.json"},
        {"with tools", toolRequest, "testdata/chat/tools.json"},
        {"streaming", streamRequest, "testdata/chat/stream.json"},
        {"error rate limit", rateLimitRequest, "testdata/chat/rate-limit.json"},
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Call Node.js (baseline)
            nodeResp := callNodeJS(tc.request)

            // Call Go (implementation)
            goResp := callGoServer(tc.request)

            // Compare
            assertStreamingEqual(t, nodeResp, goResp)
            assertHeadersEqual(t, nodeResp, goResp)
        })
    }
}

func TestContract_ErrorFormat(t *testing.T) {
    testCases := []struct {
        statusCode int
        body       string
    }{
        {400, `{"model":"invalid"}`},
        {401, `{"model":"gpt-4"}`},
        {429, `{"model":"gpt-4"}`},
        {500, `{"model":"gpt-4"}`},
    }

    for _, tc := range testCases {
        resp := callGoServer(ChatRequest{Body: tc.body})
        assert.Equal(t, tc.statusCode, resp.StatusCode)
        assertValidErrorJSON(t, resp.Body)
    }
}
```

---

#### 6.5 Performance Tests (Week 2-3)

**Framework:** `go wrk` + custom benchmark

```go
// benchmark_test.go

func BenchmarkChatCompletions_Streaming(b *testing.B) {
    app := fiber.New()
    setupBenchmarkHandler(app, mockProvider)

    // Warm up
    req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(testRequest))
    app.Test(req)

    // Benchmark
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(testRequest))
        app.Test(req)
    }
}

func BenchmarkProvidersList(b *testing.B) {
    app := fiber.New()
    setupProviderBenchmark(app)

    for i := 0; i < b.N; i++ {
        req := httptest.NewRequest("GET", "/api/providers", nil)
        app.Test(req)
    }
}

// wrk script for real-world benchmarking
// wrk -t12 -c400 -d30s -s benchmark.lua http://localhost:8090/v1/chat/completions
```

**Performance Targets:**

| Metric | Target | Measurement |
|--------|--------|-------------|
| Requests/sec (chat) | 1000+ | wrk benchmark |
| Latency p50 | <50ms | wrk latency |
| Latency p99 | <200ms | wrk latency |
| Memory usage | <100MB | runtime.ReadMemStats |
| Startup time | <2s | time.Since(start) |
| DB query time | <10ms | p99 GORM queries |

---

#### 6.6 Test Infrastructure

**CI/CD Setup:**

```yaml
# .github/workflows/test.yml

name: Tests

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run unit tests with coverage
        run: |
          go install github.com/pressly/goose/v3/cmd/goose@latest
          go test -v -race -coverprofile=coverage.out ./...
          go tool cover -html=coverage.out -o coverage.html
      - name: Upload coverage
        uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.html

  integration-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_PASSWORD: test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Run integration tests
        run: |
          DATABASE_URL=postgres://postgres:test@localhost:5432/test go test -v ./internal/...
      - name: Run E2E tests
        run: |
          playwright install chromium
          go test -v ./e2e/...

  e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Start server
        run: go run cmd/server/main.go &
      - name: Run Playwright E2E
        run: |
          npx playwright install
          npx playwright test
```

**Test Data Management:**
```
testdata/
├── unit/           # Fixtures for unit tests
├── integration/    # Fixtures for integration tests
├── contract/        # Recorded Node.js responses
├── providers/      # Test API keys (mock)
└── fixtures/       # JSON fixtures for requests
```

---

#### 6.7 Coverage Enforcement

** gates:**
```bash
# Makefile
lint: golangci-lint run
test: go test -v -cover ./...
coverage: go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# PR cannot merge if coverage < 85%
check-coverage:
    @COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | tr -d '%')
    @if [ $$(echo "$COVERAGE < 85" | bc -l) -eq 1 ]; then \
        echo "Coverage $$COVERAGE% is below 85% threshold"; \
        exit 1; \
    fi
    @echo "Coverage: $$COVERAGE%"
```

**Coverage Dashboard:**
- Upload to Codecov or Coveralls
- Set threshold: 85% minimum
- Break down by package

---

## 5. Deliverables Checklist

### Documentation
- [ ] OpenAPI spec (openapi.yaml)
- [ ] Database schema (GORM model docs)
- [ ] Routing documentation (routing.md)
- [ ] Translator matrix (translator.md)
- [ ] Credentials flow (credentials.md)
- [ ] Architecture diagram (architecture.md)
- [ ] Deployment guide (deploy.md)
- [ ] API changelog (changelog.md)

### Code
- [ ] Go server (cmd/server/main.go)
- [ ] All handlers (internal/handler/)
- [ ] All executors (internal/executor/)
- [ ] All translators (internal/translator/)
- [ ] GORM models (internal/model/)
- [ ] Repository layer (internal/repository/)
- [ ] Auth middleware (internal/middleware/)
- [ ] Auto-migration (repository/db.go)
- [ ] Tests (internal/*/*_test.go)
- [ ] Makefile
- [ ] Dockerfile

### Configuration
- [ ] .env.example
- [ ] golangci.yml
- [ ] Makefile targets
- [ ] Nginx config (if used)
- [ ] Systemd service file

---

## 7. Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| SSE streaming edge cases | High | Medium | Extensive testing with real clients |
| Format translator bugs | High | Medium | Compare output byte-by-byte with Node.js |
| GORM AutoMigrate tidak cocok dengan schema existing | Tinggi | Rendah | Test migration dengan copy database |
| OAuth flows complex | Medium | High | Document all OAuth implementations before coding |
| Performance regression | Medium | Medium | Benchmark early, tune continuously |
| Missing endpoints | High | Low | Check against frontend usage |
| Auth edge cases | High | Medium | Test with real browser sessions |
| Fiber fasthttp incompatibility | Medium | Low | Jika ada, fallback ke standar net/http wrapper |

---

## 8. Timeline

```
Week  1-2:  Phase 0 (Reverse Engineering & Spec)
Week  3-5:  Phase 1 (Core Infrastructure — Fiber + GORM setup)
Week  6-8:  Phase 2 (REST API Implementation)
Week  9-12: Phase 3 (AI Proxy Engine)
Week 13-14: Phase 4 (Streaming SSE)
Week 15-16: Phase 5 (Migration Strategy)
Week 17+:  Phase 6 (Testing & Polish)

Total Estimated: 17-20 weeks (4-5 months)
```

---

## 9. Success Criteria

### Functional
- [ ] All 126 API endpoints implemented
- [ ] All 19 provider executors working
- [ ] Streaming chat completions working
- [ ] Combo strategies working correctly
- [ ] Auth flows (password, OIDC, API key) working
- [ ] MITM integration working
- [ ] CLI integration working
- [ ] Frontend works without modification

### Performance
- [ ] No regression in requests/sec vs Node.js
- [ ] Memory usage < Node.js (Go advantage)
- [ ] Startup time < Node.js (Go advantage)

### Quality
- [ ] 85%+ test coverage (minimum 80% per package)
- [ ] 300+ unit tests passing
- [ ] 150+ integration tests passing
- [ ] 50+ E2E tests passing
- [ ] Contract tests verify Go matches Node.js
- [ ] All lint checks passing (golangci-lint)
- [ ] No critical security vulnerabilities
- [ ] Documentation complete
- [ ] Coverage gate enforcement in CI/CD (85% minimum)

---

## 10. Post-Migration

### Next Steps (Out of Scope, but Documented)
1. Consider React frontend rewrite (optional)
2. Add GraphQL API for flexibility
3. Implement rate limiting per API key
4. Add distributed tracing (OpenTelemetry)
5. Implement caching layer (Redis) for hot paths
6. Add real-time dashboard updates (WebSocket)

### Monitoring
- Set up metrics endpoint (/metrics for Prometheus)
- Log aggregation (structured JSON logs via zerolog)
- Alerting on error rate
- Usage dashboards (Grafana)

---

## Appendix A: Provider Matrix

| Provider | Auth Type | Executor | Special Handling |
|----------|-----------|----------|------------------|
| openai | API Key | openai | Standard |
| anthropic | API Key | anthropic | Translators, thinking |
| gemini | API Key | gemini | Translators |
| azure | API Key | azure | Deployment mapping |
| vertex | OAuth | vertex | Project ID |
| github | PAT | github | No refresh |
| kiro | OAuth | kiro | EventStream |
| grok | API Key | grok | Standard |
| qwen | API Key | qwen | Standard |
| perplexity | API Key | perplexity | Web search |
| ollama | None | ollama | Local |
| antigravity | OAuth | antigravity | Gemini format |

---

## Appendix B: API Endpoint Summary

### Public Endpoints (No Auth Required)
```
POST /v1/chat/completions
POST /v1/messages
POST /v1/embeddings
POST /v1/responses
POST /v1/search
POST /v1/web/fetch
POST /v1/audio/speech
POST /v1/audio/transcriptions
GET  /v1/models
GET  /health
GET  /api/providers (if requireApiKey=false)
```

### Protected Endpoints (JWT Required)
```
All /api/* except public allowlist
All /dashboard/* routes
```

### CLI-Only Endpoints (x-9r-cli-token)
```
/api/cli-tools/*
/api/shutdown
/api/version/shutdown
/api/version/update
```

---

## Appendix C: Go Dependencies Summary

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/gofiber/fiber/v2` | latest | Web framework & router |
| `gorm.io/gorm` | latest | ORM & query builder |
| `gorm.io/driver/sqlite` | latest | SQLite driver (via CGO or pure-go) |
| `gorm.io/driver/postgres` | latest | PostgreSQL driver |
| `github.com/golang-jwt/jwt/v5` | latest | JWT sign & verify |
| `golang.org/x/crypto` | latest | bcrypt password hashing |
| `github.com/joho/godotenv` | latest | .env file loader |
| `github.com/go-playground/validator/v10` | latest | Struct validation |
| `github.com/rs/zerolog` | latest | Structured logging |
| `github.com/google/uuid` | latest | UUID generation |
| `github.com/stretchr/testify` | latest | Testing assertions |
| `github.com/playwright-community/playwright-go` | latest | E2E browser testing |
| `github.com/nhooyr/websocket` | latest | WebSocket for streaming tests |
| `github.com/pressly/goose/v3` | latest | Database migrations |
| `github.com/valyala/fasthttp` | indirect | Fiber HTTP engine |

---

## Appendix D: Fiber vs Gin Comparison

| Aspect | Fiber | Gin |
|--------|-------|-----|
| **Engine** | fasthttp | net/http |
| **Performance** | ~2x faster | Fast |
| **Express-like API** | Yes (intentional) | No |
| **Middleware ecosystem** | Good | Excellent |
| **Community size** | Large | Largest |
| **Learning curve** | Low (Express devs) | Medium |
| **SSE streaming** | `c.Context().SetBodyStreamWriter()` | `c.Stream()` |
| **Route grouping** | `app.Group("/api")` | `router.Group("/api")` |
| **Error handling** | `return c.Status(500).JSON(...)` | `c.JSON(500, ...)` |

*Alasan pilih Fiber: Express-like API lebih natural bagi codebase yang saat ini pakai Express, performa lebih tinggi, dan middleware pattern yang sederhana.*

---

*Document Version: 1.2*
*Last Updated: 2026-06-04*
*Author: Claude Code*