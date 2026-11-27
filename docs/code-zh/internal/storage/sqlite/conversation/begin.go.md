```go
package 会话

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	时间包 "time"

	GORM包 "gorm.io/gorm"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	附件包 "github.com/xiaoyu98628/callum/internal/attachment"
	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	历史包 "github.com/xiaoyu98628/callum/internal/conversation/history"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	智能体仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// 开始 在同一写事务中接受输入并建立执行；事务返回后才可调用模型。
func (当前仓储 *仓储) 开始(执行上下文 上下文包.上下文, 输入 业务域包.开始) (输出 业务域包.执行, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		// 与设置更新按同一数据库事务顺序确定配置；读失败时不创建会话或回退权限。
		快照, 错误 := 执行设置(事务, 执行上下文)
		if 错误 != nil {
			return 错误
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		var 会话 表结构包.会话
		错误 = 事务.条件("id = ?", 输入.会话标识).取出(&会话).错误
		if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
			if !输入.创建 || 输入.编辑重发消息标识 != "" {
				return 业务域包.未找到错误
			}
			if 错误 := 活动工作区(事务, 输入.工作区标识); 错误 != nil {
				return 错误
			}
			会话 = 表结构包.会话{标识: 输入.会话标识, 工作区标识: 输入.工作区标识, 标题: 历史包.标题(输入.内容), 标题来源: "default", 模型: 输入.模型, 创建时间: 当前时间, 更新时间: 当前时间}
			if 错误 := 事务.省略("Workspace").创建(&会话).错误; 错误 != nil {
				return 错误
			}
			输出.已创建 = true
		} else if 错误 != nil {
			return 错误
		} else {
			if 输入.创建 {
				return 业务域包.冲突错误
			}
			if 输入.工作区标识 != nil {
				return 业务域包.无效错误
			}
			if 会话.已归档位于 != nil || 会话.删除时间 != nil {
				return 业务域包.非活动错误
			}
			if 错误 := 活动工作区(事务, 会话.工作区标识); 错误 != nil {
				return 错误
			}
		}
		if 错误 := 繁忙(事务, 会话.标识); 错误 != nil {
			return 错误
		}
		if 输入.继续 {
			输出, 错误 = 继续执行(事务, 输入, 会话, 快照, 当前时间)
			return 错误
		}
		if 输入.后续消息 != nil {
			if 错误 := 消费后续消息(事务, 输入); 错误 != nil {
				return 错误
			}
		}
		var 用户 表结构包.消息
		var 已替换 *string
		if 输入.编辑重发消息标识 != "" {
			if 错误 := 事务.条件("id = ? AND conversation_id = ? AND role = 'user'", 输入.编辑重发消息标识, 会话.标识).取出(&用户).错误; 错误 != nil {
				if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
					return 业务域包.未找到错误
				}
				return 错误
			}
			var 最新 int64
			if 错误 := 事务.模型(&表结构包.消息{}).条件("conversation_id = ? AND role = 'user'", 会话.标识).选择("COALESCE(MAX(sequence),0)").扫描(&最新).错误; 错误 != nil {
				return 错误
			}
			if 用户.序号 != 最新 || (输入.编辑重发执行标识 != "" && 用户.执行标识 != 输入.编辑重发执行标识) {
				return 业务域包.冲突错误
			}
			if 错误 := 使失效上下文集(事务, 会话.标识, 用户.序号, 当前时间); 错误 != nil {
				return 错误
			}
			旧 := 用户.执行标识
			已替换 = &旧
			if 错误 := 事务.模型(&表结构包.消息{}).条件("conversation_id = ? AND sequence > ?", 会话.标识, 用户.序号).提取列("id", &输出.已移除消息标识集).错误; 错误 != nil {
				return 错误
			}
			var 引导标识集 []string
			if 错误 := 事务.模型(&表结构包.后续消息{}).条件("conversation_id=? AND expected_run_id=? AND status=?", 会话.标识, 旧, 业务域包.后续消息已生效).提取列("id", &引导标识集).错误; 错误 != nil {
				return 错误
			}
			输出.已移除消息标识集 = append(输出.已移除消息标识集, 引导标识集...)
			if 错误 := 事务.条件("conversation_id = ? AND sequence > ?", 会话.标识, 用户.序号).删除(&表结构包.消息{}).错误; 错误 != nil {
				return 错误
			}
			// 清理被替换回答及标题的模型副本；仍有效的历史摘要、实际工具结果和 用量 保留。
			if 错误 := 事务.模型(&表结构包.步骤{}).条件("run_id = ? AND type = 'model_call' AND COALESCE(json_extract(input,'$.purpose'),'')<>'context_summary'", 旧).更新("output", nil).错误; 错误 != nil {
				return 错误
			}
			if 用户.序号 == 1 && 会话.标题来源 == "default" {
				会话.标题 = 历史包.标题(输入.内容)
			}
		} else {
			var 最新 int64
			if 错误 := 事务.模型(&表结构包.消息{}).条件("conversation_id = ?", 会话.标识).选择("COALESCE(MAX(sequence),0)").扫描(&最新).错误; 错误 != nil {
				return 错误
			}
			用户 = 表结构包.消息{标识: 输入.用户消息标识, 会话标识: 会话.标识, 序号: 最新 + 1, 角色: "user", 创建时间: 当前时间}
		}
		过去, 当前, 更多, 错误 := 加载上下文历史(事务, 会话.标识, 用户.序号)
		if 错误 != nil {
			return 错误
		}
		已编码, 错误 := JSON包.序列化(过去)
		if 错误 != nil {
			return 错误
		}
		传入, 错误 := JSON包.序列化(输入.内容)
		if 错误 != nil {
			return 错误
		}
		if len(已编码)+len(传入) > 16<<20 {
			return 智能体包.输出上限错误
		}
		输出.上下文, 输出.历史更多 = 当前, 更多
		执行行 := 表结构包.执行{标识: 输入.执行标识, 会话标识: 会话.标识, 替换执行标识: 已替换, 模型: 输入.模型, 流: 输入.流, 设置快照: &快照, 状态: 智能体包.运行中, 创建时间: 当前时间, 更新时间: 当前时间}
		if 错误 := 事务.省略("Conversation", "Replaces").创建(&执行行).错误; 错误 != nil {
			return 错误
		}
		if 错误 := 绑定后续消息队列(事务, 输入); 错误 != nil {
			return 错误
		}
		用户.执行标识, 用户.内容, 用户.状态, 用户.更新时间, 用户.已完成位于 = 输入.执行标识, 输入.内容, 智能体包.已完成, 当前时间, &当前时间
		if 输入.编辑重发消息标识 != "" {
			if 错误 := 事务.省略("Run").选择("RunID", "Content", "Status", "UpdatedAt", "CompletedAt").更新集(&用户).错误; 错误 != nil {
				return 错误
			}
		} else if 错误 := 事务.省略("Run").创建(&用户).错误; 错误 != nil {
			return 错误
		}
		助手 := 表结构包.消息{标识: 输入.助手消息标识, 会话标识: 会话.标识, 执行标识: 输入.执行标识, 序号: 用户.序号 + 1, 角色: "assistant", 内容: []模型包.内容块{}, 状态: 智能体包.运行中, 创建时间: 当前时间, 更新时间: 当前时间}
		if 错误 := 事务.省略("Run").创建(&助手).错误; 错误 != nil {
			return 错误
		}
		输出.历史 = 过去
		来源集 := make([]map[string]any, 0, len(过去)+1)
		for _, 行 := range 过去 {
			来源集 = append(来源集, map[string]any{"id": 行.标识, "fingerprint": 历史包.指纹(行.内容)})
		}
		输出.历史 = append(输出.历史, 消息(用户))
		来源集 = append(来源集, map[string]any{"id": 用户.标识, "fingerprint": 历史包.指纹(用户.内容)})
		字段集 := map[string]any{"protocol": 输入.协议, "messages": 来源集}
		输入, 错误 := JSON包.序列化(字段集)
		if 错误 != nil {
			return 错误
		}
		步骤 := 表结构包.步骤{标识: 输入.步骤标识, 会话标识: 会话.标识, 执行标识: 输入.执行标识, 序号: 1, 类型: "model_call", 名称: "生成回答", 状态: 智能体包.待处理, 输入: 输入, 模型: &输入.模型, 创建时间: 当前时间, 更新时间: 当前时间}
		if 错误 := 事务.省略("Run", "Parent").创建(&步骤).错误; 错误 != nil {
			return 错误
		}
		会话.模型, 会话.更新时间 = 输入.模型, 当前时间
		if 错误 := 事务.省略("Workspace").选择("Model", "Title", "UpdatedAt").更新集(&会话).错误; 错误 != nil {
			return 错误
		}
		输出.会话, 输出.用户, 输出.助手 = 转换(会话), 消息(用户), 消息(助手)
		输出.执行 = 智能体包.执行{标识: 输入.执行标识, 会话标识: 会话.标识, 替换执行标识: 已替换, 模型: 输入.模型, 流: 输入.流, 设置快照: &快照, 状态: 智能体包.运行中, 创建时间: 当前时间, 更新时间: 当前时间}
		输出.步骤 = 智能体包.步骤{标识: 步骤.标识, 会话标识: 会话.标识, 执行标识: 输入.执行标识, 序号: 1, 类型: 步骤.类型, 名称: 步骤.名称, 状态: 步骤.状态, 输入: 输入, 模型: &输入.模型, 创建时间: 当前时间, 更新时间: 当前时间}
		return nil
	})
	return 输出, 错误
}

// 继续执行 保留消息和执行的标识、顺序，新增的模型步骤记录本次恢复的审计边界。
func 继续执行(事务 *GORM包.数据库, 输入 业务域包.开始, 会话 表结构包.会话, 快照 设置包.快照, 当前时间 int64) (输出 业务域包.执行, 错误 error) {
	var 用户 表结构包.消息
	错误 = 事务.条件("conversation_id=? AND role='user'", 会话.标识).顺序("sequence DESC").取出(&用户).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 输出, 业务域包.无法继续错误
	}
	if 错误 != nil {
		return 输出, 错误
	}
	if 用户.执行标识 != 输入.执行标识 {
		return 输出, 业务域包.无法继续错误
	}
	var 运行 表结构包.执行
	if 错误 := 事务.条件("id=? AND conversation_id=?", 输入.执行标识, 会话.标识).取出(&运行).错误; 错误 != nil {
		return 输出, 错误
	}
	if 运行.状态 != 智能体包.已取消 && 运行.状态 != 智能体包.已中断 {
		return 输出, 业务域包.无法继续错误
	}
	var 助手 表结构包.消息
	if 错误 := 事务.条件("conversation_id=? AND run_id=? AND role='assistant'", 会话.标识, 运行.标识).取出(&助手).错误; 错误 != nil {
		return 输出, 错误
	}
	if 助手.状态 != 运行.状态 {
		return 输出, 业务域包.无法继续错误
	}
	过去, 当前, 更多, 错误 := 加载上下文历史(事务, 会话.标识, 用户.序号)
	if 错误 != nil {
		return 输出, 错误
	}
	if !输入.继续图片输入 {
		if 错误 := 检查继续图片输入(事务, 用户, 当前); 错误 != nil {
			return 输出, 错误
		}
	}
	var 序号 int64
	if 错误 := 事务.模型(&表结构包.步骤{}).条件("run_id=?", 运行.标识).选择("COALESCE(MAX(sequence),0)").扫描(&序号).错误; 错误 != nil {
		return 输出, 错误
	}
	// 状态、权限和模型快照随本次受理更新，上一次执行的数据留在新步骤的来源记录中。
	输入, 错误 := JSON包.序列化(map[string]any{
		"purpose": "answer", "protocol": 输入.协议,
		"resume": map[string]any{"status": 运行.状态, "finished_at": 运行.结束时间, "model": 运行.模型,
			"settings_snapshot": 运行.设置快照, "error_kind": 运行.错误类别, "error_message": 运行.错误消息},
	})
	if 错误 != nil {
		return 输出, 错误
	}
	步骤 := 表结构包.步骤{标识: 输入.步骤标识, 会话标识: 会话.标识, 执行标识: 运行.标识, 序号: 序号 + 1, 类型: "model_call", 名称: "继续生成回答", 状态: 智能体包.待处理, 输入: 输入, 模型: &输入.模型, 创建时间: 当前时间, 更新时间: 当前时间}
	if 错误 := 事务.省略("Run", "Parent").创建(&步骤).错误; 错误 != nil {
		return 输出, 错误
	}
	运行.模型, 运行.设置快照, 运行.状态, 运行.更新时间 = 输入.模型, &快照, 智能体包.运行中, 当前时间
	运行.错误类别, 运行.错误消息, 运行.结束时间 = nil, nil, nil
	if 错误 := 事务.选择("Model", "SettingsSnapshot", "Status", "UpdatedAt", "ErrorKind", "ErrorMessage", "FinishedAt").更新集(&运行).错误; 错误 != nil {
		return 输出, 错误
	}
	// 保留中断时的正文，直到新模型返回内容；已发布的交付物仍与原消息关联。
	交付物集 := []模型包.内容块{}
	for _, 内容块 := range 助手.内容 {
		if 附件包.有效已发布内容块(内容块, 会话.标识, 运行.标识) {
			交付物集 = append(交付物集, 内容块)
		}
	}
	助手.状态, 助手.更新时间, 助手.已完成位于 = 智能体包.运行中, 当前时间, nil
	if 错误 := 事务.选择("Status", "UpdatedAt", "CompletedAt").更新集(&助手).错误; 错误 != nil {
		return 输出, 错误
	}
	会话.模型, 会话.更新时间 = 输入.模型, 当前时间
	if 错误 := 事务.选择("Model", "UpdatedAt").更新集(&会话).错误; 错误 != nil {
		return 输出, 错误
	}
	return 业务域包.执行{已继续: true, 已发布交付物集: 交付物集, 会话: 转换(会话), 用户: 消息(用户), 助手: 消息(助手),
		上下文: 当前, 历史更多: 更多, 历史: append(过去, 消息(用户)),
		执行: 智能体仓储包.执行(智能体仓储包.执行行{执行: 运行, 已继续位于: &当前时间}),
		步骤: 智能体包.步骤{标识: 步骤.标识, 会话标识: 会话.标识, 执行标识: 运行.标识, 序号: 步骤.序号, 类型: 步骤.类型, 名称: 步骤.名称, 状态: 步骤.状态, 输入: 输入, 模型: &输入.模型, 创建时间: 当前时间, 更新时间: 当前时间},
	}, nil
}

// 检查继续图片输入 检查摘要边界后的用户消息及已生效引导，包含历史分页尚未加载的轮次。
// 当前任务的原消息始终随继续请求发送。工具图片由适配器按模型能力处理。
func 检查继续图片输入(事务 *GORM包.数据库, 用户 表结构包.消息, 当前 *业务域包.上下文版本) error {
	var 下限 int64
	if 当前 != nil {
		下限 = 当前.通过序号
	}
	消息图片集 := 事务.表("json_each(m.content) AS b").选择("1").
		条件("json_extract(b.value, '$.type') = 'image'")
	引导图片集 := 事务.模型(&表结构包.后续消息{}).表("conversation_followups AS f").选择("1").
		关联("JOIN conversation_run_steps AS s ON s.id = f.applied_step_id AND s.run_id = r.id").
		关联("JOIN json_each(f.content) AS b").
		条件("f.conversation_id = m.conversation_id AND f.expected_run_id = r.id").
		条件("f.status = ? AND (r.finished_at IS NOT NULL OR m.id = ?)", 业务域包.后续消息已生效, 用户.标识).
		条件("json_extract(b.value, '$.type') = 'image'")
	var 需要图片 bool
	错误 := 事务.模型(&表结构包.消息{}).表("conversation_messages AS m").选择("1").
		关联("JOIN conversation_runs AS r ON r.id = m.run_id").
		条件("m.conversation_id = ? AND m.role = 'user' AND m.sequence <= ?", 用户.会话标识, 用户.序号).
		条件("(m.sequence > ? OR m.id = ?)", 下限, 用户.标识).
		条件("(EXISTS (?) OR EXISTS (?))", 消息图片集, 引导图片集).
		上限(1).扫描(&需要图片).错误
	if 错误 != nil {
		return 错误
	}
	if 需要图片 {
		return 模型包.不支持输出错误
	}
	return nil
}

func (当前仓储 *仓储) 应用标题(执行上下文 上下文包.上下文, 标识, 用户标识, 指纹, 标题 string) (输出 业务域包.会话, 已生效 bool, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.会话
		if 错误 := 事务.条件("id = ?", 标识).取出(&行).错误; 错误 != nil {
			return 错误
		}
		if 行.标题来源 != "default" {
			输出 = 转换(行)
			return nil
		}
		var 首个 表结构包.消息
		if 错误 := 事务.条件("id = ? AND conversation_id = ? AND sequence = 1 AND role = 'user'", 用户标识, 标识).取出(&首个).错误; 错误 != nil {
			return 错误
		}
		if 历史包.指纹(首个.内容) != 指纹 {
			输出 = 转换(行)
			return nil
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		if 错误 := 事务.模型(&行).更新集(map[string]any{"title": 标题, "title_source": "generated", "updated_at": 当前时间}).错误; 错误 != nil {
			return 错误
		}
		行.标题, 行.标题来源, 行.更新时间 = 标题, "generated", 当前时间
		输出 = 转换(行)
		已生效 = true
		return nil
	})
	return 输出, 已生效, 错误
}
```
