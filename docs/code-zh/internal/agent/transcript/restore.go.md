```go
package 执行轨迹

import (
	JSON包 "encoding/json"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 恢复 恢复单次执行的有序轨迹，过滤未完成回答并补齐未执行的调用结果。
// 输入记录必须属于同一执行；原始数据只读，返回值不持有数据库或执行资源。
func 恢复(状态 string, 行集 []条目) ([]模型包.消息, error) {
	if len(行集) == 0 {
		return []模型包.消息{}, nil
	}
	已解码 := make([]模型包.消息, len(行集))
	结果集 := map[string]map[string]int{}
	for 索引, 行 := range 行集 {
		if JSON包.反序列化(行.消息, &已解码[索引]) != nil {
			return nil, 模型包.无效响应错误
		}
		映射 := 已解码[索引]
		if 映射.角色 == 模型包.角色工具 {
			if 行.父级步骤标识 == nil || 行.工具调用标识 == nil || 映射.工具调用标识 != *行.工具调用标识 {
				return nil, 模型包.无效响应错误
			}
			if 结果集[*行.父级步骤标识] == nil {
				结果集[*行.父级步骤标识] = map[string]int{}
			}
			if _, 存在 := 结果集[*行.父级步骤标识][映射.工具调用标识]; 存在 {
				return nil, 模型包.无效响应错误
			}
			结果集[*行.父级步骤标识][映射.工具调用标识] = 索引
		}
	}
	if 状态 == "completed" {
		return 已解码, 检查执行轨迹(已解码)
	}
	输出 := []模型包.消息{}
	已使用 := map[int]bool{}
	for 索引, 行 := range 行集 {
		消息 := 已解码[索引]
		if 消息.角色 == 模型包.角色用户 {
			输出 = append(输出, 消息)
			continue
		}
		if 消息.角色 != 模型包.角色助手 || len(消息.工具调用集) == 0 {
			continue
		}
		// 未成功完成的回答正文不进入后续上下文，只恢复调用及事实。
		消息.内容, 消息.内容块集 = "", nil
		输出 = append(输出, 消息)
		for _, 调用 := range 消息.工具调用集 {
			if 索引, 成功 := 结果集[行.步骤标识][调用.标识]; 成功 {
				输出 = append(输出, 已解码[索引])
				已使用[索引] = true
			} else {
				// 同轮剩余调用尚未建立步骤，未进入执行边界，补齐未执行结果。
				正文, 错误 := JSON包.序列化(map[string]any{"operation": 工具包.操作结果记录{版本: 工具包.操作版本, 操作结果: 工具包.操作结果不已生效, 已恢复: true}, "result": nil,
					"error": map[string]string{"kind": "not_executed", "message": "执行已结束，该调用尚未执行"}})
				if 错误 != nil {
					return nil, 错误
				}
				输出 = append(输出, 模型包.消息{角色: 模型包.角色工具, 工具调用标识: 调用.标识, 内容: string(正文)})
			}
		}
	}
	for 索引, 行 := range 行集 {
		if 行.操作 == nil {
			continue
		}
		var 事实集 工具包.操作结果记录
		if JSON包.反序列化(行.操作, &事实集) != nil || !事实集.有效() || (事实集.操作结果.可能拥有影响集() && !已使用[索引]) {
			return nil, 模型包.无效响应错误
		}
	}
	return 输出, 检查执行轨迹(输出)
}

func 检查执行轨迹(消息集 []模型包.消息) error {
	待处理 := map[string]bool{}
	for _, 消息 := range 消息集 {
		switch 消息.角色 {
		case 模型包.角色用户:
			if len(待处理) != 0 || len(消息.工具调用集) != 0 || 消息.工具调用标识 != "" {
				return 模型包.无效响应错误
			}
		case 模型包.角色助手:
			if len(待处理) != 0 {
				return 模型包.无效响应错误
			}
			for _, 调用 := range 消息.工具调用集 {
				if 调用.标识 == "" || 待处理[调用.标识] {
					return 模型包.无效响应错误
				}
				待处理[调用.标识] = true
			}
		case 模型包.角色工具:
			if !待处理[消息.工具调用标识] {
				return 模型包.无效响应错误
			}
			delete(待处理, 消息.工具调用标识)
		default:
			return 模型包.无效响应错误
		}
	}
	if len(待处理) != 0 {
		return 模型包.无效响应错误
	}
	return nil
}
```
