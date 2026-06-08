# Database Domain — Atomic Task Breakdown

## Scope Coverage

This domain covers the full GORM-based database layer for the Go backend rewrite: model definitions, connection management with dual-driver support (SQLite + PostgreSQL), auto-migration, connection pooling, JSON data column helpers, the repository pattern, and the migration system.

**Source-of-truth files analyzed:**
- `/Users/alkaaf/project/9router/src/lib/db/schema.js` — SQLite schema (columns, indexes, types)
- `/Users/alkaaf/project/9router/src/lib/db/schema.postgres.js` — PostgreSQL schema (JSONB, TIMESTAMPTZ, BOOLEAN, BIGSERIAL, NUMERIC)
- `/Users/alkaaf/project/9router/src/lib/db/adapters/postgresAdapter.js` — PG connection pool, placeholder translation, row key camelCase transform, savepoint-based nested transactions
- `/Users/alkaaf/project/9router/scripts/migrate-sqlite-to-postgres.js` — Migration logic, column type transformers, batch insert, idempotency
- `/Users/alkaaf/project/9router/src/lib/db/driver.js` — Driver selection priority (PostgreSQL first, then SQLite fallback)
- `/Users/alkaaf/project/9router/src/lib/db/migrate.js` — Versioned migration runner, schema sync, legacy JSON import
- `/Users/alkaaf/project/9router/src/lib/db/helpers/jsonCol.js` — `parseJson` / `stringifyJson` utilities
- `/Users/alkaaf/project/9router/src/lib/db/repos/connectionsRepo.js` — representative existing repository pattern

**Domain context from manifesto:** Sections 4.1 (GORM model definitions), 4.2 (repository layer), 4.3 (JSON data columns), 1.3 (database layer task).

---

## Tasks

### Group A: Model Definitions (internal/model/)

---

#### DB-001: Define `_meta`, `settings`, and `kv` GORM models

**Description:** Create Go structs for the three simplest tables — key-value metadata, single-row settings, and composite-key key-value store. These have no JSON data columns (all TEXT) and no boolean fields.

**Input contract:**
- `_meta`: PK `key TEXT`, col `value TEXT NOT NULL`
- `settings`: PK `id INTEGER CHECK(id=1)`, col `data TEXT NOT NULL` (JSON string)
- `kv`: composite PK `(scope TEXT, key TEXT)`, col `value TEXT NOT NULL`

**Output contract:**
```go
// File: internal/model/meta.go
type Meta struct {
    Key   string `gorm:"primaryKey;type:text"`
    Value string `gorm:"not null;type:text"`
}

// File: internal/model/setting.go
type Setting struct {
    ID   uint   `gorm:"primaryKey;check:id = 1"`
    Data string `gorm:"not null;type:text"`
}

// File: internal/model/kv.go
type KV struct {
    Scope string `gorm:"primaryKey;type:text"`
    Key   string `gorm:"primaryKey;type:text"`
    Value string `gorm:"not null;type:text"`
}
```

**Test strategy:**
- Struct compiles and can be instantiated
- GORM can migrate to in-memory SQLite
- Tags produce correct DDL: check `check:id = 1` for settings, composite PK for kv
- Verify `TableName()` returns correct table names (GORM defaults to snake_case plural; verify matches `_meta`, `settings`, `kv`)

**Dependencies:** None (foundational models)

---

#### DB-002: Define `providerConnections` model

**Description:** Create the most complex model — provider credentials with boolean fields, text PK, multiple indexes, and a JSON `data` column storing credential-specific fields (accessToken, refreshToken, expiresAt, etc.).

**Input contract:**
```go
type ProviderConnection struct {
    ID         string    `gorm:"primaryKey;type:text"`
    Provider   string    `gorm:"not null;index:idx_pc_provider;index:idx_pc_provider_active,priority:1;index:idx_pc_priority,priority:1"`
    AuthType   string    `gorm:"not null"`
    Name       *string
    Email      *string
    Priority   *int
    IsActive   *bool     `gorm:"default:true"`
    Data       string    `gorm:"not null;type:text"`    // JSON
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

**Important duality consideration:** The Go model uses `string Data` (text/JSONB) not a typed struct, because the JSON payload varies per provider. The `*bool` for `IsActive` must map correctly in both SQLite (INTEGER 0/1) and PostgreSQL (BOOLEAN true/false) — GORM handles this automatically.

**Indexes:** 3 composite indexes matching `schema.js` / `schema.postgres.js`:
- `idx_pc_provider(provider)`
- `idx_pc_provider_active(provider, isActive)`
- `idx_pc_priority(provider, priority)`

**Test strategy:**
- Auto-migrate to in-memory SQLite and verify table structure
- Create record and verify all fields persist
- Verify `*bool` zero-value behavior (nil vs false vs true)
- Verify composite indexes exist via `db.Migrator().HasIndex()`

**Dependencies:** DB-001 (pattern established)

---

#### DB-003: Define `providerNodes` and `proxyPools` models

**Description:** Create models for provider node configurations and proxy pool definitions — both feature JSON data columns and boolean fields.

**Input contract:**
```go
type ProviderNode struct {
    ID        string    `gorm:"primaryKey;type:text"`
    Type      *string   `gorm:"index"`
    Name      *string
    Data      string    `gorm:"not null;type:text"`     // JSON
    CreatedAt time.Time
    UpdatedAt time.Time
}

type ProxyPool struct {
    ID         string    `gorm:"primaryKey;type:text"`
    IsActive   *bool     `gorm:"default:true;index"`
    TestStatus *string   `gorm:"index"`
    Data       string    `gorm:"not null;type:text"`     // JSON
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

**Test strategy:**
- Auto-migrate and verify column types
- CRUD operations with JSON data
- Index creation verification

**Dependencies:** DB-002 (pattern established)

---

#### DB-004: Define `apiKeys` and `combos` models

**Description:** Create models for API key management and model combo definitions — both feature unique constraints and text primary keys.

**Input contract:**
```go
type ApiKey struct {
    ID        string    `gorm:"primaryKey;type:text"`
    Key       string    `gorm:"uniqueIndex;not null;type:text"`
    Name      *string
    MachineID *string
    IsActive  *bool     `gorm:"default:true"`
    CreatedAt time.Time
}

type Combo struct {
    ID        string    `gorm:"primaryKey;type:text"`
    Name      string    `gorm:"uniqueIndex;not null;type:text"`
    Kind      *string
    Models    string    `gorm:"not null;type:text"`   // JSON array
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Test strategy:**
- Verify `uniqueIndex` creates correct composite index entries
- Verify duplicate key/combo name is rejected
- JSON array in `Models` column

**Dependencies:** DB-002 (pattern established)

---

#### DB-005: Define `usageHistory` model

**Description:** Create the highest-write-rate model — append-only usage log with auto-incrementing BIGSERIAL PK, NUMERIC cost, JSONB tokens/meta, and 10 indexes for time-series queries.

**Input contract:**
```go
type UsageHistory struct {
    ID               uint      `gorm:"primaryKey;autoIncrement"`  // BIGSERIAL in PG
    Timestamp        time.Time `gorm:"not null;index:idx_uh_ts"`  // +9 more indexes
    Provider         *string   `gorm:"index"`
    Model            *string   `gorm:"index"`
    ConnectionID     *string   `gorm:"index"`
    ApiKey           *string   `gorm:"index"`
    Endpoint         *string
    PromptTokens     int       `gorm:"default:0"`
    CompletionTokens int       `gorm:"default:0"`
    Cost             float64   `gorm:"default:0;type:numeric(12,6)"`  // precision for PG
    Status           *string
    Tokens           *string   `gorm:"type:text"`  // JSON
    Meta             *string   `gorm:"type:text"`  // JSON
}
```

**Important duality consideration:** `autoIncrement` on SQLite uses INTEGER, on PG uses BIGSERIAL. The `Cost` field with `numeric(12,6)` is PG-specific; SQLite ignores type modifiers. All 10 indexes must match `schema.postgres.js`.

**Name mapping note:** The Node codebase uses `connectionId` and `apiKey` (camelCase after pg lowercase transform). Go GORM column naming via struct tags must use `connection_id`, `api_key` snake_case, or match via `column:connectionId` tag override.

**Test strategy:**
- Auto-migrate and verify auto-increment works
- Write 1000 rows and verify DESC index scan performance
- Verify numeric precision is maintained (cost with 6 decimal places)
- Verify JSON columns store valid JSON and can be retrieved

**Dependencies:** DB-002 (pattern established)

---

#### DB-006: Define `usageDaily` and normalized rollup models

**Description:** Create models for daily aggregated usage data — the core table uses a DATE PK with JSONB blob; the 5 normalized rollups use composite PKs with typed columns (BIGINT counts, NUMERIC cost).

**Input contract:**
```go
type UsageDaily struct {
    DateKey string `gorm:"primaryKey;type:text"`  // DATE type in PG, TEXT in SQLite
    Data    string `gorm:"not null;type:text"`     // JSON
}

// Normalized rollups (5 tables):
type UsageDailyByProvider struct {
    Date         time.Time `gorm:"primaryKey;type:date"`
    Provider     string    `gorm:"primaryKey;type:text"`
    RequestCount int64     `gorm:"not null;default:0"`
    InputTokens  int64     `gorm:"not null;default:0"`
    OutputTokens int64     `gorm:"not null;default:0"`
    TotalTokens  int64     `gorm:"not null;default:0"`
    Cost         float64   `gorm:"not null;default:0;type:numeric(12,6)"`
    UpdatedAt    time.Time `gorm:"not null"`
}
// Similarly: UsageDailyByModel, UsageDailyByApiKey, UsageDailyByAccount, UsageDailyByEndpoint
```

**Important:** The Node codebase does NOT have these 5 normalized rollup tables in SQLite — they only exist in PostgreSQL (`schema.postgres.js`). For the Go rewrite, we must decide: create them in both drivers, or keep them PG-only. **Recommendation:** Create in both for parity; normalizing in SQLite is harmless and future-proof. Use `AutoMigrate` which is additive.

**Test strategy:**
- Auto-migrate all 6 usage daily tables
- Verify composite PKs work (date + dimension)
- Verify upsert behavior (INSERT ON CONFLICT for daily rollups)
- Write and read back DATE values

**Dependencies:** DB-005 (usage pattern)

---

#### DB-007: Define `requestDetails` model

**Description:** Create model for per-request detail logs with JSON data payload and 4 indexes.

**Input contract:**
```go
type RequestDetail struct {
    ID           string    `gorm:"primaryKey;type:text"`
    Timestamp    time.Time `gorm:"not null;index:idx_rd_ts"`
    Provider     *string   `gorm:"index:idx_rd_provider"`
    Model        *string   `gorm:"index:idx_rd_model"`
    ConnectionID *string   `gorm:"index:idx_rd_conn"`
    Status       *string
    Data         string    `gorm:"not null;type:text"`  // JSON
}
```

**Test strategy:**
- Auto-migrate and verify indexes
- CRUD with text PK
- JSON data roundtrip

**Dependencies:** DB-002 (pattern established)

---

### Group B: Connection & Configuration

---

#### DB-008: GORM connection manager with dual-driver support

**Description:** Create the `repository/db.go` entry point with `NewGormDB()` that selects SQLite or PostgreSQL driver based on configuration, configures logging, sets up connection pooling, runs auto-migration, and returns the `*gorm.DB` handle.

**Input contract:**
- Config struct with `DBDriver` ("sqlite" | "postgres"), `DBPath`, `DatabaseURL`, `LogLevel`, `MaxIdleConns`, `MaxOpenConns`
- Environment variable fallback: `DATABASE_URL` for PG, `DB_PATH` for SQLite

**Output contract:**
```go
func NewGormDB(cfg *config.Config) (*gorm.DB, error) {
    switch cfg.DBDriver {
    case "sqlite":
        return gorm.Open(sqlite.Open(cfg.DBPath), &gorm.Config{
            Logger: logger.Default.LogMode(logger.Warn),
            SkipDefaultTransaction: true,       // perf: avoid wrapping single-op in txn
            PrepareStmt: false,                  // not needed for SQLite
        })
    case "postgres":
        return gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
            Logger: logger.Default.LogMode(logger.Warn),
            SkipDefaultTransaction: true,
            PrepareStmt: true,                   // beneficial for PG
        })
    }
}

func ConfigurePool(db *gorm.DB, cfg *config.Config) {
    sqlDB, _ := db.DB()
    sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)    // default: 10
    sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)    // default: 25 (SQLite), 100 (PG)
    sqlDB.SetConnMaxLifetime(cfg.DBMaxLifetime)   // default: 5min
    sqlDB.SetConnMaxIdleTime(cfg.DBMaxIdleTime)   // default: 1min
}
```

**Key concerns:**
- `SkipDefaultTransaction: true` — critical for write-heavy tables like `usageHistory`
- `PrepareStmt: true` for PG (reduces query planning overhead), but NOT for SQLite (complicates with concurrent writers)
- SQLite WAL mode must be set after connection: `db.Exec("PRAGMA journal_mode=WAL")`
- PG statement timeout via `SET statement_timeout = '30s'`
- Logger level: `Warn` in production, `Info` in development
- Error handling: close sqlDB on init failure

**Test strategy:**
- Initialize with SQLite `:memory:` and verify connection works
- Initialize with SQLite file path and verify WAL pragma
- Initialize with mock PG and verify connection string parsing
- Verify pool configuration values are applied (assert on `sqlDB.Stats()`)
- Verify `SkipDefaultTransaction` is set correctly

**Dependencies:** DB-001 through DB-007 (models must exist for auto-migration)

---

#### DB-009: Auto-migration setup

**Description:** Implement the `AutoMigrateAll()` call that registers all models and applies them, plus a verification step. Handle the duality where `usageDailyBy*` tables may not exist in all environments.

**Output contract:**
```go
func AutoMigrateAll(db *gorm.DB) error {
    models := []interface{}{
        &model.Meta{},
        &model.Setting{},
        &model.ProviderConnection{},
        &model.ProviderNode{},
        &model.ProxyPool{},
        &model.ApiKey{},
        &model.Combo{},
        &model.KV{},
        &model.UsageHistory{},
        &model.UsageDaily{},
        &model.UsageDailyByProvider{},
        &model.UsageDailyByModel{},
        &model.UsageDailyByApiKey{},
        &model.UsageDailyByAccount{},
        &model.UsageDailyByEndpoint{},
        &model.RequestDetail{},
    }
    return db.AutoMigrate(models...)
}

func VerifyMigration(db *gorm.DB) error {
    migrator := db.Migrator()
    for _, m := range models {
        if !migrator.HasTable(m) {
            return fmt.Errorf("table %s not created by auto-migrate", ...)
        }
    }
    return nil
}
```

**Test strategy:**
- Run `AutoMigrateAll` on in-memory SQLite
- Verify all tables exist via `db.Migrator().HasTable()`
- Verify indexes exist via `db.Migrator().HasIndex()`
- Run a second time (idempotent — no error)

**Dependencies:** DB-008 (connection must work), DB-001 through DB-007 (all models)

---

#### DB-010: JSON data column helpers

**Description:** Create `pkg/jsonutil` (or `internal/repository/json.go`) with generic JSON marshal/unmarshal helpers for the `data` columns stored as TEXT/JSONB. Each model with a JSON column gets typed data structs for parse/set methods.

**Output contract:**
```go
// Generic helpers
func ParseJSON[T any](s string) (*T, error)
func MustParseJSON[T any](s string) *T         // panics on error (for init-time)
func ToJSON(v any) (string, error)
func MustToJSON(v any) string

// Typed data structs per model
type ProviderConnectionData struct {
    AccessToken          string         `json:"accessToken"`
    RefreshToken         string         `json:"refreshToken"`
    ExpiresAt            int64          `json:"expiresAt"`
    ProjectID            string         `json:"projectId"`
    ProviderSpecificData map[string]any `json:"providerSpecificData"`
    // ... all other optional fields from connectionsRepo.js OPTIONAL_FIELDS
}

type ProviderNodeData struct { /* varies by type */ }
type ProxyPoolData struct { /* proxy config */ }
type RequestDetailData struct { /* request metadata */ }
type ComboModelsData []string

// Methods on model structs
func (m *ProviderConnection) GetData() (*ProviderConnectionData, error)
func (m *ProviderConnection) SetData(d *ProviderConnectionData) error
// Same pattern for ProviderNode, ProxyPool, RequestDetail
```

**Test strategy:**
- Marshal and unmarshal known JSON structures
- Edge cases: empty JSON, null values, deeply nested objects, special characters
- Verify `ProviderConnectionData` fields match the Node codebase's `OPTIONAL_FIELDS` list
- Verify `ComboModelsData` marshals as a JSON string array `["gpt-4", "claude-3"]`
- Benchmarks for hot-path serialization

**Dependencies:** None (standalone utility package)

---

#### DB-011: UUID generation hook (BeforeCreate)

**Description:** Implement a GORM hook that auto-generates UUIDv4 for string primary key fields that are empty at creation time. Models with text PKs (`ProviderConnection`, `ProviderNode`, `ProxyPool`, `ApiKey`, `Combo`, `RequestDetail`, `Meta`) need this.

**Output contract:**
```go
// BeforeCreate hook function
func GenerateUUID(model interface{}) error {
    // Reflection-based: if the model has a string ID field tagged with
    // gorm:"primaryKey" and the value is empty, set to uuid.New().String()
}

// Per-model hooks (alternative to reflection):
func (m *ProviderConnection) BeforeCreate(tx *gorm.DB) error {
    if m.ID == "" { m.ID = uuid.New().String() }
    return nil
}

// Timestamp management
func (m *ProviderConnection) BeforeCreate(tx *gorm.DB) error {
    now := time.Now()
    if m.CreatedAt.IsZero() { m.CreatedAt = now }
    if m.UpdatedAt.IsZero() { m.UpdatedAt = now }
    return nil
}

func (m *ProviderConnection) BeforeUpdate(tx *gorm.DB) error {
    m.UpdatedAt = time.Now()
    return nil
}
```

**Decision point:** Reflection-based generic hook vs per-model hooks. Manifesto shows per-model. **Recommendation:** Per-model hooks are more explicit and debuggable; use a shared helper function `SetCreateTimestamps(m, now)` to avoid repetition.

**Test strategy:**
- Create model without ID → verify UUID is generated
- Create model with pre-set ID → verify UUID is NOT overwritten
- Verify timestamps are set correctly on create and update
- Verify UUID format (8-4-4-4-12 hex pattern)

**Dependencies:** DB-001 through DB-007 (models to attach hooks to)

---

#### DB-012: GORM query logger and performance instrumentation

**Description:** Configure GORM's logger with slow-query threshold, query-level logging, and optional Prometheus metrics hooks. Add `db.InstanceSet()` for context propagation.

**Output contract:**
```go
func NewLogger(cfg *config.Config) logger.Interface {
    return logger.New(
        log.New(os.Stdout, "\r\n", log.LstdFlags),
        logger.Config{
            SlowThreshold:             200 * time.Millisecond,  // log slow queries
            LogLevel:                  mapLogLevel(cfg.LogLevel), // Silent | Error | Warn | Info
            IgnoreRecordNotFoundError: true,                     // reduce noise
            ParameterizedQueries:      cfg.Environment != "dev", // bind params in prod
            Colorful:                  cfg.Environment == "dev",
        },
    )
}

// Callback: log query duration
func RegisterQueryCallbacks(db *gorm.DB) {
    db.Callback().Query().After("gorm:query").Register("instrument:query", func(db *gorm.DB) {
        if db.DryRun { return }
        duration := time.Since(db.Statement.ReflectTime)
        if duration > 200*time.Millisecond {  // ctx.Value("request_id")
            log.Warn().Dur("duration", duration).Msg("slow query")
        }
    })
}
```

**Test strategy:**
- Verify logger level mapping for Silent/Error/Warn/Info
- Verify slow query threshold fires callback
- Verify `IgnoreRecordNotFoundError` suppresses gorm.ErrRecordNotFound logs

**Dependencies:** DB-008 (connection manager)

---

### Group C: Repository Layer (internal/repository/)

---

#### DB-013: Base repository pattern (repository/base.go)

**Description:** Define the base repository interface and common helper functions that all specific repositories embed. Establish the singleton `*gorm.DB` accessor pattern.

**Output contract:**
```go
// Base repository struct — embedded in all repos
type BaseRepository struct {
    db *gorm.DB
}

func NewBaseRepository(db *gorm.DB) *BaseRepository {
    return &BaseRepository{db: db}
}

// Common helpers
func (r *BaseRepository) DB() *gorm.DB { return r.db }

// Paged query helper
type Pagination struct {
    Page    int   `json:"page"`
    PerPage int   `json:"perPage"`
    Total   int64 `json:"total"`
}

// Generic JSON column helpers bound to the repository
func (r *BaseRepository) parseJSON(s string, target interface{}) error {
    return json.Unmarshal([]byte(s), target)
}

// Now() always returns UTC
func Now() time.Time { return time.Now().UTC() }
```

**Key decisions:**
- Repository pattern: struct embedding (not interface-based) for simplicity. Interface only if multiple implementations needed (e.g., caching layer).
- Session handling: `db.Session(&gorm.Session{DryRun: true})` for query debugging.
- Context propagation: all repo methods should accept `context.Context` and use `db.WithContext(ctx)`.

**Test strategy:**
- Verify `BaseRepository` embeds correctly in specific repos
- Verify `Pagination` struct serializes to JSON correctly
- Verify `Now()` returns UTC

**Dependencies:** DB-008 (must have working GORM connection)

---

#### DB-014: Provider connection repository (repository/provider.go)

**Description:** Full CRUD + complex queries for the `providerConnections` table — the most feature-rich repository mirroring `connectionsRepo.js`.

**Methods:**
```go
func (r *ProviderRepository) FindAll(ctx context.Context, filter ...ProviderFilter) ([]model.ProviderConnection, error)
func (r *ProviderRepository) FindByID(ctx context.Context, id string) (*model.ProviderConnection, error)
func (r *ProviderRepository) FindByProvider(ctx context.Context, provider string, onlyActive bool) ([]model.ProviderConnection, error)
func (r *ProviderRepository) Create(ctx context.Context, conn *model.ProviderConnection) error
func (r *ProviderRepository) Update(ctx context.Context, conn *model.ProviderConnection) error
func (r *ProviderRepository) Delete(ctx context.Context, id string) error
func (r *ProviderRepository) DeleteByProvider(ctx context.Context, provider string) (int64, error)
func (r *ProviderRepository) Reorder(ctx context.Context, provider string) error
func (r *ProviderRepository) Cleanup(ctx context.Context) (int, error)  // remove stale fields
```

**Key business logic from Node codebase (`connectionsRepo.js`):**
- `FindAll`: returns sorted by priority ASC
- `FindByProvider`: filters by provider name, optionally by isActive
- `Reorder`: inside transaction, re-prioritizes all connections for a provider
- `Create`: inside transaction, dedup by authType+email or name, auto-assign priority, merge existing
- `Update`: inside transaction, merge fields onto existing row, reorder if priority changed
- `Delete`: inside transaction, delete row then reorder remaining
- `Cleanup`: inside transaction, purge null fields from `data` column

**Test strategy:**
- Full CRUD with in-memory SQLite
- Integration: create connection → set priority → reorder → verify sort order
- Integration: duplicate detection (same email for oauth, same name for apikey)
- Integration: delete triggers reorder
- Edge: cleanup removes null fields from JSON data
- Edge: update with partial fields (only update specified, preserve others)
- Verify all operations use `db.WithContext(ctx)` for context propagation

**Dependencies:** DB-002 (model), DB-011 (UUID hook), DB-013 (base repo)

---

#### DB-015: Provider node repository (repository/protonode.go or repository/node.go)

**Description:** CRUD for `providerNodes` table — provider endpoint/middleware configurations stored as JSON data.

**Methods:**
```go
func (r *ProviderNodeRepository) FindAll(ctx context.Context) ([]model.ProviderNode, error)
func (r *ProviderNodeRepository) FindByID(ctx context.Context, id string) (*model.ProviderNode, error)
func (r *ProviderNodeRepository) FindByType(ctx context.Context, nodeType string) ([]model.ProviderNode, error)
func (r *ProviderNodeRepository) Create(ctx context.Context, node *model.ProviderNode) error
func (r *ProviderNodeRepository) Update(ctx context.Context, node *model.ProviderNode) error
func (r *ProviderNodeRepository) Delete(ctx context.Context, id string) error
```

**Test strategy:**
- Standard CRUD with JSON data roundtrip
- `FindByType` with index scan verification
- Update preserves existing fields (merge pattern)

**Dependencies:** DB-003 (model), DB-011 (UUID hook), DB-013 (base repo)

---

#### DB-016: Proxy pool repository (repository/proxypool.go)

**Description:** CRUD for `proxyPools` table — proxy IP pool management with active/status filtering.

**Methods:**
```go
func (r *ProxyPoolRepository) FindAll(ctx context.Context) ([]model.ProxyPool, error)
func (r *ProxyPoolRepository) FindByID(ctx context.Context, id string) (*model.ProxyPool, error)
func (r *ProxyPoolRepository) FindActive(ctx context.Context) ([]model.ProxyPool, error)
func (r *ProxyPoolRepository) FindByStatus(ctx context.Context, status string) ([]model.ProxyPool, error)
func (r *ProxyPoolRepository) Create(ctx context.Context, pool *model.ProxyPool) error
func (r *ProxyPoolRepository) Update(ctx context.Context, pool *model.ProxyPool) error
func (r *ProxyPoolRepository) Delete(ctx context.Context, id string) error
```

**Test strategy:**
- Standard CRUD with JSON data
- Boolean field queries (FindActive)
- Status filter queries

**Dependencies:** DB-003 (model), DB-011 (UUID hook), DB-013 (base repo)

---

#### DB-017: API key repository (repository/apikey.go)

**Description:** CRUD for `apiKeys` table — API key management with key hashing/lookup support.

**Methods:**
```go
func (r *ApiKeyRepository) FindAll(ctx context.Context) ([]model.ApiKey, error)
func (r *ApiKeyRepository) FindByID(ctx context.Context, id string) (*model.ApiKey, error)
func (r *ApiKeyRepository) FindByKey(ctx context.Context, key string) (*model.ApiKey, error)
func (r *ApiKeyRepository) Create(ctx context.Context, key *model.ApiKey) error
func (r *ApiKeyRepository) Update(ctx context.Context, key *model.ApiKey) error
func (r *ApiKeyRepository) Delete(ctx context.Context, id string) error
func (r *ApiKeyRepository) FindValidKey(ctx context.Context, key string) (*model.ApiKey, error) // active + exists
```

**Key consideration:** `FindByKey` and `FindValidKey` are critical for auth middleware — must be performant (indexed). `FindValidKey` should filter `isActive = true`.

**Test strategy:**
- Unique constraint on `key` column
- FindValidKey returns only active keys
- FindByKey with non-existent key returns gorm.ErrRecordNotFound
- Edge: key with special characters

**Dependencies:** DB-004 (model), DB-011 (UUID hook), DB-013 (base repo)

---

#### DB-018: Combo repository (repository/combo.go)

**Description:** CRUD for `combos` table — model combo definitions with JSON models array.

**Methods:**
```go
func (r *ComboRepository) FindAll(ctx context.Context) ([]model.Combo, error)
func (r *ComboRepository) FindByID(ctx context.Context, id string) (*model.Combo, error)
func (r *ComboRepository) FindByName(ctx context.Context, name string) (*model.Combo, error)
func (r *ComboRepository) Create(ctx context.Context, combo *model.Combo) error
func (r *ComboRepository) Update(ctx context.Context, combo *model.Combo) error
func (r *ComboRepository) Delete(ctx context.Context, id string) error
```

**Test strategy:**
- Unique constraint on `name` column
- JSON array in `Models` field
- FindByName with non-existent name
- Edge: empty models array

**Dependencies:** DB-004 (model), DB-011 (UUID hook), DB-013 (base repo)

---

#### DB-019: Settings repository (repository/settings.go)

**Description:** Single-row read/write for `settings` table with JSON data parsing. The `id` constraint `CHECK(id=1)` ensures single-row enforcement.

**Methods:**
```go
func (r *SettingsRepository) Get(ctx context.Context) (*model.Setting, error)
func (r *SettingsRepository) Upsert(ctx context.Context, data string) error
func (r *SettingsRepository) GetData(ctx context.Context) (map[string]interface{}, error)  // parsed JSON
```

**Key consideration:** Unlike other repos, settings uses `Upsert` (not Update) because the row may not exist yet. Use `INSERT ... ON CONFLICT DO UPDATE` idiom via GORM's `Clause(clause.OnConflict{...})`.

**Test strategy:**
- Get returns the single row (id=1)
- Upsert creates row if absent, updates if present
- GetData returns parsed map
- Edge: empty settings data is valid JSON `{}`

**Dependencies:** DB-001 (model), DB-013 (base repo)

---

#### DB-020: Metadata repository (repository/meta.go)

**Description:** Simple key-value CRUD for `_meta` table — schema version tracking, app version, migration markers.

**Methods:**
```go
func (r *MetaRepository) Get(ctx context.Context, key string) (string, error)
func (r *MetaRepository) Set(ctx context.Context, key, value string) error
func (r *MetaRepository) Delete(ctx context.Context, key string) error
func (r *MetaRepository) GetAll(ctx context.Context) (map[string]string, error)
func (r *MetaRepository) GetInt(ctx context.Context, key string) (int, error)
```

**Test strategy:**
- Set then Get returns same value
- Get non-existent key returns empty (not error)
- GetAll returns all key-value pairs
- GetInt parses integer from value
- Delete removes key

**Dependencies:** DB-001 (model), DB-013 (base repo)

---

#### DB-021: Key-value repository (repository/kv.go)

**Description:** CRUD for `kv` table with composite primary key (scope, key) — used for model aliases, pricing data, custom models, MITM aliases, disabled models.

**Methods:**
```go
func (r *KVRepository) Get(ctx context.Context, scope, key string) (string, error)
func (r *KVRepository) Set(ctx context.Context, scope, key, value string) error
func (r *KVRepository) Delete(ctx context.Context, scope, key string) error
func (r *KVRepository) GetScope(ctx context.Context, scope string) (map[string]string, error)
func (r *KVRepository) DeleteScope(ctx context.Context, scope string) error
func (r *KVRepository) GetAll(ctx context.Context) ([]model.KV, error)
```

**Key consideration:** `GetScope` is the most-used operation — returns all key-value pairs within a scope (e.g., all pricing entries). The `scope` column has its own index (`idx_kv_scope`).

**Test strategy:**
- Composite PK enforcement (same scope + key = upsert)
- GetScope returns all entries for a scope
- DeleteScope removes all entries for a scope
- Edge: scope with large number of keys (1000+)

**Dependencies:** DB-001 (model), DB-013 (base repo)

---

#### DB-022: Usage history repository (repository/usage.go)

**Description:** Write-heavy append-only repository with aggregation queries — the performance-critical path for cost tracking and dashboard charts.

**Methods:**
```go
type UsageFilter struct {
    Provider     *string
    Model        *string
    ConnectionID *string
    ApiKey       *string
    Since        *time.Time
    Until        *time.Time
    Limit        int
    Offset       int
}

func (r *UsageRepository) Create(ctx context.Context, u *model.UsageHistory) error
func (r *UsageRepository) BatchCreate(ctx context.Context, records []model.UsageHistory) error
func (r *UsageRepository) Find(ctx context.Context, filter UsageFilter) ([]model.UsageHistory, error)
func (r *UsageRepository) AggregateByProvider(ctx context.Context, since, until time.Time) ([]UsageAggregation, error)
func (r *UsageRepository) AggregateByModel(ctx context.Context, since, until time.Time) ([]UsageAggregation, error)
func (r *UsageRepository) CostByProvider(ctx context.Context, since, until time.Time) ([]CostSummary, error)
func (r *UsageRepository) DailyStats(ctx context.Context, since, until time.Time) ([]DailyStat, error)
func (r *UsageRepository) PerKeyStats(ctx context.Context, keyID string, since, until time.Time) ([]UsageAggregation, error)
func (r *UsageRepository) TotalCost(ctx context.Context) (float64, error)
func (r *UsageRepository) TotalRequests(ctx context.Context) (int64, error)
func (r *UsageRepository) DeleteOlderThan(ctx context.Context, t time.Time) (int64, error)
```

**Key considerations:**
- `BatchCreate` must use `CreateInBatches` (GORM) or raw `INSERT INTO ... VALUES (...), (...)` for performance
- Aggregations must use raw SQL or `db.Model().Select(...).Group(...)` for `SUM`, `COUNT`, `GROUP BY`
- `DeleteOlderThan` for data retention — used by cleanup cron
- The `id` field uses BIGSERIAL in PG and AUTOINCREMENT in SQLite — never set by the application
- The migration script (`migrate-sqlite-to-postgres.js`) SKIPS the `id` column for `usageHistory`; the repository must not set it either

**Test strategy:**
- Create single record and verify auto-increment
- BatchCreate with 500 records — verify all inserted
- AggregateByProvider returns correct sums
- CostByProvider returns correct decimal values
- DailyStats shows correct daily breakdown
- DeleteOlderThan removes correct rows
- Performance benchmark: 10K rows insert

**Dependencies:** DB-005 (model), DB-013 (base repo)

---

#### DB-023: Usage daily repository (repository/usage_daily.go)

**Description:** CRUD for `usageDaily` and 5 normalized rollup tables — upsert-heavy for daily batch aggregation jobs.

**Methods:**
```go
type DailyRecord struct {
    DateKey      string
    RequestCount int64
    InputTokens  int64
    OutputTokens int64
    TotalTokens  int64
    Cost         float64
}

func (r *UsageDailyRepository) Upsert(ctx context.Context, dateKey string, data string) error  // raw usageDaily
func (r *UsageDailyRepository) Get(ctx context.Context, dateKey string) (string, error)

// Normalized rollup upserts
func (r *UsageDailyRepository) UpsertByProvider(ctx context.Context, date string, provider string, rec DailyRecord) error
func (r *UsageDailyRepository) UpsertByModel(ctx context.Context, ...) error
func (r *UsageDailyRepository) UpsertByApiKey(ctx context.Context, ...) error
func (r *UsageDailyRepository) UpsertByAccount(ctx context.Context, ...) error
func (r *UsageDailyRepository) UpsertByEndpoint(ctx context.Context, ...) error

// Bulk read for charts
func (r *UsageDailyRepository) GetRange(ctx context.Context, from, to string) ([]model.UsageDaily, error)
```

**Key consideration:** Using GORM's `Clause(clause.OnConflict{DoUpdates: clause.Assignments(map[string]interface{}{...})})` for reliable upserts.

**Test strategy:**
- Upsert creates new row on first call
- Upsert updates existing row on second call with same PK
- GetRange returns correct date range
- Verify DATE column type works in both SQLite and PG

**Dependencies:** DB-006 (models), DB-013 (base repo)

---

#### DB-024: Request details repository (repository/request_detail.go)

**Description:** CRUD for `requestDetails` table — append-only request metadata with JSON payload.

**Methods:**
```go
func (r *RequestDetailRepository) Create(ctx context.Context, d *model.RequestDetail) error
func (r *RequestDetailRepository) FindByID(ctx context.Context, id string) (*model.RequestDetail, error)
func (r *RequestDetailRepository) Find(ctx context.Context, filter UsageFilter) ([]model.RequestDetail, error)
func (r *RequestDetailRepository) DeleteOlderThan(ctx context.Context, t time.Time) (int64, error)
```

**Test strategy:**
- Standard CRUD with text PK and JSON data
- Find with filter combinations
- DeleteOlderThan for data retention

**Dependencies:** DB-007 (model), DB-011 (UUID hook), DB-013 (base repo)

---

### Group D: Migration System

---

#### DB-025: Versioned migration framework

**Description:** Implement a lightweight migration runner in Go (analogous to the Node `migrate.js` + `migrations/index.js` system). Each migration is a numbered file with `Up()` and `Down()` methods, tracked via the `_meta` table.

**Output contract:**
```go
// internal/migration/migration.go
type Migration struct {
    Version int
    Name    string
    Up      func(tx *gorm.DB) error
    Down    func(tx *gorm.DB) error
}

type Migrator struct {
    db         *gorm.DB
    migrations []Migration
}

func NewMigrator(db *gorm.DB, migrations []Migration) *Migrator
func (m *Migrator) Run(ctx context.Context) error  // applies pending migrations
func (m *Migrator) Rollback(ctx context.Context, target int) error  // rollback to version
```

**Registration pattern:**
```go
// internal/migration/register.go
var Migrations = []migration.Migration{
    {Version: 1, Name: "initial", Up: upInitial, Down: downInitial},
}
```

**Version tracking:** Uses the `_meta` table with key `"schemaVersion"` (matching the Node codebase's approach).

**Key differences from Node:**
- Node uses `WeakSet` to track per-adapter; Go uses `sync.Once` for init
- Node's `syncSchemaFromTables()` is SQLite-specific (uses `PRAGMA table_info`); Go relies on GORM's `AutoMigrate` which handles both drivers
- Node runs legacy JSON import; Go starts from a clean schema (legacy is Node's concern)

**Test strategy:**
- Run migration on empty DB — verify `schemaVersion` in `_meta` table
- Run again — idempotent, no-op
- Simulate migration failure — verify rollback sets correct version
- Verify `_meta` table creation happens before running migrations
- Bench: 100 migrations applied sequentially

**Dependencies:** DB-008 (GORM connection), DB-001 (`_meta` model), DB-013 (base repo)

---

#### DB-026: Initial schema migration

**Description:** The first migration (version 1) that stamps the initial schema. In practice, GORM's `AutoMigrate` handles the initial schema creation; this migration exists for the framework to track versioning and for future schema changes.

**Implementation:**
```go
func upInitial(tx *gorm.DB) error {
    // AutoMigrate is handled by the connection manager; this migration
    // is a marker that the schema has been initialized.
    return tx.Model(&model.Meta{}).Where("key = ?", "schemaVersion").
        Update("value", "1").Error
}

func downInitial(tx *gorm.DB) error {
    return tx.Exec("DROP TABLE IF EXISTS ...").Error  // not needed in practice
}
```

**Test strategy:**
- Run migration — verify version is recorded
- Verify no-op on re-run

**Dependencies:** DB-025 (migration framework)

---

### Group E: Cross-Cutting

---

#### DB-027: Context propagation pattern

**Description:** Establish the pattern for passing `context.Context` through all repository calls — enables cancellation, tracing, request-ID propagation, and timeout control.

**Pattern:**
```go
// All repository methods receive context.Context as first parameter
func (r *ProviderRepository) FindAll(ctx context.Context, ...) ([]model.ProviderConnection, error) {
    return r.db.WithContext(ctx).Find(&providers).Error
}

// Handler-level: create context with timeout
ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
defer cancel()
providers, err := providerRepo.FindAll(ctx)
```

**GORM hooks for trace context injection:** Register a `BeforeQuery` callback that logs the request ID from context.

**Test strategy:**
- Verify `db.WithContext(ctx)` propagates cancellation
- Verify context deadline triggers query timeout
- Verify middleware sets request-ID in context

**Dependencies:** DB-013 (base repo)

---

#### DB-028: Repository initialization and wiring

**Description:** Create a single `repository.New()` or repository factory that initializes all repositories with the shared `*gorm.DB` instance, providing a clean dependency injection point for handlers and services.

**Output contract:**
```go
// internal/repository/repository.go
type Repositories struct {
    Provider     *ProviderRepository
    ProviderNode *ProviderNodeRepository
    ProxyPool    *ProxyPoolRepository
    ApiKey       *ApiKeyRepository
    Combo        *ComboRepository
    Setting      *SettingsRepository
    Meta         *MetaRepository
    KV           *KVRepository
    Usage        *UsageRepository
    UsageDaily   *UsageDailyRepository
    RequestDetail *RequestDetailRepository
}

func NewRepositories(db *gorm.DB) *Repositories {
    return &Repositories{
        Provider:       NewProviderRepository(db),
        ProviderNode:   NewProviderNodeRepository(db),
        ProxyPool:      NewProxyPoolRepository(db),
        ApiKey:         NewApiKeyRepository(db),
        Combo:          NewComboRepository(db),
        Setting:        NewSettingsRepository(db),
        Meta:           NewMetaRepository(db),
        KV:             NewKVRepository(db),
        Usage:          NewUsageRepository(db),
        UsageDaily:     NewUsageDailyRepository(db),
        RequestDetail:  NewRequestDetailRepository(db),
    }
}
```

**Test strategy:**
- Wire up all repos with in-memory SQLite
- Verify each repo can perform basic operations
- Verify shared GORM instance (same connection pool across repos)

**Dependencies:** DB-014 through DB-024 (all individual repos)

---

#### DB-029: SQLite WAL mode and pragma configuration

**Description:** Configure SQLite-specific pragmas for optimal concurrent read performance and crash safety — matching the `PRAGMA_SQL` from `schema.js`.

**Output contract:**
```go
func ConfigureSQLite(db *gorm.DB) error {
    pragmas := []string{
        "PRAGMA journal_mode = WAL",
        "PRAGMA synchronous = NORMAL",
        "PRAGMA temp_store = MEMORY",
        "PRAGMA mmap_size = 30000000",
        "PRAGMA cache_size = -64000",
        "PRAGMA foreign_keys = ON",
        "PRAGMA busy_timeout = 5000",
    }
    for _, p := range pragmas {
        if err := db.Exec(p).Error; err != nil {
            return fmt.Errorf("sqlite pragma failed: %w", err)
        }
    }
    return nil
}
```

**Test strategy:**
- Execute during connection init for SQLite driver only
- Verify `journal_mode=wal` after pragma
- Skip entirely for PostgreSQL driver
- Verify performance improvement with pragmas (optional benchmark)

**Dependencies:** DB-008 (connection manager)

---

#### DB-030: Prepared statement caching for PostgreSQL

**Description:** Enable and configure GORM's `PrepareStmt` mode for PostgreSQL to reduce query planning overhead on hot paths, while keeping it disabled for SQLite (which uses its own statement cache via the native driver).

**Implementation detail:**
```go
// In NewGormDB():
case "postgres":
    return gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
        PrepareStmt: true,
    })

// SQLite — PrepareStmt disabled because:
// 1. SQLite's native driver (mattn/go-sqlite3 or modernc.org/sqlite) manages its own cache
// 2. GORM's PrepareStmt holds a connection, reducing concurrency
// 3. WAL mode provides better read concurrency than prepared stmts
```

**Test strategy:**
- Verify `PrepareStmt` config flag is set for PG
- Verify it is NOT set for SQLite
- Bench: hot query with and without PrepareStmt on PG

**Dependencies:** DB-008 (connection manager)

---

### Group F: Testing

---

#### DB-031: Test fixture and test database setup

**Description:** Create reusable test helpers for all database tests — in-memory SQLite initialization, model registration, per-test migration, and cleanup.

**Output contract:**
```go
// internal/repository/testing.go (or separate testutil package)
func SetupTestDB(t *testing.T, models ...interface{}) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
        SkipDefaultTransaction: true,
    })
    require.NoError(t, err)
    require.NoError(t, db.AutoMigrate(models...))
    return db
}

func SetupAllModelsTestDB(t *testing.T) *gorm.DB {
    return SetupTestDB(t,
        &model.Meta{}, &model.Setting{}, &model.ProviderConnection{},
        &model.ProviderNode{}, &model.ProxyPool{}, &model.ApiKey{},
        &model.Combo{}, &model.KV{}, &model.UsageHistory{},
        &model.UsageDaily{}, &model.RequestDetail{},
    )
}
```

**Test strategy:** Shared test helper ensures consistent setup across all repository tests.

**Dependencies:** DB-008, DB-009 (auto-migration)

---

#### DB-032: Connection manager integration tests

**Description:** Tests that validate the full connection lifecycle: initialization, driver selection, auto-migration, pooling configuration, and graceful shutdown.

**Test cases:**
- `TestNewGormDB_SQLite`: connect to `:memory:`, verify connection works
- `TestNewGormDB_Postgres`: connect to env-provided PG URL, verify connection works
- `TestConfigurePool`: verify `sqlDB.Stats()` after configuration
- `TestAutoMigrateAll`: migrate all models, verify table existence
- `TestSQLitePragma`: verify WAL mode after connection
- `TestGracefulShutdown`: close DB and verify no leaked connections
- `TestDualDriverConsistency`: same operations on both SQLite and PG produce same results

**Dependencies:** DB-008, DB-009, DB-029

---

#### DB-033: JSON column helper tests

**Description:** Exhaustive unit tests for `parseJSON` / `toJSON` utilities and typed data structs.

**Test cases:**
- `TestParseJSON`: valid JSON, invalid JSON, empty string, null
- `TestToJSON`: struct, map, array, primitive, nil
- `TestProviderConnectionData_GetData`: roundtrip all fields, missing fields, extra fields
- `TestProviderConnectionData_SetData`: set fields, verify JSON output
- `TestComboModelsData`: array serialization
- `TestEdgeCases`: nested ProviderSpecificData, special characters, large strings

**Dependencies:** DB-010

---

#### DB-034: Repository CRUD unit tests (all tables)

**Description:** Comprehensive table-driven tests for every repository method across all 11+ tables. Each repository gets its own test file.

**Test files:**
- `repository/provider_test.go` — ~25 test cases
- `repository/node_test.go` — ~10 test cases
- `repository/proxypool_test.go` — ~10 test cases
- `repository/apikey_test.go` — ~15 test cases
- `repository/combo_test.go` — ~10 test cases
- `repository/settings_test.go` — ~8 test cases
- `repository/meta_test.go` — ~8 test cases
- `repository/kv_test.go` — ~12 test cases
- `repository/usage_test.go` — ~20 test cases
- `repository/usage_daily_test.go` — ~12 test cases
- `repository/request_detail_test.go` — ~10 test cases

**Key patterns to test:**
- Standard CRUD (Create, Read, Update, Delete)
- Edge cases (null fields, empty arrays, missing records)
- Transactional operations (create-then-reorder, batch inserts)
- Filter queries (FindByProvider, FindByScope)
- Aggregation queries (SUM, COUNT, GROUP BY)
- Upsert operations (settings, kv, usageDaily)
- Concurrent access safety

**Dependencies:** DB-014 through DB-024 (all repos), DB-031 (test setup)

---

#### DB-035: Auto-migration and index verification

**Description:** Verify that `AutoMigrateAll` creates the correct schema — all tables, columns, indexes, and constraints — matching the source-of-truth `schema.js` / `schema.postgres.js` definitions.

**Test cases:**
- `TestAutoMigrate_AllTables`: count and name verification
- `TestIndexCreation`: verify all indexes exist via `db.Migrator().HasIndex()`
- `TestIdempotency`: run auto-migrate twice, no error
- `TestAdditiveSchemaChange`: add column, re-run, verify column exists
- `TestConstraintCreation`: verify CHECK(id=1) on settings, UNIQUE on apiKeys.key, composite PK on kv

**Dependencies:** DB-009

---

#### DB-036: Migration system tests

**Description:** Test the versioned migration framework — applying, idempotency, rollback, version tracking.

**Test cases:**
- `TestMigrationApply`: run all migrations, verify version in `_meta`
- `TestMigrationIdempotency`: run twice, no error, version unchanged
- `TestMigrationRollback`: rollback to version N, verify version
- `TestMigrationPartialFailure`: ensure version is NOT incremented on failure
- `TestMigrationEmpty`: no migrations, version is 0

**Dependencies:** DB-025, DB-026

---

#### DB-037: Dual-driver parity tests

**Description:** Same operations executed against both SQLite and PostgreSQL — verify identical results. Catches driver-specific behavior (type coercion, default values, NULL handling).

**Test pattern:**
```go
func TestDualDriver_Parity(t *testing.T) {
    tests := []struct{
        name string
        fn   func(t *testing.T, db *gorm.DB)
    }{
        {"CRUD ProviderConnection", testProviderCRUD},
        {"JSON Column Roundtrip", testJSONRoundtrip},
        {"Bool Coercion", testBoolCoercion},
        {"Timestamp Precision", testTimestampPrecision},
        {"Numeric Precision", testNumericPrecision},
        {"AutoIncrement", testAutoIncrement},
    }

    for _, driver := range []string{"sqlite", "postgres"} {
        db := setupDriverDB(t, driver)
        t.Run(driver, func(t *testing.T) {
            for _, tt := range tests {
                t.Run(tt.name, func(t *testing.T) { tt.fn(t, db) })
            }
        })
    }
}
```

**Dependencies:** DB-032 (connection setup), DB-014 through DB-024 (repo operations)

---

## Dependency Graph

```
DB-001  DB-002  DB-003  DB-004  DB-005  DB-006  DB-007  DB-010 (parallel)
    \       |       |       |       |       |       |      /
     \      |       |       |       |       |       |     /
      \     |       |       |       |       |       |    /
       v    v       v       v       v       v       v   v
                        DB-008
                           |
                        DB-009
                        /  |  \
                       /   |   \
              DB-011  DB-012  DB-029
                 |
        +--------+--------+---------+---------+---------+
        |        |        |         |         |         |
    DB-014   DB-015   DB-016    DB-017    DB-018    DB-019
    DB-020   DB-021   DB-022    DB-023    DB-024
        |        |        |         |         |         |
        +--------+--------+---------+---------+---------+
                           |
                        DB-028
                           |
               DB-025 --- DB-026
                           |
        DB-027  DB-030  (cross-cutting)

Testing (after implementation):
    DB-031 → DB-032 → DB-033 → DB-034 → DB-035 → DB-036 → DB-037
```

---

## Implementation Order

| Phase | Tasks | Description | Effort |
|-------|-------|-------------|--------|
| T1 | DB-010 | JSON helpers (zero dependencies) | 1h |
| T2 | DB-001..007 | All model structs (7 parallel tasks) | 2h |
| T3 | DB-008, DB-012, DB-029, DB-030 | Connection manager + logging + SQLite pragma + PG prep stmt | 3h |
| T4 | DB-009 | Auto-migration setup | 1h |
| T5 | DB-011 | UUID + timestamp hooks | 1h |
| T6 | DB-013 | Base repository pattern | 1h |
| T7 | DB-014..024 | 11 repository implementations (can be parallelized) | 8h |
| T8 | DB-028 | Repository initialization and wiring | 1h |
| T9 | DB-025, DB-026 | Migration framework + initial migration | 2h |
| T10 | DB-027 | Context propagation | 1h |
| T11 | DB-031..037 | All tests | 6h |

**Total estimated effort: ~27 hours (3-4 days for one developer)**

---

## Key Duality Notes (SQLite vs PostgreSQL)

| Feature | SQLite | PostgreSQL | GORM Handling |
|---------|--------|------------|---------------|
| PK type | `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` | `autoIncrement` tag |
| PK type | `TEXT PRIMARY KEY` | `TEXT PRIMARY KEY` | `primaryKey;type:text` |
| Boolean | `INTEGER 0/1` | `BOOLEAN true/false` | `*bool` / `default:true` |
| JSON | `TEXT` (string) | `JSONB` (native) | `string` field, type:text |
| Timestamp | `TEXT` (ISO string) | `TIMESTAMPTZ` | `time.Time` |
| Decimal | `REAL` | `NUMERIC(12,6)` | `float64`, type:numeric(12,6) |
| Auto-ID | `INTEGER AUTOINCREMENT` | `BIGSERIAL` | `autoIncrement` + omit on insert |
| Composite PK | `PRIMARY KEY (a, b)` | `PRIMARY KEY (a, b)` | Two `primaryKey` tags |
| WAL | Manual PRAGMA | Server-managed | `Exec(PRAGMA)` on SQLite only |
| Unique | `UNIQUE` | `UNIQUE` | `uniqueIndex` |
| CONFLICT | `ON CONFLICT DO NOTHING` | `ON CONFLICT DO NOTHING` | `Clause(clause.OnConflict{...})` |
| Placeholders | `?` | `$1, $2...` | GORM handles internally |

---

## Key Column Name Mapping (Node camelCase vs Go snake_case)

The Node codebase uses camelCase column names throughout (e.g., `connectionId`, `authType`, `isActive`, `createdAt`). GORM's default naming strategy converts struct field names to snake_case (e.g., `ConnectionID` → `connection_id`). To maintain compatibility:

```go
// Option A: Override GORM naming strategy to use camelCase columns
db, err := gorm.Open(driver, &gorm.Config{
    NamingStrategy: schema.NamingStrategy{
        TablePrefix:   "",
        SingularTable: false,
        NameReplacer:  nil,  // GORM defaults to snake_case
    },
})

// Option B: Explicit `column:` tag on every field
type ProviderConnection struct {
    ID         string    `gorm:"primaryKey;type:text;column:id"`
    Provider   string    `gorm:"not null;column:provider"`
    AuthType   string    `gorm:"not null;column:authType"`
    // ...
}
```

**Recommendation:** Use `column:` tags on every field to match the existing schema exactly. This avoids migration complexity when the Go backend reads databases created by the Node backend during parallel run (Phase 5). The existing PostgreSQL schema uses camelCase column names; changing to snake_case would break the migration path.
