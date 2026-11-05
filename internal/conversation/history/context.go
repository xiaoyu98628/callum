package history

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
)

// Context 在所属用户消息后恢复成对的调用轨迹，包含未完成执行留下的操作事实。
// 普通的失败回答不作为完整回答恢复，已替换分支由仓储过滤。
func Context(messages []conversation.Message, transcripts map[string][]model.Message) ([]model.Message, error) {
	out, err := Assemble(messages, transcripts)
	if err != nil {
		return nil, err
	}
	if err := Check(out); err != nil {
		return nil, err
	}
	return out, nil
}

// Assemble 允许在硬请求上限前准备压缩，但准备窗口也限制为 512 条、16 MiB。
func Assemble(messages []conversation.Message, transcripts map[string][]model.Message) ([]model.Message, error) {
	out := make([]model.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "assistant" && (len(transcripts[message.RunID]) > 0 || message.Status != agent.Completed) {
			continue
		}
		out = append(out, model.Message{Role: model.Role(message.Role), Blocks: message.Content})
		if message.Role == "user" {
			out = append(out, transcripts[message.RunID]...)
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(out) > 512 || len(raw) > 16<<20 {
		return nil, model.ErrInputLimit
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
