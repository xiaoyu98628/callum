```go
package 会话

import (
	上下文包 "context"
	错误判断包 "errors"

	GORM包 "gorm.io/gorm"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// 上下文状态 在短事务中读取设置、有效摘要及消息边界，不计算 词元 或持有运行缓存。
func (当前仓储 *仓储) 上下文状态(执行上下文 上下文包.上下文, 标识 string) (输出 业务域包.上下文状态, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 错误 error
		输出.设置, 错误 = 执行设置(事务, 执行上下文)
		if 错误 != nil || 标识 == "" {
			return 错误
		}
		var 行 表结构包.会话
		if 错误 := 事务.条件("id=?", 标识).取出(&行).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.未找到错误
			}
			return 错误
		}
		if 行.删除时间 != nil || 行.已归档位于 != nil {
			return 业务域包.非活动错误
		}
		if 错误 := 活动工作区(事务, 行.工作区标识); 错误 != nil {
			return 错误
		}
		输出.会话 = 转换(行)
		输出.上下文, 错误 = 活动上下文(事务, 标识)
		if 错误 != nil {
			return 错误
		}
		if 错误 := 事务.模型(&表结构包.消息{}).条件("conversation_id=?", 标识).选择("COALESCE(MAX(sequence),0)").扫描(&输出.通过序号).错误; 错误 != nil {
			return 错误
		}
		if 错误 := 事务.模型(&表结构包.消息{}).条件("conversation_id=? AND role='user'", 标识).选择("run_id").顺序("sequence DESC").上限(1).扫描(&输出.最后用户执行标识).错误; 错误 != nil {
			return 错误
		}
		var 运行中 表结构包.执行
		错误 = 事务.条件("conversation_id=? AND status IN ?", 标识, []string{智能体包.运行中, 智能体包.等待中审批, 智能体包.等待中用户输入}).取出(&运行中).错误
		if 错误 == nil {
			输出.活动执行 = &智能体包.执行{标识: 运行中.标识, 会话标识: 运行中.会话标识, 模型: 运行中.模型, 设置快照: 运行中.设置快照, 状态: 运行中.状态}
		} else if !错误判断包.是否属于(错误, GORM包.记录未找到错误) {
			return 错误
		}
		var 实际 struct {
			词元   *int64
			模型标识 string
		}
		// 只读取当前消息分支的主模型调用；未知或旧记录保留空值，不解释厂商私有 用量。
		if 错误 := 事务.表("conversation_run_steps AS s").
			关联("JOIN conversation_runs AS r ON r.id=s.run_id").
			关联("JOIN conversation_messages AS m ON m.run_id=r.id AND m.role='user'").
			条件("s.conversation_id=? AND s.type='model_call' AND s.parent_step_id IS NULL AND (json_extract(s.input,'$.purpose')='answer' OR json_extract(s.input,'$.purpose') IS NULL)", 标识).
			选择("CASE WHEN json_valid(s.output) THEN json_extract(s.output,'$.context_usage.actual_input_tokens') END AS tokens, json_extract(r.model,'$.id') AS model_id").
			顺序("s.created_at DESC,s.id DESC").上限(1).扫描(&实际).错误; 错误 != nil {
			return 错误
		}
		输出.实际输入词元, 输出.实际模型标识 = 实际.词元, 实际.模型标识
		return nil
	})
	return 输出, 错误
}

func (当前仓储 *仓储) 上下文页(执行上下文 上下文包.上下文, 标识 string, 之后, 之前 int64) ([]业务域包.消息, bool, error) {
	return 加载上下文页(当前仓储.数据库.带有上下文(执行上下文), 标识, 之后, 之前)
}
```
