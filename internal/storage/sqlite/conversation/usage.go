package conversation

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// ContextState 在短事务中读取设置、有效摘要及消息边界，不计算 Token 或持有运行缓存。
func (r *Repository) ContextState(ctx context.Context, id string) (out domain.ContextState, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out.Settings, err = executionSettings(tx, ctx)
		if err != nil || id == "" {
			return err
		}
		var row schema.Conversation
		if err := tx.Where("id=?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if row.DeletedAt != nil || row.ArchivedAt != nil {
			return domain.ErrInactive
		}
		if err := activeWorkspace(tx, row.WorkspaceID); err != nil {
			return err
		}
		out.Conversation = convert(row)
		out.Context, err = activeContext(tx, id)
		if err != nil {
			return err
		}
		if err := tx.Model(&schema.Message{}).Where("conversation_id=?", id).Select("COALESCE(MAX(sequence),0)").Scan(&out.ThroughSequence).Error; err != nil {
			return err
		}
		if err := tx.Model(&schema.Message{}).Where("conversation_id=? AND role='user'", id).Select("run_id").Order("sequence DESC").Limit(1).Scan(&out.LastUserRunID).Error; err != nil {
			return err
		}
		var running schema.Run
		err = tx.Where("conversation_id=? AND status IN ?", id, []string{agent.Running, agent.AwaitingApproval, agent.AwaitingUserInput}).Take(&running).Error
		if err == nil {
			out.ActiveRun = &agent.Run{ID: running.ID, ConversationID: running.ConversationID, Model: running.Model, SettingsSnapshot: running.SettingsSnapshot, Status: running.Status}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var actual struct {
			Tokens  *int64
			ModelID string
		}
		// 只读取当前消息分支的主模型调用；未知或旧记录保留空值，不解释厂商私有 usage。
		if err := tx.Table("conversation_run_steps AS s").
			Joins("JOIN conversation_runs AS r ON r.id=s.run_id").
			Joins("JOIN conversation_messages AS m ON m.run_id=r.id AND m.role='user'").
			Where("s.conversation_id=? AND s.type='model_call' AND s.parent_step_id IS NULL AND (json_extract(s.input,'$.purpose')='answer' OR json_extract(s.input,'$.purpose') IS NULL)", id).
			Select("CASE WHEN json_valid(s.output) THEN json_extract(s.output,'$.context_usage.actual_input_tokens') END AS tokens, json_extract(r.model,'$.id') AS model_id").
			Order("s.created_at DESC,s.id DESC").Limit(1).Scan(&actual).Error; err != nil {
			return err
		}
		out.ActualInputTokens, out.ActualModelID = actual.Tokens, actual.ModelID
		return nil
	})
	return out, err
}

func (r *Repository) ContextPage(ctx context.Context, id string, after, before int64) ([]domain.Message, bool, error) {
	return loadContextPage(r.db.WithContext(ctx), id, after, before)
}
