package repository

import (
	"context"
	"errors"
	"time"

	"github.com/9router/9router/internal/model"
	"gorm.io/gorm"
)

// ErrProviderNotFound is returned when a connection lookup yields no
// row. Callers can use errors.Is to test for it.
var ErrProviderNotFound = errors.New("provider connection not found")

// =====================================================================
// Per-model repository stubs
// =====================================================================
//
// Each repository is intentionally minimal: the DB-015 task only
// requires the factory to wire them. The actual CRUD operations are
// added by the corresponding domain tasks (PROV-004, AUTH-007,
// USAGE-002, SYS-002, etc.). Until those land, repositories can still
// be constructed and used via the embedded BaseRepository.

type ProviderRepository struct{ BaseRepository }

func NewProviderRepository(db *gorm.DB) *ProviderRepository {
	return &ProviderRepository{BaseRepository{db: db}}
}

// ListAll is a placeholder used by the DB-015 tests; replaced by
// PROV-003 in the providers domain.
func (r *ProviderRepository) ListAll() ([]model.ProviderConnection, error) {
	var out []model.ProviderConnection
	if err := r.db.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ProviderFilter narrows a List query. All fields are optional.
type ProviderFilter struct {
	Provider string
	AuthType string
	IsActive *bool
}

// HistoryFilter narrows a usage history query.
type HistoryFilter struct {
	Provider     string
	Model        string
	ConnectionID string
	ApiKey       string
	Status       string
	StartDate    time.Time
	EndDate      time.Time
	Limit        int
	Offset       int
}

// RequestDetailFilter narrows a request detail query.
type RequestDetailFilter struct {
	Provider     string
	Model        string
	ConnectionID string
	Status       string
	StartDate    time.Time
	EndDate      time.Time
	Page         int
	PageSize     int
}

// List returns connections matching the filter, ordered by priority
// ASC then createdAt ASC.
func (r *ProviderRepository) List(ctx context.Context, f ProviderFilter) ([]model.ProviderConnection, error) {
	q := r.db.WithContext(ctx).Model(&model.ProviderConnection{})
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	if f.AuthType != "" {
		q = q.Where("authType = ?", f.AuthType)
	}
	if f.IsActive != nil {
		q = q.Where("isActive = ?", *f.IsActive)
	}
	var out []model.ProviderConnection
	if err := q.Order("priority ASC, createdAt ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// GetByID returns a single connection by primary key.
func (r *ProviderRepository) GetByID(ctx context.Context, id string) (*model.ProviderConnection, error) {
	var pc model.ProviderConnection
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&pc).Error; err != nil {
		if err.Error() == "record not found" {
			return nil, ErrProviderNotFound
		}
		return nil, err
	}
	return &pc, nil
}

// Create persists a new connection row. The ID is set by the caller.
func (r *ProviderRepository) Create(ctx context.Context, pc *model.ProviderConnection) error {
	return r.db.WithContext(ctx).Create(pc).Error
}

// Update saves changes to an existing connection row.
func (r *ProviderRepository) Update(ctx context.Context, pc *model.ProviderConnection) error {
	return r.db.WithContext(ctx).Save(pc).Error
}

// Delete removes a connection row by primary key. Returns
// ErrProviderNotFound when no row matches.
func (r *ProviderRepository) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.ProviderConnection{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrProviderNotFound
	}
	return nil
}

type ProviderNodeRepository struct{ BaseRepository }

func NewProviderNodeRepository(db *gorm.DB) *ProviderNodeRepository {
	return &ProviderNodeRepository{BaseRepository{db: db}}
}

func (r *ProviderNodeRepository) ListAll() ([]model.ProviderNode, error) {
	var out []model.ProviderNode
	if err := r.db.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

type ProxyPoolRepository struct{ BaseRepository }

func NewProxyPoolRepository(db *gorm.DB) *ProxyPoolRepository {
	return &ProxyPoolRepository{BaseRepository{db: db}}
}

func (r *ProxyPoolRepository) ListAll() ([]model.ProxyPool, error) {
	var out []model.ProxyPool
	if err := r.db.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

type ApiKeyRepository struct{ BaseRepository }

func NewApiKeyRepository(db *gorm.DB) *ApiKeyRepository {
	return &ApiKeyRepository{BaseRepository{db: db}}
}

func (r *ApiKeyRepository) ListAll() ([]model.ApiKey, error) {
	var out []model.ApiKey
	if err := r.db.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

type ComboRepository struct{ BaseRepository }

func NewComboRepository(db *gorm.DB) *ComboRepository {
	return &ComboRepository{BaseRepository{db: db}}
}

func (r *ComboRepository) ListAll() ([]model.Combo, error) {
	var out []model.Combo
	if err := r.db.Order("createdAt ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ComboRepository) FindByID(ctx context.Context, id string) (*model.Combo, error) {
	var c model.Combo
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *ComboRepository) FindByName(ctx context.Context, name string) (*model.Combo, error) {
	var c model.Combo
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *ComboRepository) Create(ctx context.Context, c *model.Combo) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *ComboRepository) Update(ctx context.Context, c *model.Combo) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *ComboRepository) Delete(ctx context.Context, id string) (bool, error) {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Combo{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

type SettingsRepository struct{ BaseRepository }

func NewSettingsRepository(db *gorm.DB) *SettingsRepository {
	return &SettingsRepository{BaseRepository{db: db}}
}

func (r *SettingsRepository) Get() (*model.Setting, error) {
	var s model.Setting
	if err := r.db.First(&s, "id = ?", 1).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

type MetaRepository struct{ BaseRepository }

func NewMetaRepository(db *gorm.DB) *MetaRepository {
	return &MetaRepository{BaseRepository{db: db}}
}

func (r *MetaRepository) Get(key string) (string, error) {
	var m model.Meta
	if err := r.db.First(&m, "`key` = ?", key).Error; err != nil {
		return "", err
	}
	return m.Value, nil
}

func (r *MetaRepository) Set(key, value string) error {
	return r.db.Save(&model.Meta{Key: key, Value: value}).Error
}

type KVRepository struct{ BaseRepository }

func NewKVRepository(db *gorm.DB) *KVRepository {
	return &KVRepository{BaseRepository{db: db}}
}

type UsageRepository struct{ BaseRepository }

func NewUsageRepository(db *gorm.DB) *UsageRepository {
	return &UsageRepository{BaseRepository{db: db}}
}

func (r *UsageRepository) ListAll() ([]model.UsageHistory, error) {
	var out []model.UsageHistory
	if err := r.db.Order("timestamp DESC, id DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *UsageRepository) Save(ctx context.Context, h *model.UsageHistory) error {
	return r.db.WithContext(ctx).Create(h).Error
}

func (r *UsageRepository) SaveBatch(ctx context.Context, batch []model.UsageHistory) error {
	if len(batch) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(batch, 50).Error
}

func (r *UsageRepository) GetHistory(ctx context.Context, filter HistoryFilter) ([]model.UsageHistory, error) {
	q := r.db.WithContext(ctx).Model(&model.UsageHistory{})
	if filter.Provider != "" {
		q = q.Where("provider = ?", filter.Provider)
	}
	if filter.Model != "" {
		q = q.Where("model = ?", filter.Model)
	}
	if filter.ConnectionID != "" {
		q = q.Where("connectionId = ?", filter.ConnectionID)
	}
	if filter.ApiKey != "" {
		q = q.Where("apiKey = ?", filter.ApiKey)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if !filter.StartDate.IsZero() {
		q = q.Where("timestamp >= ?", filter.StartDate)
	}
	if !filter.EndDate.IsZero() {
		q = q.Where("timestamp <= ?", filter.EndDate)
	}
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	var out []model.UsageHistory
	if err := q.Order("timestamp DESC, id DESC").Limit(int(filter.Limit)).Offset(int(filter.Offset)).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

type UsageDailyRepository struct{ BaseRepository }

func NewUsageDailyRepository(db *gorm.DB) *UsageDailyRepository {
	return &UsageDailyRepository{BaseRepository{db: db}}
}

func (r *UsageDailyRepository) GetByDate(dateKey string) (*model.UsageDaily, error) {
	var u model.UsageDaily
	if err := r.db.First(&u, "dateKey = ?", dateKey).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

type RequestDetailRepository struct{ BaseRepository }

func NewRequestDetailRepository(db *gorm.DB) *RequestDetailRepository {
	return &RequestDetailRepository{BaseRepository{db: db}}
}

func (r *RequestDetailRepository) ListAll() ([]model.RequestDetail, error) {
	var out []model.RequestDetail
	if err := r.db.Order("timestamp DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RequestDetailRepository) Save(ctx context.Context, d *model.RequestDetail) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *RequestDetailRepository) GetByID(ctx context.Context, id string) (*model.RequestDetail, error) {
	var d model.RequestDetail
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *RequestDetailRepository) GetFiltered(ctx context.Context, f RequestDetailFilter) ([]model.RequestDetail, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.RequestDetail{})
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	if f.Model != "" {
		q = q.Where("model = ?", f.Model)
	}
	if f.ConnectionID != "" {
		q = q.Where("connectionId = ?", f.ConnectionID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if !f.StartDate.IsZero() {
		q = q.Where("timestamp >= ?", f.StartDate)
	}
	if !f.EndDate.IsZero() {
		q = q.Where("timestamp <= ?", f.EndDate)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var rows []model.RequestDetail
	if err := q.Order("timestamp DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// =====================================================================
// Aggregate + factory
// =====================================================================

// Repositories is the single dependency-injection struct passed to
// handlers, services, and background workers.
type Repositories struct {
	Provider      *ProviderRepository
	ProviderNode  *ProviderNodeRepository
	ProxyPool     *ProxyPoolRepository
	ApiKey        *ApiKeyRepository
	Combo         *ComboRepository
	Setting       *SettingsRepository
	Meta          *MetaRepository
	KV            *KVRepository
	Usage         *UsageRepository
	UsageDaily    *UsageDailyRepository
	RequestDetail *RequestDetailRepository

	db *gorm.DB
}

// NewRepositories constructs all 11 repositories sharing the same
// *gorm.DB. Passing a nil *gorm.DB panics — this is a programmer
// error caught at startup, not a runtime condition.
func NewRepositories(db *gorm.DB) *Repositories {
	if db == nil {
		panic("NewRepositories: db is nil")
	}
	return &Repositories{
		Provider:      NewProviderRepository(db),
		ProviderNode:  NewProviderNodeRepository(db),
		ProxyPool:     NewProxyPoolRepository(db),
		ApiKey:        NewApiKeyRepository(db),
		Combo:         NewComboRepository(db),
		Setting:       NewSettingsRepository(db),
		Meta:          NewMetaRepository(db),
		KV:            NewKVRepository(db),
		Usage:         NewUsageRepository(db),
		UsageDaily:    NewUsageDailyRepository(db),
		RequestDetail: NewRequestDetailRepository(db),
		db:            db,
	}
}

// DB returns the underlying *gorm.DB shared by all repositories.
func (r *Repositories) DB() *gorm.DB { return r.db }

// Close closes the underlying *sql.DB. After Close, queries will fail
// with "sql: database is closed" until a new Repositories is built.
func (r *Repositories) Close() error {
	if r.db == nil {
		return errors.New("repos: db is nil")
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
