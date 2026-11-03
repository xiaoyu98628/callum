package history

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
)

// Context 保留用户、模型调用及工具结果的完整顺序，成功执行的轨迹替代单条最终回答。
func Context(messages []conversation.Message, transcripts map[string][]model.Message) ([]model.Message, error) {
	out := make([]model.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "assistant" && len(transcripts[message.RunID]) > 0 {
			out = append(out, transcripts[message.RunID]...)
		} else {
			out = append(out, model.Message{Role: model.Role(message.Role), Blocks: message.Content})
		}
	}
	if err := Check(out); err != nil {
		return nil, err
	}
	return out, nil
}

// Check 限制消息数量和实际序列化大小，不估算 Token。
func Check(messages []model.Message) error {
	if len(messages) > 256 {
		return model.ErrInputLimit
	}
	value, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	if len(value) > MaxBytes {
		return model.ErrInputLimit
	}
	return nil
}
