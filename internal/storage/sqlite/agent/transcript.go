package agent

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// Transcript 只读取当前消息分支的成功执行；查询前检查累计大小，不加载大型原始模型结果。
func (r *Repository) Transcript(ctx context.Context, id string) (map[string][]model.Message, error) {
	query := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&schema.Step{}).
			Joins("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id AND r.status='completed'").
			Joins("JOIN conversation_messages m ON m.run_id=r.id AND m.role='user'").
			Where("conversation_run_steps.conversation_id=? AND json_type(conversation_run_steps.output,'$.context_message')='object'", id)
	}
	var bounds struct {
		Size  int64
		Count int64
	}
	if err := query().Select("COALESCE(SUM(length(CAST(json_extract(conversation_run_steps.output,'$.context_message') AS BLOB))),0) AS size,COUNT(*) AS count").Scan(&bounds).Error; err != nil {
		return nil, err
	}
	if bounds.Size > 8<<20 || bounds.Count > 256 {
		return nil, model.ErrInputLimit
	}
	var rows []struct {
		RunID   string
		Message string
	}
	if err := query().Select("conversation_run_steps.run_id,json_extract(conversation_run_steps.output,'$.context_message') AS message").Order("r.created_at").Order("r.id").Order("conversation_run_steps.sequence").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string][]model.Message{}
	for _, row := range rows {
		var message model.Message
		if json.Unmarshal([]byte(row.Message), &message) != nil {
			return nil, model.ErrInvalidResponse
		}
		out[row.RunID] = append(out[row.RunID], message)
	}
	return out, nil
}
