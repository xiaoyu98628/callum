package workspace

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	domain "github.com/xiaoyu98628/callum/internal/workspace"
)

// Repository 持有借用的 GORM 实例，不负责关闭连接。
type Repository struct{ db *gorm.DB }

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, domain.ErrInvalid
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Create(ctx context.Context, in domain.Workspace) (out domain.Workspace, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Workspace
		err := tx.Where("root_path = ?", in.RootPath).Take(&row).Error
		if err == nil {
			if row.DeletedAt == nil {
				return domain.ErrConflict
			}
			if err := tx.Model(&row).Updates(map[string]any{"name": in.Name, "deleted_at": nil, "updated_at": in.UpdatedAt}).Error; err != nil {
				return err
			}
			row.Name, row.DeletedAt, row.UpdatedAt = in.Name, nil, in.UpdatedAt
			out = domain.Workspace(row)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row = schema.Workspace(in)
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		out = in
		return nil
	})
	return out, err
}

func (r *Repository) Get(ctx context.Context, id string) (domain.Workspace, error) {
	var row schema.Workspace
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = domain.ErrNotFound
	}
	return domain.Workspace(row), err
}

func (r *Repository) List(ctx context.Context, filter domain.Filter) ([]domain.Workspace, error) {
	query := r.db.WithContext(ctx).Model(&schema.Workspace{})
	if filter.Deleted {
		query = query.Where("deleted_at IS NOT NULL")
	} else {
		query = query.Where("deleted_at IS NULL")
		if filter.Archived {
			query = query.Where("archived_at IS NOT NULL")
		} else {
			query = query.Where("archived_at IS NULL")
		}
	}
	var rows []schema.Workspace
	if err := query.Order("pinned_at DESC").Order("updated_at DESC").Order("id DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Workspace, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Workspace(row))
	}
	return out, nil
}

func (r *Repository) Update(ctx context.Context, id string, change domain.Change) (out domain.Workspace, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Workspace
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if change.Archived != nil || change.Deleted != nil {
			var count int64
			if err := tx.Model(&schema.Run{}).Joins("JOIN conversations ON conversations.id = conversation_runs.conversation_id").Where("conversations.workspace_id = ? AND conversation_runs.status IN ?", id, []string{"running", "awaiting_approval", "awaiting_user_input"}).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return domain.ErrBusy
			}
		}
		now := time.Now().UnixMilli()
		updates := map[string]any{"updated_at": now}
		if change.Name != nil {
			row.Name = *change.Name
			updates["name"] = row.Name
		}
		for key, flag := range map[string]*bool{"pinned_at": change.Pinned, "archived_at": change.Archived, "deleted_at": change.Deleted} {
			if flag != nil {
				if *flag {
					updates[key] = now
				} else {
					updates[key] = nil
				}
			}
		}
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			return err
		}
		out = domain.Workspace(row)
		return nil
	})
	return out, err
}
