package conversation

import (
	"context"
	"errors"
	"slices"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// History 在同一读事务中取得执行与消息，排除重发旧分支，并预检消息总字节。
func (r *Repository) History(ctx context.Context, id, before string, limit int) (out domain.HistoryPage, err error) {
	out.Messages, out.Runs = []domain.Message{}, []agent.Run{}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv schema.Conversation
		if err := tx.Where("id=?", id).Take(&conv).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		messages := tx.Model(&schema.Message{}).Select("1").Where("run_id=conversation_runs.id AND conversation_id=?", id)
		compactions := tx.Model(&schema.Step{}).Select("1").Where("run_id=conversation_runs.id AND sequence=1 AND type='context_compression' AND json_extract(input,'$.purpose')='context_compaction'")
		query := tx.Model(&schema.Run{}).Where("conversation_id=? AND (EXISTS (?) OR EXISTS (?))", id, messages, compactions)
		if before != "" {
			query = query.Where("id<?", before)
		}
		var rows []schema.Run
		if err := query.Order("id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		out.HasMore = len(rows) > limit
		rows = rows[:min(len(rows), limit)]
		var bytes int64
		for _, row := range rows {
			var size int64
			if err := tx.Model(&schema.Message{}).Where("run_id=? AND conversation_id=?", row.ID, id).Select("COALESCE(SUM(length(CAST(content AS BLOB))),0)").Scan(&size).Error; err != nil {
				return err
			}
			if bytes+size > 4<<20 {
				if len(out.Runs) == 0 {
					return model.ErrInputLimit
				}
				out.HasMore = true
				break
			}
			bytes += size
			out.Runs = append(out.Runs, agent.Run{ID: row.ID, ConversationID: row.ConversationID, ReplacesRunID: row.ReplacesRunID, Model: row.Model, Stream: row.Stream, SettingsSnapshot: row.SettingsSnapshot, Status: row.Status, ErrorKind: row.ErrorKind, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt})
			out.BeforeRunID = row.ID
		}
		if len(out.Runs) == 0 {
			return nil
		}
		ids := make([]string, len(out.Runs))
		for i, run := range out.Runs {
			ids[i] = run.ID
		}
		var values []schema.Message
		if err := tx.Where("conversation_id=? AND run_id IN ?", id, ids).Order("sequence").Find(&values).Error; err != nil {
			return err
		}
		for _, value := range values {
			out.Messages = append(out.Messages, message(value))
		}
		slices.Reverse(out.Runs)
		return nil
	})
	return out, err
}
