package repository

import (
	"gorm.io/gorm"
)

// BaseRepository provides the shared *gorm.DB handle plus a few
// common helpers (begin transaction, count, etc.) for all per-model
// repositories. Each concrete repository embeds it.
type BaseRepository struct {
	db *gorm.DB
}

// DB returns the underlying *gorm.DB. Useful for callers that need to
// do raw SQL or chain further GORM clauses.
func (b *BaseRepository) DB() *gorm.DB {
	return b.db
}

// Count returns the row count for the given model, applying optional
// WHERE conditions via tx.
func (b *BaseRepository) Count(model interface{}, conds ...interface{}) (int64, error) {
	var n int64
	q := b.db.Model(model)
	if len(conds) > 0 {
		q = q.Where(conds[0], conds[1:]...)
	}
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}
