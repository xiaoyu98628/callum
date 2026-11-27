```go
package 历史

import (
	JSON包 "encoding/json"
	字符串包 "strings"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 上下文 在所属用户消息后恢复成对的调用轨迹，包含未完成执行留下的操作事实。
// 普通的失败回答不作为完整回答恢复，已替换分支由仓储过滤。
func 上下文(消息集 []会话包.消息, 轨迹集 map[string][]模型包.消息) ([]模型包.消息, error) {
	输出, 错误 := 组装(消息集, 轨迹集)
	if 错误 != nil {
		return nil, 错误
	}
	if 错误 := 检查(输出); 错误 != nil {
		return nil, 错误
	}
	return 输出, nil
}

// 组装 允许在硬请求上限前准备压缩，但准备窗口也限制为 512 条、16 MiB。
func 组装(消息集 []会话包.消息, 轨迹集 map[string][]模型包.消息) ([]模型包.消息, error) {
	输出 := make([]模型包.消息, 0, len(消息集))
	for _, 消息 := range 消息集 {
		if 消息.角色 == "assistant" && (len(轨迹集[消息.执行标识]) > 0 || 消息.状态 != 智能体包.已完成) {
			continue
		}
		输出 = append(输出, 模型包.消息{角色: 模型包.角色(消息.角色), 内容块集: 模型内容(消息.内容)})
		if 消息.角色 == "user" {
			输出 = append(输出, 轨迹集[消息.执行标识]...)
		}
	}
	原始, 错误 := JSON包.序列化(输出)
	if 错误 != nil {
		return nil, 错误
	}
	if len(输出) > 512 {
		return nil, &模型包.输入上限错误详情{尺寸: "历史准备消息数量", 实际: int64(len(输出)), 上限: 512}
	}
	if len(原始) > 16<<20 {
		return nil, &模型包.输入上限错误详情{尺寸: "历史准备字节", 实际: int64(len(原始)), 上限: 16 << 20}
	}
	return 输出, nil
}

// 模型内容 将已保存的技能引用转换为模型文本和定位元数据。
// 原始文字保持独立，标题和消息展示可使用用户正文。
func 模型内容(内容 []模型包.内容块) []模型包.内容块 {
	var 引用集 []模型包.内容块
	var 名称集 []string
	for _, 内容块 := range 内容 {
		if 内容块.类型 == "skill" {
			引用集 = append(引用集, 内容块)
			名称集 = append(名称集, "$"+内容块.名称)
		}
	}
	if len(引用集) == 0 {
		return 内容
	}
	内容块集 := []模型包.内容块{{类型: "text", 文本: 字符串包.合并(名称集, " ")}}
	首个 := true
	for _, 内容块 := range 内容 {
		if 内容块.类型 == "skill" {
			continue
		}
		if 首个 && 内容块.类型 == "text" {
			内容块集[0].文本 += " " + 内容块.文本
		} else {
			内容块集 = append(内容块集, 内容块)
		}
		首个 = false
	}
	数据, _ := JSON包.序列化(map[string]any{"skills": 引用集}) // 内容块 的字段均可 JSON 编码。
	return append(内容块集, 模型包.内容块{类型: "text", 文本: "\n\n" + string(数据)})
}

// 检查 限制消息数量和实际序列化大小，不估算 词元。
func 检查(消息集 []模型包.消息) error {
	if len(消息集) > 256 {
		return &模型包.输入上限错误详情{尺寸: "消息数量", 实际: int64(len(消息集)), 上限: 256}
	}
	值, 错误 := JSON包.序列化(消息集)
	if 错误 != nil {
		return 错误
	}
	if len(值) > 最大字节 {
		return &模型包.输入上限错误详情{尺寸: "上下文字节", 实际: int64(len(值)), 上限: 最大字节}
	}
	return nil
}
```
