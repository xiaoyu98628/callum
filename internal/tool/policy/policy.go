// Package policy 根据可信操作类别判断是否需要审批，不实现操作系统沙箱。
package policy

import (
	"slices"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// ApprovalPolicy 只控制是否询问，不扩大工具可访问的工作区或资源。
type ApprovalPolicy string

const (
	OnRequest ApprovalPolicy = "on-request"
	Always    ApprovalPolicy = "always"
	Never     ApprovalPolicy = "never"
)

// Valid 用于在启动或装配时拒绝未支持的审批策略。
func (p ApprovalPolicy) Valid() bool { return p == OnRequest || p == Always || p == Never }

// Reviewer 独立指定审批人，当前只有本机用户审批可用。
type Reviewer string

const User Reviewer = "user"

// Valid 不接受尚未实现的自动审查，避免静默切换审批人。
func (r Reviewer) Valid() bool { return r == User }

// Action 是执行前的权限判定，不代表用户已经作出审批决定。
type Action string

const (
	Allow Action = "allow"
	Ask   Action = "ask"
	Deny  Action = "deny"
)

// Decision 保存判定及其规则标识，供步骤审计使用。
type Decision struct {
	Action Action `json:"action"`
	Reason string `json:"reason"`
}

// Evaluate 只处理已经完成参数和目标校验的操作；纯读取不受工作区写入边界限制。
// 工作区外普通修改需要审批或独立核验的会话目录授权，不以只读类别抵消其他影响。
// Effects 必须来自可信注册代码，不能使用模型提供的类别或风险描述。
func Evaluate(p ApprovalPolicy, effects []tool.Effect, outsideWorkspace bool) Decision {
	if !p.Valid() {
		return Decision{Deny, "invalid_approval_policy"}
	}
	for _, effect := range effects {
		if !effect.Valid() {
			return Decision{Deny, "invalid_operation_effect"}
		}
	}
	decision := classify(effects)
	if outsideWorkspace && decision.Action == Allow && slices.Contains(effects, tool.EffectWrite) {
		decision = Decision{Ask, "outside_workspace"}
	}
	switch p {
	case Always:
		return Decision{Ask, "policy_always"}
	case Never:
		if decision.Action == Ask {
			return Decision{Deny, "approval_required_but_disabled"}
		}
	}
	return decision
}

func classify(effects []tool.Effect) Decision {
	if len(effects) == 0 {
		return Decision{Ask, "unclassified_operation"}
	}
	readOnly := true
	for _, effect := range effects {
		switch effect {
		case tool.EffectWrite:
			readOnly = false
		case tool.EffectDestructive:
			return Decision{Ask, "destructive_operation"}
		case tool.EffectExternal:
			return Decision{Ask, "external_operation"}
		case tool.EffectUnknown:
			return Decision{Ask, "unclassified_operation"}
		}
	}
	if readOnly {
		return Decision{Allow, "read_only_operation"}
	}
	return Decision{Allow, "workspace_read_write_policy"}
}

// RequiresApproval 保持 always/never 的询问语义，不将拒绝伪装为用户决定。
func RequiresApproval(p ApprovalPolicy, reason string) Decision {
	if !p.Valid() {
		return Decision{Deny, "invalid_approval_policy"}
	}
	if p == Never {
		return Decision{Deny, "approval_required_but_disabled"}
	}
	if p == Always {
		return Decision{Ask, "policy_always"}
	}
	return Decision{Ask, reason}
}

// EvaluateCommand 允许受系统边界约束的常规命令，不降低命令本身的 unknown 影响类别。
func EvaluateCommand(p ApprovalPolicy, boundary tool.CommandBoundary) Decision {
	if !p.Valid() {
		return Decision{Deny, "invalid_approval_policy"}
	}
	if boundary.Escalated {
		return RequiresApproval(p, "sandbox_escalation")
	}
	if boundary.Risk != "" {
		return RequiresApproval(p, boundary.Risk)
	}
	if boundary.Sandboxed && boundary.Mode.Valid() {
		if p == Always {
			return Decision{Ask, "policy_always"}
		}
		return Decision{Allow, "sandboxed_command"}
	}
	return RequiresApproval(p, "unsandboxed_command")
}
