```go
// 包 策略 根据可信操作类别判断是否需要审批，不实现操作系统沙箱。
package 策略

import (
	切片包 "slices"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 审批策略 只控制是否询问，不扩大工具可访问的工作区或资源。
type 审批策略 string

const (
	开启请求 审批策略 = "on-request"
	总是    审批策略 = "always"
	从不     审批策略 = "never"
)

// 有效 用于在启动或装配时拒绝未支持的审批策略。
func (当前审批策略 审批策略) 有效() bool { return 当前审批策略 == 开启请求 || 当前审批策略 == 总是 || 当前审批策略 == 从不 }

// 审核方 独立指定审批请求由本机用户或独立的模型调用处理。
type 审核方 string

const (
	用户       审核方 = "user"
	自动审核 审核方 = "auto_review"
)

func (当前审核方 审核方) 有效() bool { return 当前审核方 == 用户 || 当前审核方 == 自动审核 }

// 动作 是执行前的权限判定，不代表用户已经作出审批决定。
type 动作 string

const (
	允许 动作 = "allow"
	询问   动作 = "ask"
	拒绝  动作 = "deny"
)

// 决定 保存判定及其规则标识，供步骤审计使用。
type 决定 struct {
	动作 动作 `json:"action"`
	原因 string `json:"reason"`
}

// 评估 只处理已经完成参数和目标校验的操作；纯读取不受工作区写入边界限制。
// 工作区外普通修改需要审批或独立核验的会话目录授权，不以只读类别抵消其他影响。
// 影响集 必须来自可信注册代码，不能使用模型提供的类别或风险描述。
func 评估(当前审批策略 审批策略, 影响集 []工具包.影响, 外部工作区 bool) 决定 {
	if !当前审批策略.有效() {
		return 决定{拒绝, "invalid_approval_policy"}
	}
	for _, 影响 := range 影响集 {
		if !影响.有效() {
			return 决定{拒绝, "invalid_operation_effect"}
		}
	}
	决定 := 分类(影响集)
	if 外部工作区 && 决定.动作 == 允许 && 切片包.包含(影响集, 工具包.影响写入) {
		决定 = 决定{询问, "outside_workspace"}
	}
	switch 当前审批策略 {
	case 总是:
		return 决定{询问, "policy_always"}
	case 从不:
		if 决定.动作 == 询问 {
			return 决定{拒绝, "approval_required_but_disabled"}
		}
	}
	return 决定
}

func 分类(影响集 []工具包.影响) 决定 {
	if len(影响集) == 0 {
		return 决定{询问, "unclassified_operation"}
	}
	读取仅 := true
	for _, 影响 := range 影响集 {
		switch 影响 {
		case 工具包.影响写入:
			读取仅 = false
		case 工具包.影响破坏性:
			return 决定{询问, "destructive_operation"}
		case 工具包.影响外部:
			return 决定{询问, "external_operation"}
		case 工具包.影响未知:
			return 决定{询问, "unclassified_operation"}
		}
	}
	if 读取仅 {
		return 决定{允许, "read_only_operation"}
	}
	return 决定{允许, "workspace_read_write_policy"}
}

// 需要审批 保持 总是/从不 的询问语义，不将拒绝伪装为用户决定。
func 需要审批(当前审批策略 审批策略, 原因 string) 决定 {
	if !当前审批策略.有效() {
		return 决定{拒绝, "invalid_approval_policy"}
	}
	if 当前审批策略 == 从不 {
		return 决定{拒绝, "approval_required_but_disabled"}
	}
	if 当前审批策略 == 总是 {
		return 决定{询问, "policy_always"}
	}
	return 决定{询问, 原因}
}

// 评估命令 允许受系统边界约束的常规命令，不降低命令本身的 未知 影响类别。
func 评估命令(当前审批策略 审批策略, 边界 工具包.命令边界) 决定 {
	if !当前审批策略.有效() {
		return 决定{拒绝, "invalid_approval_policy"}
	}
	if 边界.提权 {
		return 需要审批(当前审批策略, "sandbox_escalation")
	}
	if 边界.风险 != "" {
		return 需要审批(当前审批策略, 边界.风险)
	}
	if 边界.沙箱内 && 边界.模式 == 工具包.沙箱工作区写入 {
		return 需要审批(当前审批策略, "sandbox_workspace_write")
	}
	if 边界.沙箱内 && 边界.模式 == 工具包.沙箱只读 {
		if 当前审批策略 == 总是 {
			return 决定{询问, "policy_always"}
		}
		return 决定{允许, "sandboxed_command"}
	}
	return 需要审批(当前审批策略, "unsandboxed_command")
}
```
