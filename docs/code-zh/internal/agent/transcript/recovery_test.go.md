```go
package 执行轨迹测试

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	测试包 "testing"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	执行轨迹包 "github.com/xiaoyu98628/callum/internal/agent/transcript"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test恢复输出保留已知事实集并且标记执行边界(测试器 *测试包.测试器) {
	for _, 测试用例 := range []struct {
		名称  string
		已开始 bool
		影响集 []工具包.影响
		已知  工具包.操作结果
		预期  工具包.操作结果
	}{
		{"not_started", false, []工具包.影响{工具包.影响写入}, "", 工具包.操作结果不已生效},
		{"started_write", true, []工具包.影响{工具包.影响写入}, "", 工具包.操作结果未知},
		{"started_read", true, []工具包.影响{工具包.影响读取}, "", 工具包.操作结果无},
		{"applied", true, []工具包.影响{工具包.影响写入}, 工具包.操作结果已生效, 工具包.操作结果已生效},
		{"partial", true, []工具包.影响{工具包.影响写入}, 工具包.操作结果部分, 工具包.操作结果部分},
	} {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			步骤 := 智能体包.步骤{工具调用标识: new("call"), 输入: JSON包.原始JSON消息(`{"request_hash":"fixed"}`), 定义: 编码(测试器, 工具包.定义{影响集: 测试用例.影响集})}
			if 测试用例.已开始 {
				步骤.开始时间 = new(int64(1))
			}
			if 测试用例.已知 != "" {
				步骤.输出 = 编码(测试器, map[string]any{"operation": 工具包.操作结果记录{版本: 1, 请求哈希: "fixed", 操作结果: 测试用例.已知}, "result": map[string]bool{"saved": true}, "error": map[string]string{"kind": "original"}, "context_message": 模型包.消息{内容: "过期副本"}})
			}
			之前 := string(步骤.输出)
			原始, 错误 := 执行轨迹包.恢复输出(步骤)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var 已恢复 struct {
				操作 工具包.操作结果记录          `json:"operation"`
				结果 JSON包.原始JSON消息      `json:"result"`
				错误 struct{ 类别 string } `json:"error"`
				消息 模型包.消息              `json:"context_message"`
			}
			if 错误 := JSON包.反序列化(原始, &已恢复); 错误 != nil || 已恢复.操作.操作结果 != 测试用例.预期 || !已恢复.操作.已恢复 || 已恢复.操作.请求哈希 != "fixed" || 已恢复.消息.角色 != 模型包.角色工具 || 已恢复.消息.工具调用标识 != "call" {
				测试器.格式化报告致命错误("恢复事实错误: %s %v", 原始, 错误)
			}
			if 测试用例.已知 != "" && (string(已恢复.结果) != `{"saved":true}` || 已恢复.错误.类别 != "original") {
				测试器.格式化报告致命错误("已保存结果被覆盖: %s", 原始)
			}
			var 上下文 map[string]JSON包.原始JSON消息
			if 错误 := JSON包.反序列化([]byte(已恢复.消息.内容), &上下文); 错误 != nil || 上下文["context_message"] != nil || string(上下文["operation"]) != string(编码(测试器, 已恢复.操作)) {
				测试器.格式化报告致命错误("上下文没有独立保存相同事实: %+v %v", 已恢复.消息, 错误)
			}
			if string(步骤.输出) != 之前 {
				测试器.报告致命错误("恢复改写了来源 JSON")
			}
		})
	}
}

func Test恢复输出拒绝不匹配或者损坏事实集(测试器 *测试包.测试器) {
	for _, 输出 := range []string{`null`, `{`, `{"operation":{"version":1,"outcome":"applied","request_hash":"other"}}`, `{"operation":{"version":1,"outcome":"invalid","request_hash":"fixed"}}`} {
		_, 错误 := 执行轨迹包.恢复输出(智能体包.步骤{工具调用标识: new("call"), 输入: JSON包.原始JSON消息(`{"request_hash":"fixed"}`), 输出: JSON包.原始JSON消息(输出)})
		if !错误判断包.是否属于(错误, 模型包.无效响应错误) {
			测试器.格式化报告致命错误("损坏事实未被拒绝: %s %v", 输出, 错误)
		}
	}
}
```
