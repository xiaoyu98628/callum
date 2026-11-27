```go
package 策略

import (
	切片包 "slices"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 操作 是编排层提供的可信操作事实，不持有设置快照、调用资源或存储句柄。
type 操作 struct {
	影响集   []工具包.影响
	外部工作区 bool
	沙箱模式  工具包.沙箱模式
	命令    *工具包.命令边界
	先前已拒绝 bool
	结果未确认 bool
}

// 评估操作 按历史决定、待核实事实和实际执行边界确定审批要求。
// 历史拒绝和未知结果优先于普通权限，目录授权不能解除这些约束。
func 评估操作(当前审批策略 审批策略, 操作 操作) 决定 {
	if !当前审批策略.有效() {
		return 决定{拒绝, "invalid_approval_policy"}
	}
	if 操作.先前已拒绝 {
		return 决定{拒绝, "previously_rejected"}
	}
	if 操作.结果未确认 {
		if 当前审批策略 == 从不 {
			return 决定{拒绝, "previous_result_requires_confirmation"}
		}
		return 决定{询问, "previous_result_requires_confirmation"}
	}
	if 操作.沙箱模式 == 工具包.沙箱完整访问 {
		if 当前审批策略 != 从不 {
			return 决定{拒绝, "invalid_approval_policy"}
		}
		return 决定{允许, "full_access"}
	}
	if 操作.命令 != nil {
		return 评估命令(当前审批策略, *操作.命令)
	}
	if 操作.沙箱模式 == 工具包.沙箱只读 && 切片包.包含(操作.影响集, 工具包.影响写入) {
		return 需要审批(当前审批策略, "read_only_mode")
	}
	return 评估(当前审批策略, 操作.影响集, 操作.外部工作区)
}

// 应用目录授权 只解除工作区外普通文件操作的询问，不覆盖其他审批原因。
// 已匹配 必须来自当前会话的实时授权查询；执行认领仍须在事务内复核授权。
func 应用目录授权(决定 决定, 已匹配 bool) 决定 {
	if 已匹配 && 决定.动作 == 询问 && 决定.原因 == "outside_workspace" {
		return 决定{允许, "conversation_directory_grant"}
	}
	return 决定
}
```
