package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// Begin 在同一写事务中接受输入并建立执行；事务返回后才可调用模型。
func (r *Repository) Begin(ctx context.Context, in domain.Begin) (out domain.Execution, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 与设置更新按同一数据库事务顺序确定配置；读失败时不创建会话或回退权限。
		snapshot, err := executionSettings(tx, ctx)
		if err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		var conv schema.Conversation
		err = tx.Where("id = ?", in.ConversationID).Take(&conv).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !in.Create || in.ResendMessageID != "" {
				return domain.ErrNotFound
			}
			if err := activeWorkspace(tx, in.WorkspaceID); err != nil {
				return err
			}
			conv = schema.Conversation{ID: in.ConversationID, WorkspaceID: in.WorkspaceID, Title: history.Title(in.Content), TitleSource: "default", Model: in.Model, CreatedAt: now, UpdatedAt: now}
			if err := tx.Omit("Workspace").Create(&conv).Error; err != nil {
				return err
			}
			out.Created = true
		} else if err != nil {
			return err
		} else {
			if in.Create {
				return domain.ErrConflict
			}
			if in.WorkspaceID != nil {
				return domain.ErrInvalid
			}
			if conv.ArchivedAt != nil || conv.DeletedAt != nil {
				return domain.ErrInactive
			}
			if err := activeWorkspace(tx, conv.WorkspaceID); err != nil {
				return err
			}
		}
		if err := busy(tx, conv.ID); err != nil {
			return err
		}
		var user schema.Message
		var replaced *string
		if in.ResendMessageID != "" {
			if err := tx.Where("id = ? AND conversation_id = ? AND role = 'user'", in.ResendMessageID, conv.ID).Take(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return domain.ErrNotFound
				}
				return err
			}
			var latest int64
			if err := tx.Model(&schema.Message{}).Where("conversation_id = ? AND role = 'user'", conv.ID).Select("COALESCE(MAX(sequence),0)").Scan(&latest).Error; err != nil {
				return err
			}
			if user.Sequence != latest || (in.ResendRunID != "" && user.RunID != in.ResendRunID) {
				return domain.ErrConflict
			}
			if err := invalidateContexts(tx, conv.ID, user.Sequence, now); err != nil {
				return err
			}
			old := user.RunID
			replaced = &old
			if err := tx.Model(&schema.Message{}).Where("conversation_id = ? AND sequence > ?", conv.ID, user.Sequence).Pluck("id", &out.RemovedMessageIDs).Error; err != nil {
				return err
			}
			if err := tx.Where("conversation_id = ? AND sequence > ?", conv.ID, user.Sequence).Delete(&schema.Message{}).Error; err != nil {
				return err
			}
			// 清理被替换回答及标题的模型副本；仍有效的历史摘要、实际工具结果和 usage 保留。
			if err := tx.Model(&schema.Step{}).Where("run_id = ? AND type = 'model_call' AND COALESCE(json_extract(input,'$.purpose'),'')<>'context_summary'", old).Update("output", nil).Error; err != nil {
				return err
			}
			if user.Sequence == 1 && conv.TitleSource == "default" {
				conv.Title = history.Title(in.Content)
			}
		} else {
			var latest int64
			if err := tx.Model(&schema.Message{}).Where("conversation_id = ?", conv.ID).Select("COALESCE(MAX(sequence),0)").Scan(&latest).Error; err != nil {
				return err
			}
			user = schema.Message{ID: in.UserMessageID, ConversationID: conv.ID, Sequence: latest + 1, Role: "user", CreatedAt: now}
		}
		past, current, more, err := loadContextHistory(tx, conv.ID, user.Sequence)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(past)
		if err != nil {
			return err
		}
		incoming, err := json.Marshal(in.Content)
		if err != nil {
			return err
		}
		if len(encoded)+len(incoming) > 16<<20 {
			return agent.ErrOutputLimit
		}
		out.Context, out.HistoryMore = current, more
		runRow := schema.Run{ID: in.RunID, ConversationID: conv.ID, ReplacesRunID: replaced, Model: in.Model, Stream: in.Stream, SettingsSnapshot: &snapshot, Status: agent.Running, CreatedAt: now, UpdatedAt: now}
		if err := tx.Omit("Conversation", "Replaces").Create(&runRow).Error; err != nil {
			return err
		}
		user.RunID, user.Content, user.Status, user.UpdatedAt, user.CompletedAt = in.RunID, in.Content, agent.Completed, now, &now
		if in.ResendMessageID != "" {
			if err := tx.Omit("Run").Select("RunID", "Content", "Status", "UpdatedAt", "CompletedAt").Updates(&user).Error; err != nil {
				return err
			}
		} else if err := tx.Omit("Run").Create(&user).Error; err != nil {
			return err
		}
		assistant := schema.Message{ID: in.AssistantMessageID, ConversationID: conv.ID, RunID: in.RunID, Sequence: user.Sequence + 1, Role: "assistant", Content: []model.Block{}, Status: agent.Running, CreatedAt: now, UpdatedAt: now}
		if err := tx.Omit("Run").Create(&assistant).Error; err != nil {
			return err
		}
		out.History = past
		sources := make([]map[string]any, 0, len(past)+1)
		for _, row := range past {
			sources = append(sources, map[string]any{"id": row.ID, "fingerprint": history.Fingerprint(row.Content)})
		}
		out.History = append(out.History, message(user))
		sources = append(sources, map[string]any{"id": user.ID, "fingerprint": history.Fingerprint(user.Content)})
		input, err := json.Marshal(map[string]any{"protocol": in.Protocol, "messages": sources})
		if err != nil {
			return err
		}
		step := schema.Step{ID: in.StepID, ConversationID: conv.ID, RunID: in.RunID, Sequence: 1, Type: "model_call", Name: "生成回答", Status: agent.Pending, Input: input, Model: &in.Model, CreatedAt: now, UpdatedAt: now}
		if err := tx.Omit("Run", "Parent").Create(&step).Error; err != nil {
			return err
		}
		conv.Model, conv.UpdatedAt = in.Model, now
		if err := tx.Omit("Workspace").Select("Model", "Title", "UpdatedAt").Updates(&conv).Error; err != nil {
			return err
		}
		out.Conversation, out.User, out.Assistant = convert(conv), message(user), message(assistant)
		out.Run = agent.Run{ID: in.RunID, ConversationID: conv.ID, ReplacesRunID: replaced, Model: in.Model, Stream: in.Stream, SettingsSnapshot: &snapshot, Status: agent.Running, CreatedAt: now, UpdatedAt: now}
		out.Step = agent.Step{ID: step.ID, ConversationID: conv.ID, RunID: in.RunID, Sequence: 1, Type: step.Type, Name: step.Name, Status: step.Status, Input: input, Model: &in.Model, CreatedAt: now, UpdatedAt: now}
		return nil
	})
	return out, err
}

func (r *Repository) ApplyTitle(ctx context.Context, id, userID, fingerprint, title string) (out domain.Conversation, applied bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Conversation
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			return err
		}
		if row.TitleSource != "default" {
			out = convert(row)
			return nil
		}
		var first schema.Message
		if err := tx.Where("id = ? AND conversation_id = ? AND sequence = 1 AND role = 'user'", userID, id).Take(&first).Error; err != nil {
			return err
		}
		if history.Fingerprint(first.Content) != fingerprint {
			out = convert(row)
			return nil
		}
		now := time.Now().UnixMilli()
		if err := tx.Model(&row).Updates(map[string]any{"title": title, "title_source": "generated", "updated_at": now}).Error; err != nil {
			return err
		}
		row.Title, row.TitleSource, row.UpdatedAt = title, "generated", now
		out = convert(row)
		applied = true
		return nil
	})
	return out, applied, err
}
