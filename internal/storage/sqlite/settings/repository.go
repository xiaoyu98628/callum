package settings

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"

	domain "github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// Repository 借用 GORM 实例；传入受理事务时，Get 使用该事务而不另占连接。
type Repository struct{ db *gorm.DB }

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, domain.ErrInvalid
	}
	return &Repository{db: db}, nil
}

func record(row schema.Settings) domain.Record {
	return domain.Record{ID: row.ID, Key: row.Key, Value: row.Value, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (r *Repository) Get(ctx context.Context, key string) (domain.Record, error) {
	var row schema.Settings
	err := r.db.WithContext(ctx).Where("key = ?", key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Record{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Record{}, errors.Join(domain.ErrStorage, err)
	}
	return record(row), nil
}

func (r *Repository) Update(ctx context.Context, key string, revision int64, value json.RawMessage) (out domain.Record, retErr error) {
	if revision < 1 || revision == math.MaxInt64 || len(value) > domain.MaxValueBytes {
		return out, domain.ErrInvalid
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Settings
		if err := tx.Where("key = ?", key).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if row.Revision != revision {
			return domain.ErrConflict
		}
		row.Value, row.Revision, row.UpdatedAt = value, revision+1, time.Now().UnixMilli()
		result := tx.Model(&schema.Settings{}).Where("id = ? AND revision = ?", row.ID, revision).Select("Value", "Revision", "UpdatedAt").Updates(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		out = record(row)
		return nil
	})
	if err != nil && !errors.Is(err, domain.ErrConflict) && !errors.Is(err, domain.ErrNotFound) {
		return domain.Record{}, errors.Join(domain.ErrStorage, err)
	}
	return out, err
}
