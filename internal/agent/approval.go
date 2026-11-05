package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/xiaoyu98628/callum/internal/tool"
)

const AwaitingApproval = "awaiting_approval"
const ApprovalPending = "pending"
const ApprovalApproved = "approved"
const ApprovalRejected = "rejected"
const ApprovalCancelled = "cancelled"

var ErrConflict = errors.New("执行或审批状态冲突")
var ErrApprovalInvalid = errors.New("审批参数无效")

// Approval 是一个确定工具操作的审批记录，Request 由服务端生成，不接受客户端替换。
type Approval struct {
	ID             string               `json:"id"`
	RunID          string               `json:"run_id"`
	StepID         string               `json:"step_id"`
	Request        json.RawMessage      `json:"request"`
	RequestHash    string               `json:"request_hash"`
	Status         string               `json:"status"`
	DecidedBy      *string              `json:"decided_by"`
	DecisionNote   *string              `json:"decision_note"`
	CreatedAt      int64                `json:"created_at"`
	UpdatedAt      int64                `json:"updated_at"`
	DecidedAt      *int64               `json:"decided_at"`
	DirectoryScope *tool.DirectoryScope `json:"directory_scope,omitempty"`
	DecisionScope  string               `json:"decision_scope,omitempty"`
	DirectoryGrant *DirectoryGrant      `json:"directory_grant,omitempty"`
}

type ApprovalFilter struct {
	RunID  string `json:"run_id,omitempty"`
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// ApprovalRepository 的创建与决定均须校验执行和步骤状态，并以事务保存。
type ApprovalRepository interface {
	CreateApproval(context.Context, Approval) (Approval, error)
	GetApproval(context.Context, string) (Approval, error)
	ListApprovals(context.Context, ApprovalFilter) ([]Approval, error)
	DecideApproval(context.Context, string, string, *string, *string, ...string) (Approval, error)
	CancelApproval(context.Context, string) (Approval, error)
	ListDirectoryGrants(context.Context, string, int, int) ([]DirectoryGrant, error)
	RevokeDirectoryGrant(context.Context, string, string) (DirectoryGrant, error)
}

const ApprovalOnce = "once"
const ApprovalConversationDirectory = "conversation_directory"

// DirectoryGrant 保留会话内目录授权的来源和撤销事实，不复制为后续操作的用户审批。
type DirectoryGrant struct {
	ID             string              `json:"id"`
	ConversationID string              `json:"conversation_id"`
	ApprovalID     string              `json:"approval_id"`
	Scope          tool.DirectoryScope `json:"scope"`
	CreatedAt      int64               `json:"created_at"`
	RevokedAt      *int64              `json:"revoked_at"`
}
