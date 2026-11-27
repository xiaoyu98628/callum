```go
package 智能体

import (
	字节包 "bytes"
	上下文包 "context"
	JSON包 "encoding/json"
	输入输出包 "io"
	字符串包 "strings"
	UTF8包 "unicode/utf8"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

const 用户输入工具 = "request_user_input"
const 等待中用户输入 = "awaiting_user_input"

// 输入问题 是模型请求用户确认的固定问题，不包含审批或权限字段。
type 输入问题 struct {
	问题  string   `json:"question"`
	选项集 []string `json:"options"`
}

// 输入回答 只提交选项下标、补充文字或跳过；选项标签从固定问题读取。
type 输入回答 struct {
	选项索引 *int   `json:"option_index"`
	文本   string `json:"text"`
	已跳过  bool   `json:"skipped"`
}

type 用户输入 struct {
	标识   string `json:"id"`
	会话标识 string `json:"conversation_id"`
	执行标识 string `json:"run_id"`
	步骤标识 string `json:"step_id"`
	输入问题
	状态   string `json:"status"`
	回答   *输入回答  `json:"answer"`
	创建时间 int64  `json:"created_at"`
	更新时间 int64  `json:"updated_at"`
}

type 用户输入仓储 interface {
	创建用户输入(上下文包.上下文, 步骤) (步骤, error)
	用户输入步骤集(上下文包.上下文, string, bool, int, int) ([]步骤, error)
	回答用户输入(上下文包.上下文, string, string, 输入回答) (步骤, bool, error)
	取消用户输入(上下文包.上下文, string) (步骤, error)
}

// 用户输入提示词 参考上游提问指引及选项描述，适配本服务的单问题和字符串选项。
const 用户输入提示词 = `Ask the user for missing information, a preference, constraint, or clarification. Use the available read tools to establish facts first. Be mindful of cognitive load on the user and prefer multiple-choice questions.

Ask one clear question at a time. Provide 2-3 mutually exclusive choices when appropriate. Put the recommended option first and suffix its label with "(Recommended)" or its equivalent in the user's language. Do not include an "Other" option. Each option is a single string and may briefly explain its tradeoff. Pass an empty options array if only a free-text answer is needed.

Ask in the user's language. The user can choose an option, answer in their own words, or skip. Complete useful independent work before requesting an answer. If a reply is required, wait for an explicit answer before dependent work. Skipping is not consent. Use the service's approval flow for tool permissions.`

func 用户输入定义() 模型包.工具定义 {
	return 模型包.工具定义{名称: 用户输入工具, 说明: 用户输入提示词, 参数: JSON包.原始JSON消息(`{"type":"object","properties":{"question":{"type":"string","minLength":1,"maxLength":4096},"options":{"type":"array","maxItems":8,"items":{"type":"string","minLength":1,"maxLength":1024}}},"required":["question","options"],"additionalProperties":false}`)}
}

// 解析输入问题 拒绝重复、未知及大小写不匹配字段和尾随内容。
func 解析输入问题(原始 []byte) (输入问题, error) {
	var 值 输入问题
	if len(原始) > 16<<10 || !UTF8包.有效(原始) {
		return 值, 无效错误
	}
	解码器 := JSON包.新建解码器(字节包.新建读取器(原始))
	词元, 错误 := 解码器.词元()
	if 错误 != nil || 词元 != JSON包.分隔符('{') {
		return 值, 无效错误
	}
	已见 := map[string]bool{}
	for 解码器.更多() {
		词元, 错误 := 解码器.词元()
		键, 成功 := 词元.(string)
		if 错误 != nil || !成功 || 已见[键] {
			return 值, 无效错误
		}
		已见[键] = true
		switch 键 {
		case "question":
			错误 = 解码器.解码(&值.问题)
		case "options":
			错误 = 解码器.解码(&值.选项集)
		default:
			return 值, 无效错误
		}
		if 错误 != nil {
			return 值, 无效错误
		}
	}
	if 词元, 错误 = 解码器.词元(); 错误 != nil || 词元 != JSON包.分隔符('}') {
		return 值, 无效错误
	}
	if 解码器.解码(new(any)) != 输入输出包.文件结束 || !已见["question"] || !已见["options"] {
		return 值, 无效错误
	}
	值.问题 = 字符串包.裁剪空间(值.问题)
	if 值.问题 == "" || len(值.问题) > 4096 || 字符串包.包含字符(值.问题, 0) || 值.选项集 == nil || len(值.选项集) > 8 {
		return 值, 无效错误
	}
	选项集 := map[string]bool{}
	for 索引, 选项 := range 值.选项集 {
		选项 = 字符串包.裁剪空间(选项)
		if 选项 == "" || len(选项) > 1024 || 字符串包.包含字符(选项, 0) || 选项集[选项] {
			return 值, 无效错误
		}
		选项集[选项] = true
		值.选项集[索引] = 选项
	}
	return 值, nil
}

func 校验输入回答(问题 输入问题, 回答 输入回答) error {
	if !UTF8包.有效字符串(回答.文本) || len(回答.文本) > 4096 || 字符串包.包含字符(回答.文本, 0) {
		return 无效错误
	}
	if 回答.选项索引 != nil && (*回答.选项索引 < 0 || *回答.选项索引 >= len(问题.选项集)) {
		return 无效错误
	}
	if 回答.已跳过 {
		if 回答.选项索引 != nil || 回答.文本 != "" {
			return 无效错误
		}
	} else if 回答.选项索引 == nil && 字符串包.裁剪空间(回答.文本) == "" {
		return 无效错误
	}
	return nil
}

func 输入来源步骤(步骤 步骤) (用户输入, error) {
	if 步骤.类型 != "user_input" {
		return 用户输入{}, 未找到错误
	}
	问题, 错误 := 解析输入问题(步骤.输入)
	if 错误 != nil {
		return 用户输入{}, 错误
	}
	值 := 用户输入{标识: 步骤.标识, 会话标识: 步骤.会话标识, 执行标识: 步骤.执行标识, 步骤标识: 步骤.标识, 输入问题: 问题, 状态: 步骤.状态, 创建时间: 步骤.创建时间, 更新时间: 步骤.更新时间}
	if 步骤.状态 == 等待中用户输入 {
		值.状态 = "pending"
	}
	if 步骤.状态 == 已完成 {
		var 输出 struct {
			回答 *输入回答 `json:"answer"`
		}
		if JSON包.反序列化(步骤.输出, &输出) != nil || 输出.回答 == nil || 校验输入回答(问题, *输出.回答) != nil {
			return 用户输入{}, 无效错误
		}
		值.回答, 值.状态 = 输出.回答, "answered"
		if 值.回答.已跳过 {
			值.状态 = "skipped"
		}
	}
	return 值, nil
}

func 输入回答输出(步骤 步骤, 回答 输入回答) (JSON包.原始JSON消息, error) {
	问题, 错误 := 解析输入问题(步骤.输入)
	if 错误 != nil || 步骤.工具调用标识 == nil {
		return nil, 无效错误
	}
	if 错误 := 校验输入回答(问题, 回答); 错误 != nil {
		return nil, 错误
	}
	var 已选择 *string
	if 回答.选项索引 != nil {
		已选择 = new(问题.选项集[*回答.选项索引])
	}
	正文, 错误 := JSON包.序列化(map[string]any{"question": 问题.问题, "selected_option": 已选择, "text": 回答.文本, "skipped": 回答.已跳过})
	if 错误 != nil {
		return nil, 错误
	}
	消息 := 模型包.消息{角色: 模型包.角色工具, 工具调用标识: *步骤.工具调用标识, 内容: string(正文)}
	return JSON包.序列化(map[string]any{"answer": 回答, "context_message": 消息})
}
```
