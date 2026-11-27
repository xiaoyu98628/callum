```go
package 智能体

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	错误判断包 "errors"
	时间包 "time"

	业务域包 "github.com/xiaoyu98628/callum/internal/agent"
	压缩包 "github.com/xiaoyu98628/callum/internal/conversation/compaction"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	GORM包 "gorm.io/gorm"
)

func 有效检查点(当前上下文检查点 业务域包.上下文检查点) bool {
	if 当前上下文检查点.版本 != 业务域包.上下文检查点版本 || 当前上下文检查点.标识 == "" || 当前上下文检查点.来源执行标识 == "" || 当前上下文检查点.会话标识 == "" || 当前上下文检查点.之后 < 0 || 当前上下文检查点.通过 <= 当前上下文检查点.之后 || len(当前上下文检查点.来源集) == 0 || len(当前上下文检查点.来源集) > 248 || len(当前上下文检查点.统计) > 4096 || !JSON包.有效(当前上下文检查点.统计) {
		return false
	}
	规范化, 错误 := 压缩包.校验(当前上下文检查点.摘要)
	if 错误 != nil || 规范化 != 当前上下文检查点.摘要 {
		return false
	}
	最后 := 当前上下文检查点.之后
	for _, 来源 := range 当前上下文检查点.来源集 {
		if 来源.步骤标识 == "" || 来源.通过 <= 最后 || len(来源.指纹) != 64 {
			return false
		}
		if _, 错误 := 十六进制包.解码字符串(来源.指纹); 错误 != nil {
			return false
		}
		最后 = 来源.通过
	}
	return 最后 == 当前上下文检查点.通过
}

func (当前仓储 *仓储) 最新检查点(执行上下文 上下文包.上下文, 会话标识, 执行标识 string) (*业务域包.上下文检查点, error) {
	var 行 struct{ 输出 string }
	错误 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).
		条件("conversation_id=? AND type='context_compression' AND status='completed' AND json_extract(output,'$.checkpoint.source_run_id')=?", 会话标识, 执行标识).
		顺序("CAST(json_extract(output,'$.checkpoint.through_sequence') AS INTEGER) DESC").上限(1).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return nil, nil
	}
	if 错误 != nil {
		return nil, 错误
	}
	return 解码检查点(行.输出, 会话标识, 执行标识)
}

func 解码检查点(输出, 会话标识, 执行标识 string) (*业务域包.上下文检查点, error) {
	var 信封 struct {
		检查点 业务域包.上下文检查点 `json:"checkpoint"`
	}
	if JSON包.反序列化([]byte(输出), &信封) != nil || !有效检查点(信封.检查点) || 信封.检查点.会话标识 != 会话标识 || (执行标识 != "" && 信封.检查点.来源执行标识 != 执行标识) {
		return nil, 模型包.无效响应错误
	}
	return &信封.检查点, nil
}

// 读取检查点集 一次查询每个来源执行的最新摘要，避免逐轮恢复产生 数量 次 JSON 扫描。
// 数据行逐条读取并检查累计字节，不能先把所有历史版本加载进内存。
func (当前仓储 *仓储) 读取检查点集(执行上下文 上下文包.上下文, 会话标识 string, 执行标识集 []string, 最大字节 int64) (输出 map[string]*业务域包.上下文检查点, 返回错误 error) {
	输出 = map[string]*业务域包.上下文检查点{}
	if len(执行标识集) == 0 {
		return 输出, nil
	}
	已排序 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).
		选择(`output,ROW_NUMBER() OVER (
			PARTITION BY json_extract(output,'$.checkpoint.source_run_id')
			ORDER BY CAST(json_extract(output,'$.checkpoint.through_sequence') AS INTEGER) DESC) AS ordinal`).
		条件("conversation_id=? AND type=? AND status=?", 会话标识, "context_compression", 业务域包.已完成).
		条件("json_extract(output,'$.checkpoint.source_run_id') IN ?", 执行标识集)
	行集, 错误 := 当前仓储.数据库.带有上下文(执行上下文).表("(?) AS checkpoints", 已排序).
		选择("output").条件("ordinal=?", 1).行集()
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 行集.关闭()) }()
	大小 := int64(0)
	for 行集.下一个() {
		var 输出 string
		if 错误 := 行集.扫描(&输出); 错误 != nil {
			return nil, 错误
		}
		检查点, 错误 := 解码检查点(输出, 会话标识, "")
		if 错误 != nil {
			return nil, 错误
		}
		原始, 错误 := JSON包.序列化(检查点.消息())
		if 错误 != nil {
			return nil, 错误
		}
		大小 += int64(len(原始))
		if 大小 > 最大字节 {
			return nil, &模型包.输入上限错误详情{尺寸: "历史摘要字节", 实际: 大小, 上限: 最大字节}
		}
		输出[检查点.来源执行标识] = 检查点
	}
	return 输出, 行集.获取错误()
}

type 检查点行 struct {
	标识, 执行标识, 状态   string
	序号             int64
	父级步骤标识, 工具调用标识 *string
	消息             string
	操作             *string
	操作版本           int
}

const 检查点列集 = "id,run_id,status,sequence,parent_step_id,tool_call_id,json_extract(output,'$.context_message') AS message,json_extract(output,'$.operation') AS operation,COALESCE(json_extract(input,'$.operation_version'),0) AS operation_version"

func (当前仓储 *仓储) 上下文批次(执行上下文 上下文包.上下文, 会话标识, 执行标识 string, 之后 int64) (*业务域包.上下文批次, error) {
	运行, 错误 := 当前仓储.获取(执行上下文, 执行标识)
	if 错误 != nil {
		return nil, 错误
	}
	if 运行.会话标识 != 会话标识 || 之后 < 0 {
		return nil, 业务域包.无效错误
	}
	var 已继续位于 int64
	if 错误 := 继续步骤集(当前仓储.数据库.带有上下文(执行上下文)).条件("run_id=?", 执行标识).选择("COALESCE(MAX(sequence),0)").扫描(&已继续位于).错误; 错误 != nil {
		return nil, 错误
	}
	if 运行.状态 != 业务域包.运行中 && 运行.状态 != 业务域包.已完成 && 已继续位于 == 0 {
		var 影响集 int64
		if 错误 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).条件("run_id=? AND type='tool_call' AND json_extract(input,'$.operation_version')=1 AND json_extract(output,'$.operation.outcome') IN ('applied','partially_applied','unknown')", 执行标识).数量(&影响集).错误; 错误 != nil {
			return nil, 错误
		}
		if 影响集 == 0 {
			return nil, nil
		}
	}
	var 父级 检查点行
	错误 = 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).选择(检查点列集).
		条件("run_id=? AND sequence>? AND type='model_call' AND status='completed' AND COALESCE(json_extract(input,'$.purpose'),'') IN ('','answer') AND json_type(output,'$.context_message')='object'", 执行标识, 之后).
		条件("sequence>=? OR json_array_length(output,'$.context_message.tool_calls')>0", 已继续位于).
		顺序("sequence").取出(&父级).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return nil, nil
	}
	if 错误 != nil {
		return nil, 错误
	}
	已停止 := 父级.序号 < 已继续位于
	实时 := 运行.状态 == 业务域包.运行中 && !已停止
	查询 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).条件("run_id=? AND parent_step_id=? AND type IN ('tool_call','user_input')", 执行标识, 父级.标识)
	var 待处理 int64
	if 错误 := 查询.会话连接(&GORM包.会话连接{}).条件("status IN ('pending','running','awaiting_approval','awaiting_user_input')").数量(&待处理).错误; 错误 != nil {
		return nil, 错误
	}
	if 待处理 > 0 && 实时 {
		return nil, nil
	}
	var 边界 struct{ 大小, 数量 int64 }
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).条件("id=? OR (run_id=? AND parent_step_id=? AND type IN ('tool_call','user_input'))", 父级.标识, 执行标识, 父级.标识).
		选择("COALESCE(SUM(length(CAST(json_extract(output,'$.context_message') AS BLOB))),0) AS size,COUNT(*) AS count").扫描(&边界).错误; 错误 != nil {
		return nil, 错误
	}
	if 边界.大小 > 7<<20 {
		return nil, &模型包.输入上限错误详情{尺寸: "单个工具批次字节", 实际: 边界.大小, 上限: 7 << 20}
	}
	if 边界.数量 > 248 {
		return nil, &模型包.输入上限错误详情{尺寸: "单个工具批次消息数量", 实际: 边界.数量, 上限: 248}
	}
	var 子项集 []检查点行
	if 错误 := 查询.选择(检查点列集).顺序("sequence").查找(&子项集).错误; 错误 != nil {
		return nil, 错误
	}
	行集 := append([]检查点行{父级}, 子项集...)
	原始, 错误 := JSON包.序列化(行集)
	if 错误 != nil {
		return nil, 错误
	}
	摘要 := SHA256包.校验和256(原始)
	状态 := 运行.状态
	if 已停止 {
		状态 = 业务域包.已取消
	} else if 实时 {
		状态 = 业务域包.已完成
	}
	已投影 := make([]执行轨迹行, 0, len(行集))
	通过 := 父级.序号
	for _, 行 := range 行集 {
		通过 = max(通过, 行.序号)
		if 行.操作版本 == 工具包.操作版本 {
			var 事实集 工具包.操作结果记录
			if 行.操作 == nil || JSON包.反序列化([]byte(*行.操作), &事实集) != nil || !事实集.有效() {
				return nil, 模型包.无效响应错误
			}
		}
		if 行.消息 == "" {
			if 实时 {
				return nil, nil
			}
			continue
		}
		已投影 = append(已投影, 执行轨迹行{标识: 行.标识, 执行标识: 行.执行标识, 执行状态: 状态, 父级步骤标识: 行.父级步骤标识, 工具调用标识: 行.工具调用标识, 消息: 行.消息, 操作: 行.操作})
	}
	if 实时 {
		var 消息 模型包.消息
		if JSON包.反序列化([]byte(父级.消息), &消息) != nil {
			return nil, 模型包.无效响应错误
		}
		已找到 := map[string]bool{}
		for _, 行 := range 子项集 {
			if 行.工具调用标识 != nil && 行.消息 != "" {
				已找到[*行.工具调用标识] = true
			}
		}
		for _, 调用 := range 消息.工具调用集 {
			if !已找到[调用.标识] {
				return nil, nil
			}
		}
	}
	消息集, 错误 := 恢复执行轨迹(状态, 已投影)
	if 错误 != nil {
		return nil, 错误
	}
	if len(消息集) > 248 {
		return nil, &模型包.输入上限错误详情{尺寸: "单个工具批次消息数量", 实际: int64(len(消息集)), 上限: 248}
	}
	return &业务域包.上下文批次{步骤标识: 父级.标识, 通过: 通过, 指纹: 十六进制包.编码目标字符串(摘要[:]), 消息集: 消息集}, nil
}

func (当前仓储 *仓储) 保存检查点(执行上下文 上下文包.上下文, 当前上下文检查点 业务域包.上下文检查点) error {
	if !有效检查点(当前上下文检查点) {
		return 业务域包.无效错误
	}
	return 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		存储 := &仓储{数据库: 事务}
		当前, 错误 := 存储.最新检查点(执行上下文, 当前上下文检查点.会话标识, 当前上下文检查点.来源执行标识)
		if 错误 != nil {
			return 错误
		}
		if (当前 == nil && (当前上下文检查点.基准标识 != "" || 当前上下文检查点.之后 != 0)) || (当前 != nil && (当前.标识 != 当前上下文检查点.基准标识 || 当前.通过 != 当前上下文检查点.之后)) {
			return 业务域包.冲突错误
		}
		游标 := 当前上下文检查点.之后
		for _, 来源 := range 当前上下文检查点.来源集 {
			批次, 错误 := 存储.上下文批次(执行上下文, 当前上下文检查点.会话标识, 当前上下文检查点.来源执行标识, 游标)
			if 错误 != nil {
				return 错误
			}
			if 批次 == nil || 批次.批次来源 != 来源 {
				return 业务域包.冲突错误
			}
			游标 = 批次.通过
		}
		var 父级 表结构包.步骤
		if 错误 := 事务.关联("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id").条件("conversation_run_steps.id=? AND conversation_run_steps.conversation_id=? AND conversation_run_steps.type='context_compression' AND conversation_run_steps.status='running' AND r.status='running'", 当前上下文检查点.标识, 当前上下文检查点.会话标识).取出(&父级).错误; 错误 != nil {
			return 错误
		}
		输出, 错误 := JSON包.序列化(map[string]any{"compressed": true, "checkpoint": 当前上下文检查点})
		if 错误 != nil {
			return 错误
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		结果 := 事务.模型(&表结构包.步骤{}).条件("id=? AND status='running'", 当前上下文检查点.标识).更新集(map[string]any{"output": string(输出), "status": 业务域包.已完成, "finished_at": 当前时间, "updated_at": 当前时间})
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		return nil
	})
}
```
