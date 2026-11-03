package agent

import (
	"context"
	"encoding/json"
	"errors"
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
	ID           string          `json:"id"`
	RunID        string          `json:"run_id"`
	StepID       string          `json:"step_id"`
	Request      json.RawMessage `json:"request"`
	RequestHash  string          `json:"request_hash"`
	Status       string          `json:"status"`
	DecidedBy    *string         `json:"decided_by"`
	DecisionNote *string         `json:"decision_note"`
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`
	DecidedAt    *int64          `json:"decided_at"`
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
	DecideApproval(context.Context, string, string, *string, *string) (Approval, error)
	CancelApproval(context.Context, string) (Approval, error)
}
