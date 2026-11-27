```go
package 会话

import (
	上下文包 "context"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	错误判断包 "errors"
	切片包 "slices"
	时间包 "time"

	GORM包 "gorm.io/gorm"

	业务域包 "github.com/xiaoyu98628/callum/internal/conversation"
	压缩包 "github.com/xiaoyu98628/callum/internal/conversation/compaction"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	智能体仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func 上下文值(当前上下文版本 表结构包.上下文版本) 业务域包.上下文版本 {
	return 业务域包.上下文版本{标识: 当前上下文版本.标识, 会话标识: 当前上下文版本.会话标识, 版本: 当前上下文版本.版本,
		基准版本标识: 当前上下文版本.基准版本标识, 已生成依据步骤标识: 当前上下文版本.已生成依据步骤标识, 通过序号: 当前上下文版本.通过序号,
		摘要: 当前上下文版本.摘要, 来源集: 当前上下文版本.来源集, 统计: 当前上下文版本.统计, 状态: 当前上下文版本.状态,
		创建时间: 当前上下文版本.创建时间, 更新时间: 当前上下文版本.更新时间, 已激活位于: 当前上下文版本.已激活位于, 已失效位于: 当前上下文版本.已失效位于}
}

func 活动上下文(事务 *GORM包.数据库, 标识 string) (*业务域包.上下文版本, error) {
	var 行 表结构包.上下文版本
	错误 := 事务.条件("conversation_id=? AND status='active'", 标识).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return nil, nil
	}
	if 错误 != nil {
		return nil, 错误
	}
	值 := 上下文值(行)
	if 值.摘要 == "" || 值.通过序号 < 1 || len(值.来源集) == 0 || 值.已激活位于 == nil {
		return nil, 模型包.无效响应错误
	}
	规范化, 错误 := 压缩包.校验(值.摘要)
	if 错误 != nil || 规范化 != 值.摘要 {
		return nil, 模型包.无效响应错误
	}
	var 最后 int64
	for _, 来源 := range 值.来源集 {
		if 来源.序号 <= 最后 || 来源.消息标识 == "" || 来源.执行标识 == "" || len(来源.指纹) != 64 {
			return nil, 模型包.无效响应错误
		}
		if _, 错误 := 十六进制包.解码字符串(来源.指纹); 错误 != nil {
			return nil, 模型包.无效响应错误
		}
		最后 = 来源.序号
	}
	if 最后 != 值.通过序号 {
		return nil, 模型包.无效响应错误
	}
	return &值, nil
}

func (当前仓储 *仓储) 活动上下文(执行上下文 上下文包.上下文, 标识 string) (*业务域包.上下文版本, error) {
	return 活动上下文(当前仓储.数据库.带有上下文(执行上下文), 标识)
}

func (当前仓储 *仓储) 上下文版本(执行上下文 上下文包.上下文, 会话标识, 标识 string) (业务域包.上下文版本, error) {
	var 行 表结构包.上下文版本
	错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("id=? AND conversation_id=?", 标识, 会话标识).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 业务域包.上下文版本{}, 业务域包.未找到错误
	}
	return 上下文值(行), 错误
}

func (当前仓储 *仓储) 上下文版本集(执行上下文 上下文包.上下文, 标识 string, 上限, 偏移 int) ([]业务域包.上下文版本, error) {
	if _, 错误 := 当前仓储.获取(执行上下文, 标识); 错误 != nil {
		return nil, 错误
	}
	var 行集 []表结构包.上下文版本
	// 列表只读元数据，避免一页携带多份摘要和来源清单。
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).省略("Summary", "Sources", "Stats").条件("conversation_id=?", 标识).
		顺序("version DESC").上限(上限).偏移(偏移).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]业务域包.上下文版本, len(行集))
	for 索引, 行 := range 行集 {
		输出[索引] = 上下文值(行)
	}
	return 输出, nil
}

func (当前仓储 *仓储) 创建上下文(执行上下文 上下文包.上下文, 输入 业务域包.上下文版本) (输出 业务域包.上下文版本, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		if 错误 := 检查上下文来源(事务, 输入); 错误 != nil {
			return 错误
		}
		var 版本 int64
		if 错误 := 事务.模型(&表结构包.上下文版本{}).条件("conversation_id=?", 输入.会话标识).选择("COALESCE(MAX(version),0)").扫描(&版本).错误; 错误 != nil {
			return 错误
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		行 := 表结构包.上下文版本{标识: 输入.标识, 会话标识: 输入.会话标识, 版本: 版本 + 1,
			基准版本标识: 输入.基准版本标识, 已生成依据步骤标识: 输入.已生成依据步骤标识, 通过序号: 输入.通过序号,
			来源集: 输入.来源集, 摘要: "", 状态: "building", 创建时间: 当前时间, 更新时间: 当前时间}
		if 错误 := 事务.省略("Conversation", "GeneratedByStep", "Base").创建(&行).错误; 错误 != nil {
			return 错误
		}
		输出 = 上下文值(行)
		return nil
	})
	return 输出, 错误
}

// 检查上下文来源 在激活前重查固定来源及所属活动执行；网络请求不持有该事务。
func 检查上下文来源(事务 *GORM包.数据库, 输入 业务域包.上下文版本) error {
	if len(输入.来源集) == 0 || len(输入.来源集) > 256 || 输入.通过序号 < 1 {
		return 业务域包.无效错误
	}
	var 步骤 表结构包.步骤
	if 错误 := 事务.关联("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id").
		条件("conversation_run_steps.id=? AND conversation_run_steps.conversation_id=? AND conversation_run_steps.type='context_compression' AND r.status='running'", 输入.已生成依据步骤标识, 输入.会话标识).取出(&步骤).错误; 错误 != nil {
		return 错误
	}
	当前, 错误 := 活动上下文(事务, 输入.会话标识)
	if 错误 != nil {
		return 错误
	}
	下限 := int64(0)
	if 当前 != nil {
		下限 = 当前.通过序号
	}
	if (当前 == nil) != (输入.基准版本标识 == nil) || (当前 != nil && 当前.标识 != *输入.基准版本标识) || 输入.通过序号 <= 下限 {
		return 业务域包.冲突错误
	}
	var 行集 []表结构包.消息
	if 错误 := 事务.条件("conversation_id=? AND sequence>? AND sequence<=? AND (role='user' OR status='completed')", 输入.会话标识, 下限, 输入.通过序号).顺序("sequence").上限(257).查找(&行集).错误; 错误 != nil {
		return 错误
	}
	if len(行集) != len(输入.来源集) {
		return 业务域包.冲突错误
	}
	仓储, 错误 := 智能体仓储包.新建仓储(事务)
	if 错误 != nil {
		return 错误
	}
	轨迹集, 错误 := 仓储.上下文执行轨迹(事务.语句.上下文, 输入.会话标识, 下限, 输入.通过序号+1)
	if 错误 != nil {
		return 错误
	}
	for 索引, 行 := range 行集 {
		来源 := 输入.来源集[索引]
		var 轨迹 []模型包.消息
		if 行.角色 == "user" {
			轨迹 = 轨迹集[行.执行标识]
		}
		哈希, 错误 := 压缩包.指纹(消息(行), 轨迹)
		if 错误 != nil {
			return 错误
		}
		if 来源.消息标识 != 行.标识 || 来源.执行标识 != 行.执行标识 || 来源.序号 != 行.序号 || 来源.指纹 != 哈希 {
			return 业务域包.冲突错误
		}
	}
	return nil
}

func (当前仓储 *仓储) 激活上下文(执行上下文 上下文包.上下文, 输入 业务域包.上下文版本) error {
	已校验, 错误 := 压缩包.校验(输入.摘要)
	if 错误 != nil || len(输入.统计) > 4096 || !JSON包.有效(输入.统计) {
		return 业务域包.无效错误
	}
	输入.摘要 = 已校验
	return 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		if 错误 := 检查上下文来源(事务, 输入); 错误 != nil {
			return 错误
		}
		var 行 表结构包.上下文版本
		if 错误 := 事务.条件("id=? AND status='building' AND conversation_id=?", 输入.标识, 输入.会话标识).取出(&行).错误; 错误 != nil {
			return 错误
		}
		// 摘要大小与原始来源固定在创建的版本上，不能借激活更换来源或父版本。
		if 行.已生成依据步骤标识 != 输入.已生成依据步骤标识 || 行.通过序号 != 输入.通过序号 || !切片包.相等(行.来源集, 输入.来源集) || (行.基准版本标识 == nil) != (输入.基准版本标识 == nil) || (行.基准版本标识 != nil && *行.基准版本标识 != *输入.基准版本标识) {
			return 业务域包.冲突错误
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		if 错误 := 事务.模型(&表结构包.上下文版本{}).条件("conversation_id=? AND status='active'", 输入.会话标识).
			更新集(map[string]any{"status": "superseded", "updated_at": 当前时间}).错误; 错误 != nil {
			return 错误
		}
		结果 := 事务.模型(&表结构包.上下文版本{}).条件("id=? AND status='building'", 输入.标识).
			更新集(map[string]any{"summary": 输入.摘要, "size_stats": string(输入.统计), "status": "active", "activated_at": 当前时间, "updated_at": 当前时间})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		输出, 错误 := JSON包.序列化(map[string]any{"compressed": true, "context_version_id": 输入.标识, "size_stats": 输入.统计})
		if 错误 != nil {
			return 错误
		}
		已结束 := 事务.模型(&表结构包.步骤{}).条件("id=? AND status='running'", 输入.已生成依据步骤标识).更新集(map[string]any{"status": "completed", "output": string(输出), "finished_at": 当前时间, "updated_at": 当前时间})
		if 已结束.错误 != nil {
			return 已结束.错误
		}
		if 已结束.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		return nil
	})
}

func (当前仓储 *仓储) 失败上下文(执行上下文 上下文包.上下文, 标识 string) error {
	return 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.上下文版本{}).条件("id=? AND status='building'", 标识).
		更新集(map[string]any{"status": "failed", "updated_at": 时间包.当前时间().Unix毫秒()}).错误
}

// 加载上下文历史 按完整用户轮次读取有界前缀，压缩后可从新的有效摘要边界继续。
func (当前仓储 *仓储) 加载上下文历史(执行上下文 上下文包.上下文, 标识 string, 之前 int64) ([]业务域包.消息, *业务域包.上下文版本, bool, error) {
	return 加载上下文历史(当前仓储.数据库.带有上下文(执行上下文), 标识, 之前)
}

func 加载上下文历史(事务 *GORM包.数据库, 标识 string, 之前 int64) ([]业务域包.消息, *业务域包.上下文版本, bool, error) {
	当前, 错误 := 活动上下文(事务, 标识)
	if 错误 != nil {
		return nil, nil, false, 错误
	}
	下限 := int64(0)
	if 当前 != nil {
		下限 = 当前.通过序号
	}
	行集, 更多, 错误 := 加载上下文页(事务, 标识, 下限, 之前)
	return 行集, 当前, 更多, 错误
}

func 加载上下文页(事务 *GORM包.数据库, 标识 string, 下限, 之前 int64) ([]业务域包.消息, bool, error) {
	查询 := func() *GORM包.数据库 {
		查询 := 事务.模型(&表结构包.消息{}).条件("conversation_id=? AND sequence>? AND (role='user' OR status='completed')", 标识, 下限)
		if 之前 > 0 {
			查询 = 查询.条件("sequence<?", 之前)
		}
		return 查询
	}
	// 先读取长度和边界，不把无界正文加载进进程；每页留出当前输入和模型封装空间。
	var 元数据 []struct {
		序号 int64
		角色 string
		大小 int64
	}
	if 错误 := 查询().选择("sequence,role,length(CAST(content AS BLOB)) AS size").顺序("sequence").上限(512).扫描(&元数据).错误; 错误 != nil {
		return nil, false, 错误
	}
	结束, 大小 := len(元数据), int64(0)
	for 索引, 行 := range 元数据 {
		大小 += 行.大小 + 512
		if 索引 >= 511 || 大小 > 12<<20 {
			结束 = 索引
			break
		}
	}
	更多 := 结束 < len(元数据)
	if 更多 {
		// 从下一个完整轮次开始留给后续页面；不能只加载一个回答的一半。
		for 结束 > 0 && 元数据[结束].角色 != "user" {
			结束--
		}
		if 结束 == 0 {
			return nil, false, &模型包.输入上限错误详情{尺寸: "单轮历史字节", 实际: 大小, 上限: 12 << 20}
		}
	}
	if 结束 == 0 {
		return nil, false, nil
	}
	var 行集 []表结构包.消息
	if 错误 := 查询().条件("sequence<=?", 元数据[结束-1].序号).顺序("sequence").查找(&行集).错误; 错误 != nil {
		return nil, false, 错误
	}
	输出 := make([]业务域包.消息, len(行集))
	for 索引, 行 := range 行集 {
		输出[索引] = 消息(行)
	}
	return 输出, 更多, nil
}

// 使失效上下文集 与重发事务一起清理失效摘要及其模型结果，不删除操作审计和实际 用量。
func 使失效上下文集(事务 *GORM包.数据库, 标识 string, 序号, 当前时间 int64) error {
	父级集 := 事务.模型(&表结构包.上下文版本{}).选择("generated_by_step_id").条件("conversation_id=? AND through_sequence>=? AND status IN ?", 标识, 序号, []string{"active", "superseded", "building"})
	if 错误 := 事务.模型(&表结构包.步骤{}).条件("type='model_call' AND parent_step_id IN (?)", 父级集).更新("output", nil).错误; 错误 != nil {
		return 错误
	}
	return 事务.模型(&表结构包.上下文版本{}).条件("conversation_id=? AND through_sequence>=? AND status IN ?", 标识, 序号, []string{"active", "superseded", "building"}).更新集(map[string]any{"status": "invalid", "invalidated_at": 当前时间, "updated_at": 当前时间, "summary": ""}).错误
}
```
