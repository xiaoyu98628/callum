package policy

import (
	"slices"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// Operation 是编排层提供的可信操作事实，不持有设置快照、调用资源或存储句柄。
type Operation struct {
	Effects            []tool.Effect
	OutsideWorkspace   bool
	SandboxMode        tool.SandboxMode
	Command            *tool.CommandBoundary
	PreviouslyRejected bool
	ResultUnconfirmed  bool
}

// EvaluateOperation 按历史决定、待核实事实和实际执行边界确定审批要求。
// 历史拒绝和未知结果优先于普通权限，目录授权不能解除这些约束。
func EvaluateOperation(p ApprovalPolicy, operation Operation) Decision {
	if !p.Valid() {
		return Decision{Deny, "invalid_approval_policy"}
	}
	if operation.PreviouslyRejected {
		return Decision{Deny, "previously_rejected"}
	}
	if operation.ResultUnconfirmed {
		if p == Never {
			return Decision{Deny, "previous_result_requires_confirmation"}
		}
		return Decision{Ask, "previous_result_requires_confirmation"}
	}
	if operation.Command != nil {
		return EvaluateCommand(p, *operation.Command)
	}
	if operation.SandboxMode == tool.SandboxReadOnly && slices.Contains(operation.Effects, tool.EffectWrite) {
		return RequiresApproval(p, "read_only_mode")
	}
	return Evaluate(p, operation.Effects, operation.OutsideWorkspace)
}

// ApplyDirectoryGrant 只解除工作区外普通文件操作的询问，不覆盖其他审批原因。
// matched 必须来自当前会话的实时授权查询；执行认领仍须在事务内复核授权。
func ApplyDirectoryGrant(decision Decision, matched bool) Decision {
	if matched && decision.Action == Ask && decision.Reason == "outside_workspace" {
		return Decision{Allow, "conversation_directory_grant"}
	}
	return decision
}
