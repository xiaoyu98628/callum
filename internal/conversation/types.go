package conversation

import (
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/run"
)

// Conversation 是会话展示信息，首次发送时与消息、执行一起创建。
type Conversation struct {
	ID          string         `json:"id"`
	WorkspaceID *string        `json:"workspace_id"`
	Title       string         `json:"title"`
	TitleSource string         `json:"title_source"`
	Model       model.Snapshot `json:"model"`
	PinnedAt    *int64         `json:"pinned_at"`
	ArchivedAt  *int64         `json:"archived_at"`
	DeletedAt   *int64         `json:"deleted_at"`
	CreatedAt   int64          `json:"created_at"`
	UpdatedAt   int64          `json:"updated_at"`
}

// Message 仅保存用户消息及回答，工具、审批、补充参数保存在执行步骤。
type Message struct {
	ID             string        `json:"id"`
	ConversationID string        `json:"conversation_id"`
	RunID          string        `json:"run_id"`
	Sequence       int64         `json:"sequence"`
	Role           string        `json:"role"`
	Content        []model.Block `json:"content"`
	Status         string        `json:"status"`
	CreatedAt      int64         `json:"created_at"`
	UpdatedAt      int64         `json:"updated_at"`
	CompletedAt    *int64        `json:"completed_at"`
}

type Change struct {
	Title           *string         `json:"title,omitempty"`
	Model           *model.Snapshot `json:"-"`
	WorkspaceID     *string         `json:"workspace_id,omitempty"`
	DetachWorkspace bool            `json:"detach_workspace,omitempty"`
	Pinned          *bool           `json:"pinned,omitempty"`
	Archived        *bool           `json:"archived,omitempty"`
	Deleted         *bool           `json:"-"`
}

type Filter struct {
	WorkspaceID *string `json:"workspace_id,omitempty"`
	Unbound     bool    `json:"unbound,omitempty"`
	Archived    bool    `json:"archived"`
	Deleted     bool    `json:"deleted"`
	Limit       int     `json:"limit"`
	Offset      int     `json:"offset"`
}

// Begin 是一次发送或编辑重发的原子写入输入，所有 ID 由业务层生成。
type Begin struct {
	Create             bool
	ConversationID     string
	WorkspaceID        *string
	UserMessageID      string
	AssistantMessageID string
	RunID              string
	StepID             string
	ResendMessageID    string
	Model              model.Snapshot
	Protocol           string
	Content            []model.Block
	Stream             bool
}

// Execution 是提交后的执行快照，不持有数据库连接或事务。
type Execution struct {
	RemovedMessageIDs []string
	Created           bool
	Conversation      Conversation
	User              Message
	Assistant         Message
	Run               run.Run
	Step              run.Step
	History           []Message
}

// Send 是发送或编辑重发的输入。首次提问不提供 ConversationID；ModelID 可在已有会话省略。
type Send struct {
	ConversationID string        `json:"conversation_id,omitempty"`
	WorkspaceID    *string       `json:"workspace_id,omitempty"`
	ModelID        string        `json:"model_id,omitempty"`
	MessageID      string        `json:"message_id,omitempty"`
	Content        []model.Block `json:"content"`
	Stream         *bool         `json:"stream,omitempty"`
}
