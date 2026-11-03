package execution

import (
	"maps"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/run"
)

// StepSummary 用于通知和列表，完整 JSON 通过有界分段查询读取，避免大型原始结果阻塞 stdout。
type StepSummary struct {
	run.Step
	OutputBytes     int `json:"output_bytes"`
	UsageBytes      int `json:"usage_bytes"`
	DefinitionBytes int `json:"definition_bytes"`
}

func summary(step run.Step) StepSummary {
	out := StepSummary{Step: step, OutputBytes: len(step.Output), UsageBytes: len(step.Usage), DefinitionBytes: len(step.Definition)}
	out.Output, out.Usage, out.Definition = nil, nil, nil
	return out
}

// EmitPersistent 转换 Go 主动推送的对话通知，发送失败由 notify 取消通信上下文。
func EmitPersistent(notify func(string, any) error, event run.Event) error {
	params := map[string]any{}
	if event.StatePersisted != nil {
		params["persisted"] = *event.StatePersisted
	}
	if len(event.RemovedMessageIDs) > 0 {
		params["removed_message_ids"] = event.RemovedMessageIDs
	}
	if event.ConversationID != "" {
		params["conversation_id"] = event.ConversationID
	}
	if event.RunID != "" {
		params["run_id"] = event.RunID
	}
	if event.StepID != "" {
		params["step_id"] = event.StepID
	}
	if event.MessageID != "" {
		params["message_id"] = event.MessageID
	}
	switch value := event.Data.(type) {
	case conversation.Conversation:
		params["conversation"] = value
	case conversation.Message:
		params["message"] = value
	case run.Step:
		params["step"] = summary(value)
	case run.Run:
		params["run"] = value
		if value.ErrorKind != nil && value.ErrorMessage != nil {
			params["error"] = map[string]any{"kind": *value.ErrorKind, "message": *value.ErrorMessage}
		}
	case map[string]any:
		maps.Copy(params, value)
	}
	return notify(event.Type, params)
}
