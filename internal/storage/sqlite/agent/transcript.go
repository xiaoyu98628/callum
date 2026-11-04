package agent

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

type transcriptRow struct {
	ID, RunID, RunStatus     string
	ParentStepID, ToolCallID *string
	Message                  string
	Operation                *string
}

// Transcript 在当前消息分支恢复成功轨迹及失败执行留下的副作用事实。
// 查询前检查累计大小，只加载统一上下文，不加载大型原始模型结果。
func (r *Repository) Transcript(ctx context.Context, id string) (map[string][]model.Message, error) {
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&schema.Step{}).
			Joins("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id AND r.status IN ('completed','failed','cancelled','interrupted')").
			Joins("JOIN conversation_messages m ON m.run_id=r.id AND m.role='user'").
			Where("conversation_run_steps.conversation_id=?", id)
	}
	// 新步骤缺失事实属于损坏记录，不能沿用旧契约而静默丢失操作。
	var invalid int64
	if err := base().Where(`conversation_run_steps.type='tool_call'
		AND json_extract(conversation_run_steps.input,'$.operation_version')=1 AND (
			COALESCE(json_extract(conversation_run_steps.output,'$.operation.version'),0)<>1
			OR COALESCE(json_extract(conversation_run_steps.output,'$.operation.outcome'),'') NOT IN ('none','not_applied','applied','partially_applied','unknown')
			OR COALESCE(json_type(conversation_run_steps.output,'$.context_message'),'')<>'object'
		)`).Count(&invalid).Error; err != nil {
		return nil, err
	}
	if invalid != 0 {
		return nil, model.ErrInvalidResponse
	}
	query := func() *gorm.DB {
		return base().Where("json_type(conversation_run_steps.output,'$.context_message')='object'").
			Where(`r.status='completed' OR EXISTS (
				SELECT 1 FROM conversation_run_steps t WHERE t.run_id=r.id AND t.type='tool_call'
				AND json_extract(t.input,'$.operation_version')=1
				AND json_extract(t.output,'$.operation.version')=1
				AND json_extract(t.output,'$.operation.outcome') IN ('applied','partially_applied','unknown')
			)`)
	}
	var bounds struct{ Size, Count int64 }
	if err := query().Select("COALESCE(SUM(length(CAST(json_extract(conversation_run_steps.output,'$.context_message') AS BLOB))),0) AS size,COUNT(*) AS count").Scan(&bounds).Error; err != nil {
		return nil, err
	}
	if bounds.Size > 8<<20 || bounds.Count > 256 {
		return nil, model.ErrInputLimit
	}
	var rows []transcriptRow
	if err := query().Select("conversation_run_steps.id,conversation_run_steps.run_id,r.status AS run_status,conversation_run_steps.parent_step_id,conversation_run_steps.tool_call_id,json_extract(conversation_run_steps.output,'$.context_message') AS message,json_extract(conversation_run_steps.output,'$.operation') AS operation").
		Order("r.created_at").Order("r.id").Order("conversation_run_steps.sequence").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string][]model.Message{}
	count := 0
	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].RunID == rows[start].RunID {
			end++
		}
		messages, err := runTranscript(rows[start:end])
		if err != nil {
			return nil, err
		}
		count += len(messages)
		if count > 256 {
			return nil, model.ErrInputLimit
		}
		out[rows[start].RunID] = messages
		start = end
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 8<<20 {
		return nil, model.ErrInputLimit
	}
	return out, nil
}

func runTranscript(rows []transcriptRow) ([]model.Message, error) {
	decoded := make([]model.Message, len(rows))
	results := map[string]map[string]int{}
	for i, row := range rows {
		if json.Unmarshal([]byte(row.Message), &decoded[i]) != nil {
			return nil, model.ErrInvalidResponse
		}
		m := decoded[i]
		if m.Role == model.RoleTool {
			if row.ParentStepID == nil || row.ToolCallID == nil || m.ToolCallID != *row.ToolCallID {
				return nil, model.ErrInvalidResponse
			}
			if results[*row.ParentStepID] == nil {
				results[*row.ParentStepID] = map[string]int{}
			}
			if _, exists := results[*row.ParentStepID][m.ToolCallID]; exists {
				return nil, model.ErrInvalidResponse
			}
			results[*row.ParentStepID][m.ToolCallID] = i
		}
	}
	if rows[0].RunStatus == "completed" {
		return decoded, checkTranscript(decoded)
	}
	out := []model.Message{}
	used := map[int]bool{}
	for i, row := range rows {
		message := decoded[i]
		if message.Role != model.RoleAssistant || len(message.ToolCalls) == 0 {
			continue
		}
		// 未成功完成的回答正文不进入后续上下文，只恢复调用及事实。
		message.Content, message.Blocks = "", nil
		out = append(out, message)
		for _, call := range message.ToolCalls {
			if index, ok := results[row.ID][call.ID]; ok {
				out = append(out, decoded[index])
				used[index] = true
			} else {
				// 同轮剩余调用尚未建立步骤，未进入执行边界，补齐未执行结果。
				body, err := json.Marshal(map[string]any{"operation": tool.OutcomeRecord{Version: tool.OperationVersion, Outcome: tool.OutcomeNotApplied, Recovered: true}, "result": nil,
					"error": map[string]string{"kind": "not_executed", "message": "执行已结束，该调用尚未执行"}})
				if err != nil {
					return nil, err
				}
				out = append(out, model.Message{Role: model.RoleTool, ToolCallID: call.ID, Content: string(body)})
			}
		}
	}
	for i, row := range rows {
		if row.Operation == nil {
			continue
		}
		var facts tool.OutcomeRecord
		if json.Unmarshal([]byte(*row.Operation), &facts) != nil || !facts.Valid() || (facts.Outcome.MayHaveEffects() && !used[i]) {
			return nil, model.ErrInvalidResponse
		}
	}
	return out, checkTranscript(out)
}

func checkTranscript(messages []model.Message) error {
	pending := map[string]bool{}
	for _, message := range messages {
		switch message.Role {
		case model.RoleAssistant:
			if len(pending) != 0 {
				return model.ErrInvalidResponse
			}
			for _, call := range message.ToolCalls {
				if call.ID == "" || pending[call.ID] {
					return model.ErrInvalidResponse
				}
				pending[call.ID] = true
			}
		case model.RoleTool:
			if !pending[message.ToolCallID] {
				return model.ErrInvalidResponse
			}
			delete(pending, message.ToolCallID)
		default:
			return model.ErrInvalidResponse
		}
	}
	if len(pending) != 0 {
		return model.ErrInvalidResponse
	}
	return nil
}
