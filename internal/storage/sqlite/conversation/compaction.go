package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	settingsstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/settings"
)

func executionSettings(tx *gorm.DB, ctx context.Context) (settings.Snapshot, error) {
	repository, err := settingsstore.NewRepository(tx)
	if err != nil {
		return settings.Snapshot{}, err
	}
	record, err := repository.Get(ctx, settings.ExecutionKey)
	if err != nil {
		return settings.Snapshot{}, err
	}
	snapshot, err := settings.ExecutionSnapshot(record)
	if err != nil {
		return settings.Snapshot{}, err
	}
	record, err = repository.Get(ctx, settings.CompactionKey)
	if err != nil {
		return settings.Snapshot{}, err
	}
	compression, err := settings.DecodeCompactionSnapshot(record)
	if err != nil {
		return settings.Snapshot{}, err
	}
	snapshot.Compaction = &compression
	return snapshot, nil
}

// BeginCompaction 与消息发送使用相同的会话互斥和设置事务，不创建用户或回答消息。
func (r *Repository) BeginCompaction(ctx context.Context, in domain.Begin) (out domain.Execution, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		snapshot, err := executionSettings(tx, ctx)
		if err != nil {
			return err
		}
		var conv schema.Conversation
		if err := tx.Where("id=?", in.ConversationID).Take(&conv).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if conv.ArchivedAt != nil || conv.DeletedAt != nil {
			return domain.ErrInactive
		}
		if err := activeWorkspace(tx, conv.WorkspaceID); err != nil {
			return err
		}
		if err := busy(tx, conv.ID); err != nil {
			return err
		}
		out.History, out.Context, out.HistoryMore, err = loadContextHistory(tx, conv.ID, 0)
		if err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		row := schema.Run{ID: in.RunID, ConversationID: conv.ID, Model: in.Model,
			Stream: false, SettingsSnapshot: &snapshot, Status: agent.Running, CreatedAt: now, UpdatedAt: now}
		if err := tx.Omit("Conversation", "Replaces").Create(&row).Error; err != nil {
			return err
		}
		input, err := json.Marshal(map[string]any{"purpose": "context_compaction", "trigger": map[string]string{"type": "manual", "reason": "user_request"}})
		if err != nil {
			return err
		}
		step := schema.Step{ID: in.StepID, ConversationID: conv.ID, RunID: row.ID, Sequence: 1, Type: "context_compression",
			Name: "压缩历史上下文", Status: agent.Pending, Input: input, CreatedAt: now, UpdatedAt: now}
		if err := tx.Omit("Run", "Parent").Create(&step).Error; err != nil {
			return err
		}
		out.Conversation = convert(conv)
		out.Run = agent.Run{ID: row.ID, ConversationID: conv.ID, Model: in.Model, SettingsSnapshot: &snapshot, Status: agent.Running, CreatedAt: now, UpdatedAt: now}
		out.Step = agent.Step{ID: step.ID, ConversationID: conv.ID, RunID: row.ID, Sequence: 1, Type: step.Type,
			Name: step.Name, Status: step.Status, Input: input, CreatedAt: now, UpdatedAt: now}
		return nil
	})
	return out, err
}
