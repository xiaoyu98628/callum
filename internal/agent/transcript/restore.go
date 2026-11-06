package transcript

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/tool"
)

// Restore 恢复单次执行的有序轨迹，过滤未完成回答并补齐未执行的调用结果。
// 输入记录必须属于同一执行；原始数据只读，返回值不持有数据库或执行资源。
func Restore(status string, rows []Entry) ([]model.Message, error) {
	if len(rows) == 0 {
		return []model.Message{}, nil
	}
	decoded := make([]model.Message, len(rows))
	results := map[string]map[string]int{}
	for i, row := range rows {
		if json.Unmarshal(row.Message, &decoded[i]) != nil {
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
	if status == "completed" {
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
			if index, ok := results[row.StepID][call.ID]; ok {
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
		if json.Unmarshal(row.Operation, &facts) != nil || !facts.Valid() || (facts.Outcome.MayHaveEffects() && !used[i]) {
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
