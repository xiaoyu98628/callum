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
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

var 活动后续消息执行集 = []string{智能体包.运行中, 智能体包.等待中审批, 智能体包.等待中用户输入}

func 后续后续查看(行 表结构包.后续消息) 业务域包.后续消息 {
	return 业务域包.后续消息{标识: 行.标识, 会话标识: 行.会话标识, 预期执行标识: 行.预期执行标识,
		模型标识: 行.模型标识, 状态: 行.状态, 内容: 行.内容, 位置: 行.位置,
		修订号: 行.修订号, 已生效步骤标识: 行.已生效步骤标识, 创建时间: 行.创建时间}
}

type 引导行 struct {
	表结构包.后续消息
	锚点序号 int64
	之后步骤 int64
	生效时间 *int64
}

func (行 引导行) 消息() 业务域包.消息 {
	状态 := 智能体包.运行中
	var 已更新 int64 = 行.创建时间
	if 行.状态 == 业务域包.后续消息已生效 {
		状态 = 智能体包.已完成
		if 行.生效时间 != nil {
			已更新 = *行.生效时间
		}
	}
	return 业务域包.消息{标识: 行.标识, 会话标识: 行.会话标识, 执行标识: 行.预期执行标识,
		序号: 行.锚点序号, 引导顺序: 行.位置, 引导之后步骤: new(行.之后步骤),
		角色: "user", 内容: 行.内容,
		状态: 状态, 创建时间: 行.创建时间, 更新时间: 已更新, 已完成位于: 行.生效时间}
}

func 引导消息集查询(事务 *GORM包.数据库) *GORM包.数据库 {
	// 受理位置优先。旧记录仍可按首次携带引导的请求或实际采用引导的步骤恢复。
	return 事务.表("conversation_followups AS f").
		选择(`f.*, m.sequence AS anchor_sequence, s.finished_at AS applied_at,
			COALESCE(
				(SELECT value FROM json_each(root.input, '$.steering_receipts') WHERE key=f.id),
				(SELECT MIN(p.sequence) - 1 FROM conversation_run_steps p
				 WHERE p.run_id=f.expected_run_id AND p.type='model_call'
				 AND EXISTS (SELECT 1 FROM json_each(p.input, '$.steering_ids') WHERE value=f.id)),
				s.sequence - 1,
				(SELECT COALESCE(MAX(p.sequence), 0) FROM conversation_run_steps p WHERE p.run_id=f.expected_run_id)
			) AS after_step`).
		关联("JOIN conversation_messages m ON m.run_id=f.expected_run_id AND m.conversation_id=f.conversation_id AND m.role='assistant'").
		关联("LEFT JOIN conversation_run_steps root ON root.run_id=f.expected_run_id AND root.sequence=1").
		关联("LEFT JOIN conversation_run_steps s ON s.id=f.applied_step_id AND s.run_id=f.expected_run_id").
		条件("f.status IN ?", []string{业务域包.后续消息引导, 业务域包.后续消息已生效})
}

// 接受引导消息 在受理事务中把当时的过程位置写入首个步骤。
// 待启动的步骤可能直接采用引导，应排在气泡之后。
func 接受引导消息(事务 *GORM包.数据库, 行 表结构包.后续消息) (业务域包.消息, error) {
	结果 := 事务.模型(&表结构包.步骤{}).
		条件("run_id=? AND conversation_id=? AND sequence=1", 行.预期执行标识, 行.会话标识).
		更新列("input", GORM包.表达式(`json_set(input, '$.steering_receipts',
			json_patch(COALESCE(json_extract(input, '$.steering_receipts'), '{}'), json_object(?,
				(SELECT COALESCE(MAX(p.sequence), 0) FROM conversation_run_steps p WHERE p.run_id=? AND p.status<>'pending')
			)))`, 行.标识, 行.预期执行标识))
	if 结果.错误 != nil {
		return 业务域包.消息{}, 结果.错误
	}
	if 结果.行集受影响 != 1 {
		return 业务域包.消息{}, 业务域包.后续消息变化通知错误
	}
	return 引导消息(事务, 行.会话标识, 行.标识)
}

func 引导消息(事务 *GORM包.数据库, 会话标识, 后续后续标识 string) (业务域包.消息, error) {
	var 行 引导行
	错误 := 引导消息集查询(事务).
		条件("f.conversation_id=? AND f.id=?", 会话标识, 后续后续标识).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 业务域包.消息{}, 业务域包.未找到错误
	}
	return 行.消息(), 错误
}

func 后续后续行集(当前数据库 *GORM包.数据库) ([]业务域包.后续消息, error) {
	var 行集 []表结构包.后续消息
	if 错误 := 当前数据库.上限(业务域包.后续消息上限 + 1).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	if len(行集) > 业务域包.后续消息上限 {
		return nil, 业务域包.后续消息上限错误
	}
	输出 := make([]业务域包.后续消息, 0, len(行集))
	for _, 行 := range 行集 {
		输出 = append(输出, 后续后续查看(行))
	}
	return 输出, nil
}

func (当前仓储 *仓储) 后续消息集(执行上下文 上下文包.上下文, 标识 string) ([]业务域包.后续消息, error) {
	return 后续后续行集(当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.后续消息{}).
		条件("conversation_id=? AND status IN ('queued','draft')", 标识).顺序("position, id"))
}

func (当前仓储 *仓储) 后续消息(执行上下文 上下文包.上下文, 标识, 项标识 string) (业务域包.后续消息, error) {
	var 行 表结构包.后续消息
	错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("conversation_id=? AND id=?", 标识, 项标识).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		错误 = 业务域包.未找到错误
	}
	return 后续后续查看(行), 错误
}

func 检查后续消息执行(事务 *GORM包.数据库, 标识, 执行标识 string, 引导 bool) error {
	var 数量 int64
	查询 := 事务.模型(&表结构包.执行{}).条件("id=? AND conversation_id=? AND status IN ?", 执行标识, 标识, 活动后续消息执行集).
		条件("EXISTS (SELECT 1 FROM conversations c WHERE c.id=? AND c.archived_at IS NULL AND c.deleted_at IS NULL)", 标识)
	if 引导 {
		查询 = 查询.条件("status=?", 智能体包.运行中).条件("EXISTS (SELECT 1 FROM conversation_run_steps s WHERE s.run_id=conversation_runs.id AND s.sequence=1 AND s.type='model_call')")
	}
	if 错误 := 查询.数量(&数量).错误; 错误 != nil {
		return 错误
	}
	if 数量 != 1 {
		return 业务域包.后续消息变化通知错误
	}
	return nil
}

// 检查后续消息内容 限制待发快照及单次执行的引导请求大小和数量。
func 检查后续消息内容(事务 *GORM包.数据库, 行 表结构包.后续消息, 写入中 bool) error {
	基准 := 事务.模型(&表结构包.后续消息{}).条件("conversation_id=? AND id<>?", 行.会话标识, 行.标识)
	检查 := func(当前数据库 *GORM包.数据库, 边界 int64) error {
		var 总计 struct{ 字节, 数量 int64 }
		if 错误 := 当前数据库.选择("COALESCE(SUM(length(CAST(content AS BLOB))),0) AS bytes, COUNT(*) AS count").扫描(&总计).错误; 错误 != nil {
			return 错误
		}
		原始, 错误 := JSON包.序列化(行.内容)
		if 错误 != nil {
			return 错误
		}
		if 总计.数量 >= 业务域包.后续消息上限 || 总计.字节+int64(len(原始)) > 边界 {
			return 业务域包.后续消息上限错误
		}
		return nil
	}
	if 写入中 {
		if 错误 := 检查(基准.会话连接(&GORM包.会话连接{}).条件("status IN ('queued','draft','steering')"), 3<<20); 错误 != nil {
			return 错误
		}
	}
	if 行.状态 == 业务域包.后续消息引导 {
		return 检查(基准.条件("expected_run_id=? AND status IN ('steering','applied')", 行.预期执行标识), 2<<20)
	}
	return nil
}

func (当前仓储 *仓储) 创建后续消息(执行上下文 上下文包.上下文, 输入 业务域包.后续消息) (业务域包.后续消息, error) {
	var 回执 *业务域包.消息
	行 := 表结构包.后续消息{标识: 输入.标识, 会话标识: 输入.会话标识, 预期执行标识: 输入.预期执行标识,
		模型标识: 输入.模型标识, 状态: 输入.状态, 内容: 输入.内容, 修订号: 1, 创建时间: 时间包.当前时间().Unix毫秒()}
	错误 := 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		if 错误 := 检查后续消息执行(事务, 行.会话标识, 行.预期执行标识, 行.状态 == 业务域包.后续消息引导); 错误 != nil {
			return 错误
		}
		if 错误 := 检查后续消息内容(事务, 行, true); 错误 != nil {
			return 错误
		}
		var 位置 int64
		if 错误 := 事务.模型(&表结构包.后续消息{}).条件("conversation_id=? AND (status IN ('queued','steering') OR (expected_run_id=? AND status='applied'))", 行.会话标识, 行.预期执行标识).
			选择("COALESCE(MAX(position), 0)").扫描(&位置).错误; 错误 != nil {
			return 错误
		}
		行.位置 = 位置 + 1
		if 错误 := 事务.省略("Conversation", "Run").创建(&行).错误; 错误 != nil {
			return 错误
		}
		if 行.状态 == 业务域包.后续消息引导 {
			消息, 错误 := 接受引导消息(事务, 行)
			if 错误 != nil {
				return 错误
			}
			回执 = &消息
		}
		return nil
	})
	输出 := 后续后续查看(行)
	输出.回执 = 回执
	return 输出, 错误
}

func (当前仓储 *仓储) 变更后续消息(执行上下文 上下文包.上下文, 输入 业务域包.后续消息变更, 内容 []模型包.内容块) (输出 业务域包.后续消息, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.后续消息
		if 错误 := 事务.条件("conversation_id=? AND id=?", 输入.会话标识, 输入.标识).取出(&行).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.后续消息变化通知错误
			}
			return 错误
		}
		if 输入.动作 == "withdraw" && 行.状态 == 业务域包.后续消息已发送 {
			return 业务域包.后续消息已发送错误
		}
		预期 := 业务域包.后续消息已排队
		if 输入.动作 == "queue_draft" || 输入.动作 == "steer_draft" || 输入.动作 == "restore_draft" {
			预期 = 业务域包.后续消息草稿
		}
		if 行.状态 != 预期 || 行.修订号 != 输入.修订号 {
			return 业务域包.后续消息变化通知错误
		}
		switch 输入.动作 {
		case "withdraw":
			var 数量 int64
			if 错误 := 事务.模型(&表结构包.后续消息{}).条件("conversation_id=? AND status=?", 行.会话标识, 业务域包.后续消息草稿).数量(&数量).错误; 错误 != nil {
				return 错误
			}
			if 数量 != 0 {
				return 业务域包.后续消息草稿错误
			}
			行.状态 = 业务域包.后续消息草稿
		case "steer", "steer_draft", "queue_draft", "restore_draft":
			引导 := 输入.动作 == "steer" || 输入.动作 == "steer_draft"
			if 输入.预期执行标识 != "" {
				if 错误 := 检查后续消息执行(事务, 行.会话标识, 输入.预期执行标识, 引导); 错误 != nil {
					return 错误
				}
				行.预期执行标识 = 输入.预期执行标识
			}
			var 尾部 int64
			if 错误 := 事务.模型(&表结构包.后续消息{}).条件("conversation_id=? AND (status IN ('queued','steering') OR (expected_run_id=? AND status='applied'))", 行.会话标识, 行.预期执行标识).
				选择("COALESCE(MAX(position),0)").扫描(&尾部).错误; 错误 != nil {
				return 错误
			}
			行.状态, 行.位置 = 业务域包.后续消息已排队, 尾部+1
			if 引导 {
				行.状态 = 业务域包.后续消息引导
			}
			if 输入.文本 != nil {
				行.内容 = 内容
			}
			if 输入.模型标识 != "" {
				行.模型标识 = 输入.模型标识
			}
		case "remove":
			行.状态 = 业务域包.后续消息已移除
			行.内容 = []模型包.内容块{}
		case "edit":
			行.内容 = 内容
		case "up", "down":
			var 其他 表结构包.后续消息
			查询 := 事务.条件("conversation_id=? AND status=?", 行.会话标识, 业务域包.后续消息已排队)
			if 输入.动作 == "up" {
				查询 = 查询.条件("position<?", 行.位置).顺序("position DESC")
			} else {
				查询 = 查询.条件("position>?", 行.位置).顺序("position")
			}
			if 错误 := 查询.取出(&其他).错误; 错误 != nil {
				if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
					return 业务域包.后续消息变化通知错误
				}
				return 错误
			}
			行.位置, 其他.位置 = 其他.位置, 行.位置
			其他.修订号++
			if 错误 := 事务.模型(&其他).选择("Position", "Revision").更新集(&其他).错误; 错误 != nil {
				return 错误
			}
		default:
			return 业务域包.无效错误
		}
		if 输入.文本 != nil || 行.状态 == 业务域包.后续消息引导 {
			if 错误 := 检查后续消息内容(事务, 行, 输入.文本 != nil); 错误 != nil {
				return 错误
			}
		}
		行.修订号++
		输出 = 后续后续查看(行)
		if 错误 := 事务.模型(&行).省略("Conversation", "Run").选择("Status", "Content", "Revision", "Position", "ExpectedRunID", "ModelID").更新集(&行).错误; 错误 != nil {
			return 错误
		}
		if 行.状态 == 业务域包.后续消息引导 {
			消息, 错误 := 接受引导消息(事务, 行)
			if 错误 != nil {
				return 错误
			}
			输出.回执 = &消息
		}
		return nil
	})
	return
}

// 待处理引导 按受理顺序提取还未用于实际模型请求的消息。
func (当前仓储 *仓储) 待处理引导(执行上下文 上下文包.上下文, 执行标识 string) ([]业务域包.后续消息, error) {
	return 后续后续行集(当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.后续消息{}).
		条件("expected_run_id=? AND status=?", 执行标识, 业务域包.后续消息引导).顺序("position, id"))
}

func (当前仓储 *仓储) 应用引导(执行上下文 上下文包.上下文, 执行标识, 步骤标识 string, 标识集 []string) ([]业务域包.消息, error) {
	if len(标识集) == 0 {
		return nil, nil
	}
	消息集 := make([]业务域包.消息, 0, len(标识集))
	错误 := 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		结果 := 事务.模型(&表结构包.后续消息{}).条件("expected_run_id=? AND status=? AND id IN ?", 执行标识, 业务域包.后续消息引导, 标识集).
			更新集(map[string]any{"status": 业务域包.后续消息已生效, "applied_step_id": 步骤标识, "revision": GORM包.表达式("revision+1")})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != int64(len(标识集)) {
			return 业务域包.后续消息变化通知错误
		}
		var 行集 []引导行
		if 错误 := 引导消息集查询(事务).
			条件("f.expected_run_id=? AND f.id IN ?", 执行标识, 标识集).顺序("f.position, f.id").查找(&行集).错误; 错误 != nil {
			return 错误
		}
		if len(行集) != len(标识集) {
			return 业务域包.后续消息变化通知错误
		}
		for _, 行 := range 行集 {
			消息集 = append(消息集, 行.消息())
		}
		return nil
	})
	return 消息集, 错误
}

func 消费后续消息(事务 *GORM包.数据库, 输入 业务域包.开始) error {
	请求 := 输入.后续消息
	var 头部 表结构包.后续消息
	查询 := 事务.条件("conversation_id=?", 输入.会话标识)
	if 请求.文本 != nil {
		查询 = 查询.条件("id=? AND status=?", 请求.标识, 业务域包.后续消息草稿)
	} else {
		查询 = 查询.条件("status=?", 业务域包.后续消息已排队).顺序("position, id")
	}
	错误 := 查询.取出(&头部).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) || (错误 == nil && (头部.标识 != 请求.标识 || 头部.修订号 != 请求.修订号)) {
		return 业务域包.后续消息变化通知错误
	}
	if 错误 != nil {
		return 错误
	}
	if 请求.之后执行标识 != "" {
		var 前一个 表结构包.执行
		if 错误 := 事务.模型(&表结构包.执行{}).选择("conversation_runs.*").
			关联("JOIN conversation_messages m ON m.run_id=conversation_runs.id AND m.role='user'").
			条件("conversation_runs.conversation_id=?", 输入.会话标识).顺序("m.sequence DESC").取出(&前一个).错误; 错误 != nil {
			return 错误
		}
		if 前一个.标识 != 请求.之后执行标识 || 前一个.状态 != 智能体包.已完成 || 头部.预期执行标识 != 前一个.标识 {
			return 业务域包.后续消息变化通知错误
		}
	}
	if 错误 := 事务.模型(&头部).更新集(map[string]any{"status": 业务域包.后续消息已发送, "content": "[]", "revision": GORM包.表达式("revision+1")}).错误; 错误 != nil {
		return 错误
	}
	return nil
}

// 绑定后续消息队列 在新执行创建后，将剩余队列关联到该执行。
func 绑定后续消息队列(事务 *GORM包.数据库, 输入 业务域包.开始) error {
	if 输入.后续消息 == nil {
		return nil
	}
	return 事务.模型(&表结构包.后续消息{}).条件("conversation_id=? AND status=?", 输入.会话标识, 业务域包.后续消息已排队).
		更新集(map[string]any{"expected_run_id": 输入.执行标识, "revision": GORM包.表达式("revision+1")}).错误
}
```
