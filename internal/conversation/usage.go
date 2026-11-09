package conversation

import (
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/settings"
)

// ContextUsageRequest 只读预览当前有效上下文，不创建消息、调用模型或触发压缩。
type ContextUsageRequest struct {
	ConversationID string  `json:"conversation_id,omitempty"`
	WorkspaceID    *string `json:"workspace_id,omitempty"`
	ModelID        string  `json:"model_id,omitempty"`
	Draft          string  `json:"draft,omitempty"`
}

// ContextUsage 的占用来自服务端估算；实际输入用量独立保存，不以累计计费用量代替。
// Revision 仅在当前 Runtime 内递增；客户端重连后必须重新查询。
type ContextUsage struct {
	ConversationID    string `json:"conversation_id"`
	ModelID           string `json:"model_id"`
	RunID             string `json:"run_id,omitempty"`
	Revision          int64  `json:"revision"`
	UpdatedAt         int64  `json:"updated_at"`
	State             string `json:"state"`
	Reason            string `json:"reason,omitempty"`
	EstimatedTokens   *int64 `json:"estimated_tokens"`
	DraftTokens       int64  `json:"draft_tokens"`
	InputLimit        *int64 `json:"input_limit"`
	ContextWindow     *int64 `json:"context_window"`
	OutputReserve     int64  `json:"output_reserve"`
	SafetyMargin      int64  `json:"safety_margin"`
	EstimationMethod  string `json:"estimation_method"`
	ActualInputTokens *int64 `json:"actual_input_tokens"`
}

// ContextState 固定只读历史的边界；大段正文由 ContextPage 分批读取。
type ContextState struct {
	Conversation      Conversation
	Settings          settings.Snapshot
	Context           *ContextVersion
	ThroughSequence   int64
	LastUserRunID     string
	ActiveRun         *agent.Run
	ActualInputTokens *int64
	ActualModelID     string
}
