```go
package 服务

import (
	上下文包 "context"
	错误判断包 "errors"
	字符串包 "strings"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func Test纠正拒绝已合并生命周期失败集(测试器 *测试包.测试器) {
	无效 := &模型包.工具参数错误{索引: 2, 指纹: "same"}
	for _, 原因 := range []error{上下文包.已取消错误, 上下文包.超出截止时间错误, 智能体包.存储错误, 错误判断包.新建("close response failed")} {
		if _, 成功 := 唯一参数错误(错误判断包.合并(模型包.新建调用错误("decode response", 200, "request", 无效), 原因)); 成功 {
			测试器.格式化报告致命错误("纠正覆盖了其他失败: %v", 原因)
		}
	}
	if 实际值, 成功 := 唯一参数错误(错误判断包.合并(模型包.新建调用错误("decode response", 200, "request", 无效))); !成功 || 实际值 != 无效 {
		测试器.报告致命错误("没有识别单独的工具参数错误")
	}
	超限 := &模型包.工具参数错误{索引: 0, 指纹: "large", 工具: "apply_patch", 实际字节: 16386, 上限字节: 16384}
	for _, 原因 := range []error{上下文包.已取消错误, 上下文包.超出截止时间错误, 智能体包.存储错误, 错误判断包.新建("close response failed")} {
		if _, 成功 := 唯一参数错误(错误判断包.合并(超限, 原因)); 成功 {
			测试器.格式化报告致命错误("超限纠正覆盖其他失败: %v", 原因)
		}
	}
	if _, 成功 := 唯一参数错误(超限); !成功 {
		测试器.报告致命错误("未识别独立参数超限")
	}
	_, 类别, 消息 := 失败(超限)
	if 类别 != "tool_arguments_limit" || !字符串包.包含(消息, "apply_patch") || !字符串包.包含(消息, "16386 > 16384") {
		测试器.格式化报告致命错误("错误未区分参数边界: %s %s", 类别, 消息)
	}
}

func Test已拒绝参数轮次需要无影响集(测试器 *测试包.测试器) {
	调用集 := []模型包.工具调用{{标识: "one", 名称: "run_command", 参数: []byte(`{"command":"[]"}`)}}
	结果 := []模型包.消息{{角色: 模型包.角色工具, 工具调用标识: "one", 内容: `{"operation":{"version":1,"outcome":"not_applied"},"error":{"kind":"invalid_arguments"}}`}}
	指纹 := 已拒绝参数轮次(调用集, 结果)
	if 指纹 == "" {
		测试器.报告致命错误("没有识别执行前拒绝")
	}
	调用集[0].标识 = "new-id"
	结果[0].工具调用标识 = "new-id"
	if 已拒绝参数轮次(调用集, 结果) != 指纹 {
		测试器.报告致命错误("调用标识改变掩盖了重复坏参数")
	}
	for _, 正文 := range []string{
		`{"operation":{"version":1,"outcome":"unknown"},"error":{"kind":"invalid_arguments"}}`,
		`{"operation":{"version":1,"outcome":"applied"},"error":{"kind":"invalid_arguments"}}`,
		`{"operation":{"version":1,"outcome":"not_applied"},"error":{"kind":"permission_denied"}}`,
	} {
		结果[0].内容 = 正文
		if 已拒绝参数轮次(调用集, 结果) != "" {
			测试器.报告致命错误("把已执行或权限错误作为参数纠正")
		}
	}
}
```
