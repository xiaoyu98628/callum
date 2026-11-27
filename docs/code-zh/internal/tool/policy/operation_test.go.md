```go
package 策略测试

import (
	测试包 "testing"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

func Test操作决定优先级并且目录授权(测试器 *测试包.测试器) {
	沙箱内 := &工具包.命令边界{沙箱内: true, 模式: 工具包.沙箱只读}
	for _, 测试用例 := range []struct {
		名称  string
		策略  策略包.审批策略
		操作  策略包.操作
		预期  策略包.决定
		已授权 策略包.决定
	}{
		{"rejected_command", 策略包.开启请求, 策略包.操作{命令: 沙箱内, 先前已拒绝: true, 结果未确认: true}, 策略包.决定{动作: 策略包.拒绝, 原因: "previously_rejected"}, 策略包.决定{动作: 策略包.拒绝, 原因: "previously_rejected"}},
		{"unconfirmed_command", 策略包.开启请求, 策略包.操作{命令: 沙箱内, 结果未确认: true}, 策略包.决定{动作: 策略包.询问, 原因: "previous_result_requires_confirmation"}, 策略包.决定{动作: 策略包.询问, 原因: "previous_result_requires_confirmation"}},
		{"unconfirmed_never", 策略包.从不, 策略包.操作{结果未确认: true}, 策略包.决定{动作: 策略包.拒绝, 原因: "previous_result_requires_confirmation"}, 策略包.决定{动作: 策略包.拒绝, 原因: "previous_result_requires_confirmation"}},
		{"unconfirmed_always", 策略包.总是, 策略包.操作{结果未确认: true}, 策略包.决定{动作: 策略包.询问, 原因: "previous_result_requires_confirmation"}, 策略包.决定{动作: 策略包.询问, 原因: "previous_result_requires_confirmation"}},
		{"read_only_write", 策略包.开启请求, 策略包.操作{影响集: []工具包.影响{工具包.影响写入}, 沙箱模式: 工具包.沙箱只读, 外部工作区: true}, 策略包.决定{动作: 策略包.询问, 原因: "read_only_mode"}, 策略包.决定{动作: 策略包.询问, 原因: "read_only_mode"}},
		{"outside_write", 策略包.开启请求, 策略包.操作{影响集: []工具包.影响{工具包.影响写入}, 外部工作区: true}, 策略包.决定{动作: 策略包.询问, 原因: "outside_workspace"}, 策略包.决定{动作: 策略包.允许, 原因: "conversation_directory_grant"}},
		{"always_write", 策略包.总是, 策略包.操作{影响集: []工具包.影响{工具包.影响写入}, 外部工作区: true}, 策略包.决定{动作: 策略包.询问, 原因: "policy_always"}, 策略包.决定{动作: 策略包.询问, 原因: "policy_always"}},
		{"never_write", 策略包.从不, 策略包.操作{影响集: []工具包.影响{工具包.影响写入}, 外部工作区: true}, 策略包.决定{动作: 策略包.拒绝, 原因: "approval_required_but_disabled"}, 策略包.决定{动作: 策略包.拒绝, 原因: "approval_required_but_disabled"}},
		{"sandboxed_command", 策略包.开启请求, 策略包.操作{影响集: []工具包.影响{工具包.影响未知}, 命令: 沙箱内}, 策略包.决定{动作: 策略包.允许, 原因: "sandboxed_command"}, 策略包.决定{动作: 策略包.允许, 原因: "sandboxed_command"}},
		{"escalated_command", 策略包.开启请求, 策略包.操作{命令: &工具包.命令边界{提权: true}}, 策略包.决定{动作: 策略包.询问, 原因: "sandbox_escalation"}, 策略包.决定{动作: 策略包.询问, 原因: "sandbox_escalation"}},
		{"unknown_effect", 策略包.开启请求, 策略包.操作{影响集: []工具包.影响{工具包.影响未知}, 外部工作区: true}, 策略包.决定{动作: 策略包.询问, 原因: "unclassified_operation"}, 策略包.决定{动作: 策略包.询问, 原因: "unclassified_operation"}},
		{"invalid_policy", "invalid", 策略包.操作{命令: 沙箱内}, 策略包.决定{动作: 策略包.拒绝, 原因: "invalid_approval_policy"}, 策略包.决定{动作: 策略包.拒绝, 原因: "invalid_approval_policy"}},
	} {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			决定 := 策略包.评估操作(测试用例.策略, 测试用例.操作)
			if 决定 != 测试用例.预期 {
				测试器.格式化报告致命错误("判定错误: got=%+v want=%+v", 决定, 测试用例.预期)
			}
			if 实际 := 策略包.应用目录授权(决定, false); 实际 != 决定 {
				测试器.格式化报告致命错误("未匹配授权却改变判定: %+v", 实际)
			}
			if 实际 := 策略包.应用目录授权(决定, true); 实际 != 测试用例.已授权 {
				测试器.格式化报告致命错误("授权覆盖了不应解除的约束: got=%+v want=%+v", 实际, 测试用例.已授权)
			}
		})
	}
}
```
