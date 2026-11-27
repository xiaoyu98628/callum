```go
package 执行轨迹

import (
	JSON包 "encoding/json"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 恢复输出 根据已保存的执行边界恢复工具事实，保留已知结果且不重新执行操作。
// 返回独立的 JSON 输出；调用方负责将其与执行终态在同一事务中保存。
func 恢复输出(行 智能体包.步骤) (JSON包.原始JSON消息, error) {
	if 行.工具调用标识 == nil || *行.工具调用标识 == "" {
		return nil, 模型包.无效响应错误
	}
	var 输入 struct {
		请求哈希 string `json:"request_hash"`
	}
	if 错误 := JSON包.反序列化(行.输入, &输入); 错误 != nil {
		return nil, 错误
	}
	字段集 := map[string]JSON包.原始JSON消息{}
	if len(行.输出) > 0 && JSON包.反序列化(行.输出, &字段集) != nil || 字段集 == nil {
		return nil, 模型包.无效响应错误
	}
	事实集 := 工具包.操作结果记录{版本: 工具包.操作版本, 操作结果: 工具包.操作结果不已生效, 请求哈希: 输入.请求哈希, 已恢复: true}
	if 原始, 成功 := 字段集["operation"]; 成功 {
		if JSON包.反序列化(原始, &事实集) != nil || !事实集.有效() || 事实集.请求哈希 != 输入.请求哈希 {
			return nil, 模型包.无效响应错误
		}
		事实集.已恢复 = true
	} else if 行.开始时间 != nil {
		var 定义 工具包.定义
		if JSON包.反序列化(行.定义, &定义) != nil {
			return nil, 模型包.无效响应错误
		}
		事实集.操作结果 = 工具包.未确认操作结果(定义.影响集)
	}
	var 错误 error
	字段集["operation"], 错误 = JSON包.序列化(事实集)
	if 错误 != nil {
		return nil, 错误
	}
	if _, 成功 := 字段集["result"]; !成功 {
		字段集["result"] = JSON包.原始JSON消息(`null`)
	}
	if _, 成功 := 字段集["error"]; !成功 {
		类别, 消息 := "result_unconfirmed", "操作返回结果未完整保存，请核实已记录的操作事实"
		if 事实集.操作结果 == 工具包.操作结果未知 {
			类别, 消息 = "outcome_unknown", "操作可能已经生效，请核实后再重试"
		} else if 事实集.操作结果 == 工具包.操作结果不已生效 {
			类别, 消息 = "not_executed", "该操作尚未执行"
		} else if 事实集.操作结果 == 工具包.操作结果无 {
			消息 = "工具返回结果未完整保存，该工具不产生副作用"
		}
		字段集["error"], 错误 = JSON包.序列化(map[string]string{"kind": 类别, "message": 消息})
		if 错误 != nil {
			return nil, 错误
		}
	}
	delete(字段集, "context_message")
	正文, 错误 := JSON包.序列化(字段集)
	if 错误 != nil {
		return nil, 错误
	}
	字段集["context_message"], 错误 = JSON包.序列化(模型包.消息{角色: 模型包.角色工具, 内容: string(正文), 工具调用标识: *行.工具调用标识})
	if 错误 != nil {
		return nil, 错误
	}
	return JSON包.序列化(字段集)
}
```
