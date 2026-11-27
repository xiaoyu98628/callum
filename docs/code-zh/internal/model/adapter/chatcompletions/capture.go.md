```go
package 聊天补全

import (
	JSON包 "encoding/json"
	映射包 "maps"
	字符串包 "strings"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func 捕获用量(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	if len(原始) == 0 || string(原始) == "null" {
		return nil, nil
	}
	var 对象 map[string]JSON包.原始JSON消息
	if JSON包.反序列化(原始, &对象) != nil || 对象 == nil {
		return nil, 模型包.无效响应错误
	}
	return append(JSON包.原始JSON消息(nil), 原始...), nil
}

// 输入词元 只读取厂商实际 提示词词元，字段不合法或缺失时不推算；原始 用量 保留。
func 输入词元(原始 JSON包.原始JSON消息) *int64 {
	var 用量 struct {
		提示词词元 *int64 `json:"prompt_tokens"`
	}
	if JSON包.反序列化(原始, &用量) != nil || 用量.提示词词元 == nil || *用量.提示词词元 < 0 {
		return nil
	}
	return 用量.提示词词元
}

func 已捕获输出(格式 string, 响应 JSON包.原始JSON消息, 扩展集 ...map[string][]JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	var 额外 map[string][]JSON包.原始JSON消息
	if len(扩展集) > 0 {
		额外 = 扩展集[0]
	}
	return JSON包.序列化(struct {
		协议   string                      `json:"protocol"`
		格式   string                      `json:"format"`
		响应   JSON包.原始JSON消息              `json:"response"`
		流扩展集 map[string][]JSON包.原始JSON消息 `json:"stream_extensions,omitempty"`
	}{"chatcompletions", 格式, 响应, 额外})
}

// 流捕获器 保存组装结果而非 SSE 包；原始 JSON 字段避免 SDK 宽松解码改写 用量。
type 流捕获器 struct {
	根   map[string]JSON包.原始JSON消息
	选项  map[string]JSON包.原始JSON消息
	消息  map[string]JSON包.原始JSON消息
	文本  map[string]*字符串包.构建器
	用量  JSON包.原始JSON消息
	调用集 工具调用捕获
	扩展集 map[string][]JSON包.原始JSON消息
}

func (当前流捕获器 *流捕获器) 添加(原始 string) error {
	var 根 map[string]JSON包.原始JSON消息
	if 错误 := JSON包.反序列化([]byte(原始), &根); 错误 != nil {
		return 错误
	}
	if 当前流捕获器.根 == nil {
		当前流捕获器.根 = make(map[string]JSON包.原始JSON消息)
		当前流捕获器.选项 = make(map[string]JSON包.原始JSON消息)
		当前流捕获器.消息 = make(map[string]JSON包.原始JSON消息)
		当前流捕获器.文本 = make(map[string]*字符串包.构建器)
		当前流捕获器.扩展集 = make(map[string][]JSON包.原始JSON消息)
	}
	for 键, 值 := range 根 {
		if 键 != "choices" && 键 != "usage" {
			当前流捕获器.根[键] = 值
		}
	}
	if 值, 成功 := 根["usage"]; 成功 && string(值) != "null" {
		用量, 错误 := 捕获用量(值)
		if 错误 != nil {
			return 错误
		}
		当前流捕获器.用量 = 用量
	}
	var 选项集 []map[string]JSON包.原始JSON消息
	if 错误 := JSON包.反序列化(根["choices"], &选项集); 错误 != nil {
		return 错误
	}
	if len(选项集) != 1 {
		return nil
	}
	for 键, 值 := range 选项集[0] {
		if 键 != "delta" {
			if 键 == "finish_reason" && (string(值) == "null" || len(值) == 0) {
				continue
			}
			if 键 == "logprobs" && string(值) == "null" {
				if _, 存在 := 当前流捕获器.选项[键]; 存在 {
					continue
				}
			}
			if 键 == "logprobs" && string(值) != "null" {
				已合并, 错误 := 合并对数概率(当前流捕获器.选项[键], 值)
				if 错误 != nil {
					return 错误
				}
				当前流捕获器.选项[键] = 已合并
				continue
			}
			当前流捕获器.选项[键] = 值
		}
	}
	var 增量 map[string]JSON包.原始JSON消息
	if 错误 := JSON包.反序列化(选项集[0]["delta"], &增量); 错误 != nil {
		return 错误
	}
	for 键, 值 := range 增量 {
		if 键 == "content" || 键 == "refusal" || 键 == "reasoning_content" {
			var 文本 string
			if string(值) == "null" {
				continue
			}
			if 错误 := JSON包.反序列化(值, &文本); 错误 != nil {
				return 模型包.无效响应错误
			}
			构建器 := 当前流捕获器.文本[键]
			if 构建器 == nil {
				构建器 = &字符串包.构建器{}
				当前流捕获器.文本[键] = 构建器
			}
			构建器.写入字符串(文本)
		} else if 键 == "tool_calls" {
			if 错误 := 当前流捕获器.调用集.添加(值); 错误 != nil {
				return 错误
			}
		} else if 键 == "role" {
			当前流捕获器.消息[键] = 值
		} else {
			// 未知增量字段不猜测其合并语义，按字段保留全部片段，避免只剩最后一片。
			当前流捕获器.扩展集[键] = append(当前流捕获器.扩展集[键], 值)
		}
	}
	return nil
}

func (当前流捕获器 *流捕获器) 结果(请求标识 string) (模型包.响应, error) {
	结果 := 模型包.响应{请求标识: 请求标识, 用量: 当前流捕获器.用量, 输入词元: 输入词元(当前流捕获器.用量)}
	if 当前流捕获器.根 == nil {
		return 结果, nil
	}
	for 键, 构建器 := range 当前流捕获器.文本 {
		值, 错误 := JSON包.序列化(构建器.字符串())
		if 错误 != nil {
			return 结果, 错误
		}
		当前流捕获器.消息[键] = 值
		if 键 == "content" {
			结果.文本 = 构建器.字符串()
		} else if 键 == "reasoning_content" {
			结果.推理 = new(构建器.字符串())
		}
	}
	if 调用集, 错误 := 当前流捕获器.调用集.结果(); 错误 != nil {
		return 结果, 错误
	} else if len(调用集) > 0 {
		当前流捕获器.消息["tool_calls"] = 调用集
	}
	if 原始 := 当前流捕获器.选项["finish_reason"]; len(原始) > 0 {
		if 错误 := JSON包.反序列化(原始, &结果.结束原因); 错误 != nil {
			return 结果, 模型包.无效响应错误
		}
	}
	消息, 错误 := JSON包.序列化(当前流捕获器.消息)
	if 错误 != nil {
		return 结果, 错误
	}
	当前流捕获器.选项["message"] = 消息
	选项集 := []map[string]JSON包.原始JSON消息{}
	if len(当前流捕获器.选项) > 0 {
		选项集 = append(选项集, 当前流捕获器.选项)
	}
	已编码, 错误 := JSON包.序列化(选项集)
	if 错误 != nil {
		return 结果, 错误
	}
	当前流捕获器.根["choices"] = 已编码
	if 当前流捕获器.用量 != nil {
		当前流捕获器.根["usage"] = 当前流捕获器.用量
	}
	响应, 错误 := JSON包.序列化(当前流捕获器.根)
	if 错误 != nil {
		return 结果, 错误
	}
	映射包.复制(当前流捕获器.扩展集, 当前流捕获器.调用集.未知)
	结果.输出, 错误 = 已捕获输出("stream_result", 响应, 当前流捕获器.扩展集)
	if 结果.文本 != "" {
		结果.内容 = []模型包.内容块{{类型: "text", 文本: 结果.文本}}
	}
	return 结果, 错误
}

func 合并对数概率(前一个, 下一个 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	字段集 := make(map[string]JSON包.原始JSON消息)
	if len(前一个) > 0 && string(前一个) != "null" {
		if 错误 := JSON包.反序列化(前一个, &字段集); 错误 != nil {
			return nil, 错误
		}
	}
	var 传入 map[string]JSON包.原始JSON消息
	if 错误 := JSON包.反序列化(下一个, &传入); 错误 != nil {
		return nil, 错误
	}
	for 键, 值 := range 传入 {
		if (键 == "content" || 键 == "refusal") && string(值) != "null" {
			var 旧, 当前 []JSON包.原始JSON消息
			if len(字段集[键]) > 0 {
				if 错误 := JSON包.反序列化(字段集[键], &旧); 错误 != nil {
					return nil, 错误
				}
			}
			if 错误 := JSON包.反序列化(值, &当前); 错误 != nil {
				return nil, 错误
			}
			已编码, 错误 := JSON包.序列化(append(旧, 当前...))
			if 错误 != nil {
				return nil, 错误
			}
			字段集[键] = 已编码
		} else if _, 存在 := 字段集[键]; !存在 || string(值) != "null" {
			字段集[键] = 值
		}
	}
	return JSON包.序列化(字段集)
}

func (当前流捕获器 *流捕获器) 消息JSON() JSON包.原始JSON消息 {
	值, _ := JSON包.序列化(当前流捕获器.消息)
	return 值
}
```
