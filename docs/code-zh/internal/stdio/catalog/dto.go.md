```go
package 目录管理

import (
	字节包 "bytes"
	JSON包 "encoding/json"

	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

type 供应商字段集 struct {
	名称    string         `json:"name"`
	协议    string         `json:"protocol"`
	基础URL string         `json:"base_url"`
	API密钥 JSON包.原始JSON消息 `json:"api_key"`
}

func (当前供应商字段集 供应商字段集) 输入() (供应商包.供应商输入, *协议包.错误) {
	输入 := 供应商包.供应商输入{名称: 当前供应商字段集.名称, 协议: 当前供应商字段集.协议, 基础URL: 当前供应商字段集.基础URL}
	if 当前供应商字段集.API密钥 != nil {
		var 键 *string
		if JSON包.反序列化(当前供应商字段集.API密钥, &键) != nil || 键 == nil {
			return 输入, 协议包.无效参数()
		}
		输入.API密钥 = 键
	}
	return 输入, nil
}

type 供应商查看 struct {
	标识       string `json:"id"`
	名称       string `json:"name"`
	协议       string `json:"protocol"`
	基础URL    string `json:"base_url"`
	已配置API密钥 bool   `json:"has_api_key"`
	创建时间     int64  `json:"created_at"`
	更新时间     int64  `json:"updated_at"`
}

func 查看供应商(当前供应商 供应商包.供应商) 供应商查看 {
	return 供应商查看{当前供应商.标识, 当前供应商.名称, 当前供应商.协议, 当前供应商.基础URL, 当前供应商.已配置API密钥, 当前供应商.创建时间, 当前供应商.更新时间}
}

type 模型字段集 struct {
	供应商标识  string         `json:"provider_id"`
	显示名称   string         `json:"display_name"`
	远程模型标识 string         `json:"remote_model_id"`
	已启用    *bool          `json:"enabled"`
	上下文窗口  *int64         `json:"context_window"`
	最大输出词元 *int64         `json:"max_output_tokens"`
	能力集    JSON包.原始JSON消息 `json:"capabilities"`
	模型类型   []string       `json:"model_type"`
	推理等级集  []供应商包.推理级别    `json:"reasoning_levels"`
}

func (当前模型字段集 模型字段集) 输入() (供应商包.模型输入, *协议包.错误) {
	输入 := 供应商包.模型输入{供应商标识: 当前模型字段集.供应商标识, 显示名称: 当前模型字段集.显示名称, 远程模型标识: 当前模型字段集.远程模型标识,
		上下文窗口: 当前模型字段集.上下文窗口, 最大输出词元: 当前模型字段集.最大输出词元, 模型类型: 当前模型字段集.模型类型, 推理等级集: 当前模型字段集.推理等级集}
	if 当前模型字段集.已启用 == nil {
		return 输入, 协议包.无效参数()
	}
	输入.已启用 = *当前模型字段集.已启用
	if 当前模型字段集.能力集 != nil && !字节包.相等(字节包.裁剪空间(当前模型字段集.能力集), []byte("null")) {
		var 字段集 map[string]JSON包.原始JSON消息
		if JSON包.反序列化(当前模型字段集.能力集, &字段集) != nil || 字段集 == nil {
			return 输入, 协议包.无效参数()
		}
		输入.能力集 = make(map[string]bool, len(字段集))
		for 键, 原始 := range 字段集 {
			var 值 *bool
			if JSON包.反序列化(原始, &值) != nil || 值 == nil {
				return 输入, 协议包.无效参数()
			}
			输入.能力集[键] = *值
		}
	}
	return 输入, nil
}

type 模型查看 struct {
	标识     string          `json:"id"`
	供应商标识  string          `json:"provider_id"`
	显示名称   string          `json:"display_name"`
	远程模型标识 string          `json:"remote_model_id"`
	已启用    bool            `json:"enabled"`
	上下文窗口  *int64          `json:"context_window"`
	最大输出词元 *int64          `json:"max_output_tokens"`
	能力集    map[string]bool `json:"capabilities"`
	模型类型   []string        `json:"model_type"`
	推理等级集  []供应商包.推理级别     `json:"reasoning_levels"`
	创建时间   int64           `json:"created_at"`
	更新时间   int64           `json:"updated_at"`
}

func 查看模型(当前模型 供应商包.模型) 模型查看 {
	return 模型查看{当前模型.标识, 当前模型.供应商标识, 当前模型.显示名称, 当前模型.远程模型标识, 当前模型.已启用, 当前模型.上下文窗口,
		当前模型.最大输出词元, 当前模型.能力集, 当前模型.模型类型, 当前模型.推理等级集, 当前模型.创建时间, 当前模型.更新时间}
}

type 标识参数 struct {
	标识 string `json:"id"`
}
type 标识结果 struct {
	标识 string `json:"id"`
}
```
