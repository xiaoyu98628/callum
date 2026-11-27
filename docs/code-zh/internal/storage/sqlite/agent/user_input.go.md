```go
package 智能体

import (
	上下文包 "context"
	反射包 "reflect"
	时间包 "time"

	业务域包 "github.com/xiaoyu98628/callum/internal/agent"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

var _ 业务域包.用户输入仓储 = (*仓储)(nil)

// 创建用户输入 在同一事务中冻结问题并将执行、回答标记为等待输入。
func (当前仓储 *仓储) 创建用户输入(执行上下文 上下文包.上下文, 输入 业务域包.步骤) (输出 业务域包.步骤, 错误 error) {
	if 输入.类型 != "user_input" || 输入.状态 != 业务域包.等待中用户输入 {
		return 输出, 业务域包.无效错误
	}
	if _, 错误 := 业务域包.解析输入问题(输入.输入); 错误 != nil {
		return 输出, 错误
	}
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 父级 表结构包.步骤
		if 输入.父级步骤标识 == nil || 输入.工具调用标识 == nil {
			return 业务域包.无效错误
		}
		if 错误 := 事务.条件("id=? AND run_id=? AND conversation_id=?", *输入.父级步骤标识, 输入.执行标识, 输入.会话标识).取出(&父级).错误; 错误 != nil {
			return 未找到(错误)
		}
		结果 := 事务.模型(&表结构包.执行{}).条件("id=? AND conversation_id=? AND status='running'", 输入.执行标识, 输入.会话标识).更新集(map[string]any{"status": 业务域包.等待中用户输入, "updated_at": 输入.更新时间})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		结果 = 事务.模型(&表结构包.消息{}).条件("run_id=? AND role='assistant' AND status='running'", 输入.执行标识).更新集(map[string]any{"status": 业务域包.等待中用户输入, "updated_at": 输入.更新时间})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		var 序号 int64
		if 错误 := 事务.模型(&表结构包.步骤{}).条件("run_id=?", 输入.执行标识).选择("COALESCE(MAX(sequence),0)").扫描(&序号).错误; 错误 != nil {
			return 错误
		}
		输入.序号 = 序号 + 1
		行 := 记录(输入)
		if 错误 := 事务.省略("Run", "Parent").创建(&行).错误; 错误 != nil {
			return 错误
		}
		输出 = 输入
		return nil
	})
	return
}

func (当前仓储 *仓储) 用户输入步骤集(执行上下文 上下文包.上下文, 执行标识 string, 待处理 bool, 上限, 偏移 int) ([]业务域包.步骤, error) {
	if 上限 < 1 || 上限 > 200 || 偏移 < 0 {
		return nil, 业务域包.无效错误
	}
	if _, 错误 := 当前仓储.获取(执行上下文, 执行标识); 错误 != nil {
		return nil, 错误
	}
	查询 := 当前仓储.数据库.带有上下文(执行上下文).条件("run_id=? AND type='user_input'", 执行标识)
	if 待处理 {
		查询 = 查询.条件("status=?", 业务域包.等待中用户输入)
	}
	var 行集 []表结构包.步骤
	if 错误 := 查询.顺序("sequence DESC").上限(上限).偏移(偏移).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]业务域包.步骤, 0, len(行集))
	for _, 行 := range 行集 {
		输出 = append(输出, 步骤(行))
	}
	return 输出, nil
}

// 回答用户输入 与取消事务竞争同一等待状态。相同回复幂等，不覆盖已有答案。
func (当前仓储 *仓储) 回答用户输入(执行上下文 上下文包.上下文, 标识, 执行标识 string, 回答 业务域包.输入回答) (输出 业务域包.步骤, 变化通知 bool, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.步骤
		if 错误 := 事务.条件("id=? AND run_id=? AND type='user_input'", 标识, 执行标识).取出(&行).错误; 错误 != nil {
			return 未找到(错误)
		}
		输出 = 步骤(行)
		问题, 错误 := 业务域包.解析输入问题(行.输入)
		if 错误 != nil {
			return 错误
		}
		if 错误 := 业务域包.校验输入回答(问题, 回答); 错误 != nil {
			return 错误
		}
		if 行.状态 == 业务域包.已完成 {
			值, 错误 := 业务域包.输入来源步骤(输出)
			if 错误 != nil {
				return 错误
			}
			if 反射包.深度相等(值.回答, &回答) {
				return nil
			}
			return 业务域包.冲突错误
		}
		if 行.状态 != 业务域包.等待中用户输入 {
			return 业务域包.冲突错误
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		结果 := 事务.模型(&表结构包.执行{}).条件("id=? AND status=?", 执行标识, 业务域包.等待中用户输入).更新集(map[string]any{"status": 业务域包.运行中, "updated_at": 当前时间})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		结果 = 事务.模型(&表结构包.消息{}).条件("run_id=? AND role='assistant' AND status=?", 执行标识, 业务域包.等待中用户输入).更新集(map[string]any{"status": 业务域包.运行中, "updated_at": 当前时间})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		输出, 错误 := 业务域包.输入回答输出(输出, 回答)
		if 错误 != nil {
			return 错误
		}
		输出.状态, 输出.输出, 输出.更新时间, 输出.结束时间 = 业务域包.已完成, 输出, 当前时间, &当前时间
		结果 = 事务.模型(&表结构包.步骤{}).条件("id=? AND status=?", 标识, 业务域包.等待中用户输入).更新集(map[string]any{"status": 业务域包.已完成, "output": string(输出), "updated_at": 当前时间, "finished_at": 当前时间})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		变化通知 = true
		return nil
	})
	return
}

func (当前仓储 *仓储) 取消用户输入(执行上下文 上下文包.上下文, 标识 string) (输出 业务域包.步骤, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.步骤
		if 错误 := 事务.条件("id=? AND type='user_input'", 标识).取出(&行).错误; 错误 != nil {
			return 未找到(错误)
		}
		if 行.状态 == 业务域包.等待中用户输入 {
			当前时间 := 时间包.当前时间().Unix毫秒()
			行.状态, 行.更新时间, 行.结束时间 = 业务域包.已取消, 当前时间, &当前时间
			if 错误 := 事务.模型(&表结构包.步骤{}).条件("id=? AND status=?", 标识, 业务域包.等待中用户输入).更新集(map[string]any{"status": 行.状态, "updated_at": 当前时间, "finished_at": 当前时间}).错误; 错误 != nil {
				return 错误
			}
		}
		输出 = 步骤(行)
		return nil
	})
	return
}
```
