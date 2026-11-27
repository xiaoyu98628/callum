```go
package 智能体包

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	时间包 "time"

	GORM包 "gorm.io/gorm"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

var _ 智能体包.审批仓储 = (*仓储)(nil)

func 审批(行 表结构包.审批) 智能体包.审批 {
	return 智能体包.审批{标识: 行.标识, 执行标识: 行.执行标识, 步骤标识: 行.步骤标识, 请求: 行.请求, 请求哈希: 行.请求哈希, 状态: 行.状态, 已决定依据: 行.已决定依据, 决定说明: 行.决定说明, 创建时间: 行.创建时间, 更新时间: 行.更新时间, 已决定位于: 行.已决定位于}
}
func (当前仓储 *仓储) 创建审批(执行上下文 上下文包.上下文, 输入 智能体包.审批) (输出 智能体包.审批, 错误 error) {
	校验和 := SHA256包.校验和256(输入.请求)
	if 十六进制包.编码目标字符串(校验和[:]) != 输入.请求哈希 {
		return 输出, 智能体包.审批无效错误
	}
	if 输入.状态 != 智能体包.审批待处理 && 输入.状态 != "" && 输入.状态 != 智能体包.审批已批准 && 输入.状态 != 智能体包.审批已拒绝 {
		return 输出, 智能体包.审批无效错误
	}
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 步骤 表结构包.步骤
		if 错误 := 事务.条件("id=? AND run_id=? AND type='tool_call' AND status='pending'", 输入.步骤标识, 输入.执行标识).取出(&步骤).错误; 错误 != nil {
			return 未找到(错误)
		}
		if 输入.状态 == 智能体包.审批已批准 || 输入.状态 == 智能体包.审批已拒绝 {
			if 错误 := 核验自动决定(事务, 输入, 步骤); 错误 != nil {
				return 错误
			}
		} else {
			输入.状态, 输入.已决定依据, 输入.决定说明, 输入.已决定位于 = 智能体包.审批待处理, nil, nil, nil
			if 错误 := 设置状态(事务, 输入.执行标识, 智能体包.等待中审批); 错误 != nil {
				return 错误
			}
		}
		if 错误 := 事务.模型(&表结构包.步骤{}).条件("id=? AND status='pending'", 输入.步骤标识).更新集(map[string]any{"status": 智能体包.等待中审批, "updated_at": 输入.更新时间}).错误; 错误 != nil {
			return 错误
		}
		行 := 表结构包.审批{标识: 输入.标识, 执行标识: 输入.执行标识, 步骤标识: 输入.步骤标识, 请求: 输入.请求, 请求哈希: 输入.请求哈希, 状态: 输入.状态, 已决定依据: 输入.已决定依据, 决定说明: 输入.决定说明, 已决定位于: 输入.已决定位于, 创建时间: 输入.创建时间, 更新时间: 输入.更新时间}
		if 错误 := 事务.省略("Run", "Step").创建(&行).错误; 错误 != nil {
			return 错误
		}
		var 错误 error
		输出, 错误 = 装饰审批(事务, 行)
		return 错误
	})
	return
}

// 自动决定必须来自同次执行中已保存、绑定同一工具和指纹的审核步骤。
func 核验自动决定(事务 *GORM包.数据库, 输入 智能体包.审批, 步骤 表结构包.步骤) error {
	if 输入.审核 == nil || 输入.已决定依据 == nil || *输入.已决定依据 != 智能体包.审批自动审核方 || 输入.决定说明 == nil || 输入.已决定位于 == nil {
		return 智能体包.审批无效错误
	}
	var 操作 struct {
		哈希 string    `json:"request_hash"`
		审核 智能体包.审批审核 `json:"approval_review"`
	}
	if JSON包.反序列化(步骤.输入, &操作) != nil || 操作.哈希 != 输入.请求哈希 || 操作.审核 != *输入.审核 {
		return 智能体包.审批无效错误
	}
	var 运行 表结构包.执行
	if 错误 := 事务.选择("settings_snapshot").条件("id=? AND status=?", 输入.执行标识, 智能体包.运行中).取出(&运行).错误; 错误 != nil {
		return 错误
	}
	if 运行.设置快照 == nil || !运行.设置快照.有效() || 运行.设置快照.审批审核方 != 策略包.自动审核 {
		return 智能体包.审批无效错误
	}
	var 审核 表结构包.步骤
	if 错误 := 事务.选择("input", "output").条件("id=? AND run_id=? AND parent_step_id=? AND type='model_call' AND status=?", 输入.审核.步骤标识, 输入.执行标识, 输入.步骤标识, 智能体包.已完成).取出(&审核).错误; 错误 != nil {
		return 错误
	}
	var 输入 struct {
		用途 string `json:"purpose"`
		哈希 string `json:"request_hash"`
	}
	var 决定 智能体包.审批审核
	if JSON包.反序列化(审核.输入, &输入) != nil || JSON包.反序列化(审核.输出, &决定) != nil || 输入.用途 != "approval_review" || 输入.哈希 != 输入.请求哈希 || 决定 != *输入.审核 || 决定.决定 != 输入.状态 || 决定.原因 != *输入.决定说明 {
		return 智能体包.审批无效错误
	}
	return nil
}

func (当前仓储 *仓储) 获取审批(执行上下文 上下文包.上下文, 标识 string) (输出 智能体包.审批, 错误 error) {
	// 决定与目录授权在同一事务提交，读取也使用同一快照，避免返回混合状态。
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.审批
		if 错误 := 事务.条件("id=?", 标识).取出(&行).错误; 错误 != nil {
			return 未找到(错误)
		}
		var 错误 error
		输出, 错误 = 装饰审批(事务, 行)
		return 错误
	})
	return
}

func (当前仓储 *仓储) 列表审批集(执行上下文 上下文包.上下文, 当前审批筛选 智能体包.审批筛选) (输出 []智能体包.审批, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		查询 := 事务
		if 当前审批筛选.执行标识 != "" {
			查询 = 查询.条件("run_id=?", 当前审批筛选.执行标识)
		}
		if 当前审批筛选.状态 != "" {
			查询 = 查询.条件("status=?", 当前审批筛选.状态)
		}
		var 行集 []表结构包.审批
		if 错误 := 查询.顺序("created_at DESC").顺序("id DESC").上限(当前审批筛选.上限).偏移(当前审批筛选.偏移).查找(&行集).错误; 错误 != nil {
			return 错误
		}
		输出 = make([]智能体包.审批, 0, len(行集))
		for _, 行 := range 行集 {
			值, 错误 := 装饰审批(事务, 行)
			if 错误 != nil {
				return 错误
			}
			输出 = append(输出, 值)
		}
		return nil
	})
	return
}
func (当前仓储 *仓储) 决定审批(执行上下文 上下文包.上下文, 标识, 状态 string, 依据, 说明 *string, 范围集 ...string) (输出 智能体包.审批, 错误 error) {
	范围 := 智能体包.审批一次
	if len(范围集) == 1 {
		范围 = 范围集[0]
	}
	if len(范围集) > 1 || (范围 != 智能体包.审批一次 && 范围 != 智能体包.审批会话目录) || (范围 != 智能体包.审批一次 && 状态 != 智能体包.审批已批准) || (状态 != 智能体包.审批已批准 && 状态 != 智能体包.审批已拒绝) {
		return 输出, 智能体包.审批无效错误
	}
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.审批
		if 错误 := 事务.条件("id=?", 标识).取出(&行).错误; 错误 != nil {
			return 未找到(错误)
		}
		var 错误 error
		输出, 错误 = 装饰审批(事务, 行)
		if 错误 != nil {
			return 错误
		}
		if 行.状态 == 状态 {
			已保存范围 := 输出.决定范围
			if 已保存范围 == "" {
				已保存范围 = 智能体包.审批一次
			}
			if 已保存范围 != 范围 || (行.已决定依据 != nil && *行.已决定依据 == 智能体包.审批自动审核方) {
				return 智能体包.冲突错误
			}
			return nil
		}
		if 行.状态 != 智能体包.审批待处理 {
			return 智能体包.冲突错误
		}
		var 活动 int64
		if 错误 := 事务.模型(&表结构包.执行{}).条件("id=? AND status=?", 行.执行标识, 智能体包.等待中审批).数量(&活动).错误; 错误 != nil {
			return 错误
		}
		if 活动 != 1 {
			return 智能体包.冲突错误
		}
		if 范围 == 智能体包.审批会话目录 {
			if 错误 := 保存目录授权(事务, 行, 输出.目录范围); 错误 != nil {
				return 错误
			}
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		行.状态, 行.已决定依据, 行.决定说明, 行.已决定位于, 行.更新时间 = 状态, 依据, 说明, &当前时间, 当前时间
		if 错误 := 事务.模型(&表结构包.审批{}).条件("id=? AND status='pending'", 标识).选择("Status", "DecidedBy", "DecisionNote", "DecidedAt", "UpdatedAt").更新集(&行).错误; 错误 != nil {
			return 错误
		}
		输出, 错误 = 装饰审批(事务, 行)
		return 错误
	})
	return
}

// 取消审批 只取消尚在等待的审批，已经作出的决定保留原值。
func (当前仓储 *仓储) 取消审批(执行上下文 上下文包.上下文, 标识 string) (输出 智能体包.审批, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.审批
		if 错误 := 事务.条件("id=?", 标识).取出(&行).错误; 错误 != nil {
			return 未找到(错误)
		}
		if 行.状态 == 智能体包.审批待处理 {
			当前时间 := 时间包.当前时间().Unix毫秒()
			行.状态, 行.更新时间, 行.已决定位于 = 智能体包.审批已取消, 当前时间, &当前时间
			if 错误 := 事务.模型(&表结构包.审批{}).条件("id=? AND status='pending'", 标识).选择("Status", "UpdatedAt", "DecidedAt").更新集(&行).错误; 错误 != nil {
				return 错误
			}
		}
		输出 = 审批(行)
		return nil
	})
	return
}
func 设置状态(事务 *GORM包.数据库, 标识, 状态 string) error {
	当前时间 := 时间包.当前时间().Unix毫秒()
	结果 := 事务.模型(&表结构包.执行{}).条件("id=? AND status IN ?", 标识, []string{智能体包.运行中, 智能体包.等待中审批}).更新集(map[string]any{"status": 状态, "updated_at": 当前时间})
	if 结果.错误 != nil {
		return 结果.错误
	}
	if 结果.行集受影响 != 1 {
		return 智能体包.冲突错误
	}
	return 事务.模型(&表结构包.消息{}).条件("run_id=? AND role='assistant' AND status IN ?", 标识, []string{智能体包.运行中, 智能体包.等待中审批}).更新集(map[string]any{"status": 状态, "updated_at": 当前时间}).错误
}
func (当前仓储 *仓储) 继续(执行上下文 上下文包.上下文, 标识 string) error {
	return 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error { return 设置状态(事务, 标识, 智能体包.运行中) })
}
func (当前仓储 *仓储) 开始工具(执行上下文 上下文包.上下文, 执行标识, 步骤标识, 审批标识, 哈希 string) (输出 智能体包.步骤, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.步骤
		if 错误 := 事务.条件("id=? AND run_id=? AND type='tool_call' AND status IN ?", 步骤标识, 执行标识, []string{智能体包.待处理, 智能体包.等待中审批}).取出(&行).错误; 错误 != nil {
			return 未找到(错误)
		}
		if 审批标识 != "" {
			var 甲 表结构包.审批
			if 错误 := 事务.条件("id=? AND run_id=? AND step_id=? AND status='approved' AND request_hash=?", 审批标识, 执行标识, 步骤标识, 哈希).取出(&甲).错误; 错误 != nil {
				return 智能体包.冲突错误
			}
		}
		if 行.状态 == 智能体包.等待中审批 && 审批标识 == "" {
			return 智能体包.冲突错误
		}
		if 错误 := 检查目录认领(事务, 行); 错误 != nil {
			return 错误
		}
		if 错误 := 设置状态(事务, 执行标识, 智能体包.运行中); 错误 != nil {
			return 错误
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		行.状态, 行.开始时间, 行.更新时间 = 智能体包.运行中, &当前时间, 当前时间
		结果 := 事务.模型(&表结构包.步骤{}).条件("id=? AND status IN ?", 步骤标识, []string{智能体包.待处理, 智能体包.等待中审批}).选择("Status", "StartedAt", "UpdatedAt").更新集(&行)
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 智能体包.冲突错误
		}
		输出 = 步骤(行)
		return nil
	})
	return
}

func 取消审批集(事务 *GORM包.数据库, 执行标识 string, 当前时间 int64) error {
	查询 := 事务.模型(&表结构包.审批{}).条件("status='pending'")
	if 执行标识 != "" {
		查询 = 查询.条件("run_id=?", 执行标识)
	}
	return 查询.更新集(map[string]any{"status": 智能体包.审批已取消, "updated_at": 当前时间, "decided_at": 当前时间}).错误
}
```
