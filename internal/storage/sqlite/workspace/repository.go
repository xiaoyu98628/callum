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
		switch {
		case err == nil:
			if row.DeletedAt == nil {
				return domain.ErrConflict
			}
			if err := tx.Model(&row).Updates(map[string]any{"name": in.Name, "deleted_at": nil, "updated_at": in.UpdatedAt}).Error; err != nil {
				return err
			}
			row.Name, row.DeletedAt, row.UpdatedAt = in.Name, nil, in.UpdatedAt
			if in.OtherPaths != nil {
				if err := tx.Where("workspace_id = ?", row.ID).Delete(&schema.WorkspaceAdditionalFolder{}).Error; err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			row = schema.Workspace{ID: in.ID, Name: in.Name, RootPath: in.RootPath, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		default:
			return err
		}
		if len(in.OtherPaths) > 0 {
			folders := make([]schema.WorkspaceAdditionalFolder, 0, len(in.OtherPaths))
			for _, path := range in.OtherPaths {
				folders = append(folders, schema.WorkspaceAdditionalFolder{WorkspaceID: row.ID, Path: path})
			}
			if err := tx.Create(&folders).Error; err != nil {
				return err
			}
		}
		out, err = workspaceWithFolders(tx, row)
		return err
	})
	return out, err
}

func (r *Repository) Get(ctx context.Context, id string) (out domain.Workspace, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Workspace
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		out, err = workspaceWithFolders(tx, row)
		return err
	})
	return out, err
}

func (r *Repository) List(ctx context.Context, filter domain.Filter) (out []domain.Workspace, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&schema.Workspace{})
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
			return err
		}
		out = make([]domain.Workspace, 0, len(rows))
		ids := make([]string, 0, len(rows))
		index := make(map[string]int, len(rows))
		for _, row := range rows {
			index[row.ID] = len(out)
			ids = append(ids, row.ID)
			out = append(out, workspaceFromRow(row))
		}
		folders, err := otherFolders(tx, ids)
		if err != nil {
			return err
		}
		for _, folder := range folders {
			value := &out[index[folder.WorkspaceID]]
			value.OtherPaths = append(value.OtherPaths, folder.Path)
		}
		return nil
	})
	return out, err
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
		out, err = workspaceWithFolders(tx, row)
		return err
	})
	return out, err
}

func workspaceFromRow(row schema.Workspace) domain.Workspace {
	return domain.Workspace{
		ID: row.ID, Name: row.Name, RootPath: row.RootPath, OtherPaths: []string{},
		PinnedAt: row.PinnedAt, ArchivedAt: row.ArchivedAt, DeletedAt: row.DeletedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func workspaceWithFolders(tx *gorm.DB, row schema.Workspace) (domain.Workspace, error) {
	folders, err := otherFolders(tx, []string{row.ID})
	if err != nil {
		return domain.Workspace{}, err
	}
	value := workspaceFromRow(row)
	for _, folder := range folders {
		value.OtherPaths = append(value.OtherPaths, folder.Path)
	}
	return value, nil
}

func otherFolders(tx *gorm.DB, ids []string) ([]schema.WorkspaceAdditionalFolder, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var folders []schema.WorkspaceAdditionalFolder
	err := tx.Select("workspace_id", "path").Where("workspace_id IN ?", ids).Order("path").Find(&folders).Error
	return folders, err
}
