```go
package 智能体

import (
	JSON包 "encoding/json"

	GORM包 "gorm.io/gorm"

	附件包 "github.com/xiaoyu98628/callum/internal/attachment"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 恢复已发布交付物集 只为仍属于当前分支的助手消息恢复已确认的交付引用。
// 旧版标记为中断的消息也可补齐，原工具步骤保持不变。
func 恢复已发布交付物集(事务 *GORM包.数据库, 状态集 []string) error {
	var 之后 string
	for {
		var 消息集 []表结构包.消息
		if 错误 := 事务.条件("role = 'assistant' AND status IN ? AND id > ?", 状态集, 之后).
			条件(`EXISTS (
				SELECT 1 FROM conversation_run_steps s
				WHERE s.run_id = conversation_messages.run_id AND s.conversation_id = conversation_messages.conversation_id
				AND s.type = 'tool_call' AND s.name = 'publish_artifact' AND s.status = 'completed'
				AND json_extract(s.definition, '$.source') = 'builtin'
				AND json_type(s.output, '$.result.published') = 'true'
				AND NOT EXISTS (SELECT 1 FROM json_each(conversation_messages.content) AS block
					WHERE json_extract(block.value, '$.attachment_ref') = json_extract(s.output, '$.result.attachment.attachment_ref'))
			)`).顺序("id").上限(16).查找(&消息集).错误; 错误 != nil {
			return 错误
		}
		if len(消息集) == 0 {
			return nil
		}
		for _, 消息 := range 消息集 {
			if 错误 := 恢复已发布内容块集(事务, 消息); 错误 != nil {
				return 错误
			}
		}
		之后 = 消息集[len(消息集)-1].标识
	}
}

func 恢复已发布内容块集(事务 *GORM包.数据库, 消息 表结构包.消息) error {
	已见 := make(map[string]bool)
	for _, 内容块 := range 消息.内容 {
		if 内容块.类型 == "file" {
			已见[内容块.附件引用] = true
		}
	}
	初始数量 := len(消息.内容)
	var 之后 int64
	for len(消息.内容) < 64 {
		var 步骤集 []表结构包.步骤
		if 错误 := 事务.选择("sequence", "output").
			条件("run_id = ? AND conversation_id = ? AND sequence > ?", 消息.执行标识, 消息.会话标识, 之后).
			条件("type = 'tool_call' AND name = 'publish_artifact' AND status = 'completed'").
			条件("json_extract(definition, '$.source') = 'builtin' AND json_type(output, '$.result.published') = 'true'").
			顺序("sequence").上限(64).查找(&步骤集).错误; 错误 != nil {
			return 错误
		}
		if len(步骤集) == 0 {
			break
		}
		for _, 步骤 := range 步骤集 {
			var 输出 struct {
				操作 工具包.操作结果记录     `json:"operation"`
				错误 JSON包.原始JSON消息 `json:"error"`
				结果 struct {
					附件 模型包.内容块 `json:"attachment"`
				} `json:"result"`
			}
			if JSON包.反序列化(步骤.输出, &输出) != nil || len(输出.错误) != 0 ||
				!输出.操作.有效() || 输出.操作.已恢复 || 输出.操作.操作结果 != 工具包.操作结果无 {
				continue
			}
			内容块 := 输出.结果.附件
			if 已见[内容块.附件引用] || !附件包.有效已发布内容块(内容块, 消息.会话标识, 消息.执行标识) {
				continue
			}
			消息.内容 = append(消息.内容, 内容块)
			已见[内容块.附件引用] = true
			if len(消息.内容) == 64 {
				break
			}
		}
		之后 = 步骤集[len(步骤集)-1].序号
	}
	if len(消息.内容) == 初始数量 {
		return nil
	}
	已编码, 错误 := JSON包.序列化(消息.内容)
	if 错误 != nil {
		return 错误
	}
	// 普通恢复会统一更新活动消息的状态和时间。旧中断消息保留原结束时间。
	return 事务.模型(&表结构包.消息{}).条件("id = ?", 消息.标识).更新("content", string(已编码)).错误
}
```
