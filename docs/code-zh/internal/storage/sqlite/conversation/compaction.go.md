```go
package 会话

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	时间包 "time"

	GORM包 "gorm.io/gorm"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	设置仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/settings"
)

func 执行设置(事务 *GORM包.数据库, 执行上下文 上下文包.上下文) (设置包.快照, error) {
	仓储, 错误 := 设置仓储包.新建仓储(事务)
	if 错误 != nil {
		return 设置包.快照{}, 错误
	}
	记录, 错误 := 仓储.获取(执行上下文, 设置包.执行键)
	if 错误 != nil {
		return 设置包.快照{}, 错误
	}
	快照, 错误 := 设置包.执行快照(记录)
	if 错误 != nil {
		return 设置包.快照{}, 错误
	}
	记录, 错误 = 仓储.获取(执行上下文, 设置包.压缩键)
	if 错误 != nil {
		return 设置包.快照{}, 错误
	}
	压缩, 错误 := 设置包.解码压缩快照(记录)
	if 错误 != nil {
		return 设置包.快照{}, 错误
	}
	快照.压缩 = &压缩
	MCP, 错误 := 仓储.获取(执行上下文, 设置包.MCP键)
	if 错误 != nil {
		return 设置包.快照{}, 错误
	}
	if _, 错误 := 设置包.解码MCP记录(MCP); 错误 != nil {
		return 设置包.快照{}, 错误
	}
	快照.MCP修订号 = MCP.修订号
	return 快照, nil
}

// 开始压缩 与消息发送使用相同的会话互斥和设置事务，不创建用户或回答消息。
func (当前仓储 *仓储) 开始压缩(执行上下文 上下文包.上下文, 输入 业务域包.开始) (输出 业务域包.执行, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		快照, 错误 := 执行设置(事务, 执行上下文)
		if 错误 != nil {
			return 错误
		}
		var 会话 表结构包.会话
		if 错误 := 事务.条件("id=?", 输入.会话标识).取出(&会话).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.未找到错误
			}
			return 错误
		}
		if 会话.已归档位于 != nil || 会话.删除时间 != nil {
			return 业务域包.非活动错误
		}
		if 错误 := 活动工作区(事务, 会话.工作区标识); 错误 != nil {
			return 错误
		}
		if 错误 := 繁忙(事务, 会话.标识); 错误 != nil {
			return 错误
		}
		输出.历史, 输出.上下文, 输出.历史更多, 错误 = 加载上下文历史(事务, 会话.标识, 0)
		if 错误 != nil {
			return 错误
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		行 := 表结构包.执行{标识: 输入.执行标识, 会话标识: 会话.标识, 模型: 输入.模型,
			流: false, 设置快照: &快照, 状态: 智能体包.运行中, 创建时间: 当前时间, 更新时间: 当前时间}
		if 错误 := 事务.省略("Conversation", "Replaces").创建(&行).错误; 错误 != nil {
			return 错误
		}
		输入, 错误 := JSON包.序列化(map[string]any{"purpose": "context_compaction", "trigger": map[string]string{"type": "manual", "reason": "user_request"}})
		if 错误 != nil {
			return 错误
		}
		步骤 := 表结构包.步骤{标识: 输入.步骤标识, 会话标识: 会话.标识, 执行标识: 行.标识, 序号: 1, 类型: "context_compression",
			名称: "压缩历史上下文", 状态: 智能体包.待处理, 输入: 输入, 创建时间: 当前时间, 更新时间: 当前时间}
		if 错误 := 事务.省略("Run", "Parent").创建(&步骤).错误; 错误 != nil {
			return 错误
		}
		输出.会话 = 转换(会话)
		输出.执行 = 智能体包.执行{标识: 行.标识, 会话标识: 会话.标识, 模型: 输入.模型, 设置快照: &快照, 状态: 智能体包.运行中, 创建时间: 当前时间, 更新时间: 当前时间}
		输出.步骤 = 智能体包.步骤{标识: 步骤.标识, 会话标识: 会话.标识, 执行标识: 行.标识, 序号: 1, 类型: 步骤.类型,
			名称: 步骤.名称, 状态: 步骤.状态, 输入: 输入, 创建时间: 当前时间, 更新时间: 当前时间}
		return nil
	})
	return 输出, 错误
}
```
