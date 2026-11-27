```go
package 执行测试

import (
	JSON包 "encoding/json"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	执行包 "github.com/xiaoyu98628/callum/internal/stdio/execution"
)

func Test工具输出保留执行身份(测试器 *测试包.测试器) {
	值 := 智能体包.工具输出{序号: 1, 流: "stderr", 编码: "base64", 数据: "AP8="}
	错误 := 执行包.发布持久化(func(方法 string, 参数 any) error {
		if 方法 != "tool.output" {
			测试器.报告致命错误(方法)
		}
		原始, 错误 := JSON包.序列化(参数)
		if 错误 != nil {
			return 错误
		}
		var 已收到 struct {
			会话标识 string    `json:"conversation_id"`
			执行标识 string    `json:"run_id"`
			步骤标识 string    `json:"step_id"`
			输出   智能体包.工具输出 `json:"output"`
		}
		if 错误 := JSON包.反序列化(原始, &已收到); 错误 != nil {
			return 错误
		}
		if 已收到.会话标识 != "c" || 已收到.执行标识 != "r" || 已收到.步骤标识 != "s" || 已收到.输出 != 值 {
			测试器.格式化报告致命错误("工具分片身份或数据丢失: %s", 原始)
		}
		return nil
	}, 智能体包.事件{类型: "tool.output", 会话标识: "c", 执行标识: "r", 步骤标识: "s", 数据: 值})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
}
```
