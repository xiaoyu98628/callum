package conversation

import (
	"context"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// Repository 实现会话业务事务，借用连接，不持有跨调用事务。
type Repository struct{ db *gorm.DB }

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, domain.ErrInvalid
	}
	return &Repository{db: db}, nil
}

func convert(row schema.Conversation) domain.Conversation {
	return domain.Conversation{ID: row.ID, WorkspaceID: row.WorkspaceID, Title: row.Title, TitleSource: row.TitleSource, Model: row.Model, PinnedAt: row.PinnedAt, ArchivedAt: row.ArchivedAt, DeletedAt: row.DeletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func message(row schema.Message) domain.Message {
	return domain.Message{ID: row.ID, ConversationID: row.ConversationID, RunID: row.RunID, Sequence: row.Sequence, Role: row.Role, Content: row.Content, Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, CompletedAt: row.CompletedAt}
}

func (r *Repository) Get(ctx context.Context, id string) (domain.Conversation, error) {
	var row schema.Conversation
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = domain.ErrNotFound
	}
	return convert(row), err
}

func (r *Repository) List(ctx context.Context, f domain.Filter) ([]domain.Conversation, error) {
	q := r.db.WithContext(ctx).Model(&schema.Conversation{}).Select("conversations.*").Joins("LEFT JOIN workspaces ON workspaces.id = conversations.workspace_id")
	if f.WorkspaceID != nil {
		q = q.Where("conversations.workspace_id = ?", *f.WorkspaceID)
	}
	if f.Unbound {
		q = q.Where("conversations.workspace_id IS NULL")
	}
	if f.Deleted {
		q = q.Where("conversations.deleted_at IS NOT NULL")
	} else {
		q = q.Where("conversations.deleted_at IS NULL AND (workspaces.id IS NULL OR workspaces.deleted_at IS NULL)")
		if f.Archived {
			q = q.Where("conversations.archived_at IS NOT NULL")
		} else {
			q = q.Where("conversations.archived_at IS NULL AND (workspaces.id IS NULL OR workspaces.archived_at IS NULL)")
		}
	}
	var rows []schema.Conversation
	if err := q.Order("conversations.pinned_at DESC").Order("conversations.updated_at DESC").Order("conversations.id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Conversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, convert(row))
	}
	return out, nil
}

func busy(tx *gorm.DB, id string) error {
	var count int64
	if err := tx.Model(&schema.Run{}).Where("conversation_id = ? AND status IN ?", id, []string{"running", "awaiting_approval", "awaiting_user_input"}).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return domain.ErrBusy
	}
	return nil
}

func activeWorkspace(tx *gorm.DB, id *string) error {
	if id == nil {
		return nil
	}
	var row schema.Workspace
	if err := tx.Where("id = ?", *id).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	if row.DeletedAt != nil || row.ArchivedAt != nil {
		return domain.ErrInactive
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, c domain.Change) (out domain.Conversation, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Conversation
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if c.Model != nil || c.WorkspaceID != nil || c.DetachWorkspace || c.Archived != nil || c.Deleted != nil {
			if err := busy(tx, id); err != nil {
				return err
			}
		}
		now := time.Now().UnixMilli()
		updates := map[string]any{"updated_at": now}
		if c.Title != nil {
			updates["title"] = *c.Title
			updates["title_source"] = "manual"
		}
		if c.Model != nil {
			row.Model = *c.Model
			if err := tx.Model(&row).Select("Model").Updates(&row).Error; err != nil {
				return err
			}
		}
		if c.WorkspaceID != nil {
			if err := activeWorkspace(tx, c.WorkspaceID); err != nil {
				return err
			}
			updates["workspace_id"] = *c.WorkspaceID
		}
		if c.DetachWorkspace {
			updates["workspace_id"] = nil
		}
		for key, flag := range map[string]*bool{"pinned_at": c.Pinned, "archived_at": c.Archived, "deleted_at": c.Deleted} {
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
		out = convert(row)
		return nil
	})
	return out, err
}

func (r *Repository) Messages(ctx context.Context, id string, before int64, limit int) ([]domain.Message, error) {
	if _, err := r.Get(ctx, id); err != nil {
		return nil, err
	}
	q := r.db.WithContext(ctx).Where("conversation_id = ?", id)
	if before > 0 {
		q = q.Where("sequence < ?", before)
	}
	// 先读取大小，控制整个历史查询的内存和协议响应，不把 limit 当作字节上限。
	var sizes []struct {
		ID    string
		Bytes int64
	}
	if err := q.Model(&schema.Message{}).Select("id, length(CAST(content AS BLOB)) AS bytes").Order("sequence DESC").Limit(limit).Find(&sizes).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(sizes))
	total := int64(0)
	for _, row := range sizes {
		if total+row.Bytes+1024 > 7<<20 {
			if len(ids) == 0 {
				return nil, agent.ErrOutputLimit
			}
			break
		}
		ids = append(ids, row.ID)
		total += row.Bytes + 1024
	}
	if len(ids) == 0 {
		return []domain.Message{}, nil
	}
	var rows []schema.Message
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("sequence DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(rows))
	for _, row := range slices.Backward(rows) {
		out = append(out, message(row))
	}
	return out, nil
}
