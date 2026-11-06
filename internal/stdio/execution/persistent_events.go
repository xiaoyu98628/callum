package execution

import (
	"maps"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// StepSummary 保持步骤查询与通知使用相同的元数据结构。
type StepSummary = agent.StepSummary

func summary(step agent.Step) StepSummary { return agent.SummarizeStep(step) }

// EmitPersistent 在 stdio 边界将业务事件编码为兼容的 JSON-RPC 通知。
func EmitPersistent(notify func(string, any) error, event agent.Event) error {
	if data, ok := event.Data.(inference.Notification); ok {
		params := struct {
			RunID             string          `json:"run_id"`
			Text              *string         `json:"text,omitempty"`
			ProviderRequestID string          `json:"provider_request_id,omitempty"`
			Error             *protocol.Error `json:"error,omitempty"`
		}{RunID: event.RunID, Text: data.Text, ProviderRequestID: data.ProviderRequestID}
		if data.Failure != nil {
			params.Error = protocolError(*data.Failure)
		}
		return notify(event.Type, params)
	}
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
	case agent.Step:
		params["step"] = summary(value)
	case agent.StepSummary:
		params["step"] = value
	case agent.ToolOutput:
		params["output"] = value
	case agent.UserInput:
		params["input_id"] = value.ID
		params["user_input"] = value
	case agent.Approval:
		params["approval_id"] = value.ID
		params["approval"] = value
	case agent.DirectoryGrant:
		params["directory_grant"] = value
	case agent.Run:
		params["run"] = value
		if value.ErrorKind != nil && value.ErrorMessage != nil {
			params["error"] = map[string]any{"kind": *value.ErrorKind, "message": *value.ErrorMessage}
		}
	case map[string]any:
		maps.Copy(params, value)
	}
	return notify(event.Type, params)
}
