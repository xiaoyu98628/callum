```go
package 智能体

import (
	上下文包 "context"
	JSON包 "encoding/json"
	切片包 "slices"

	GORM包 "gorm.io/gorm"

	业务域包 "github.com/xiaoyu98628/callum/internal/agent"
	执行轨迹包 "github.com/xiaoyu98628/callum/internal/agent/transcript"
	历史包 "github.com/xiaoyu98628/callum/internal/conversation/history"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

type 执行轨迹行 struct {
	标识, 执行标识, 执行状态 string
	父级步骤标识, 工具调用标识 *string
	消息             string
	序号             int64
	引导             bool
	操作             *string
}

// 执行轨迹 恢复当前分支的成功轨迹、已被显式继续的轨迹及失败执行留下的副作用事实。
// 查询前检查累计大小，只加载统一上下文，不加载大型原始模型结果。
func (当前仓储 *仓储) 执行轨迹(执行上下文 上下文包.上下文, 标识 string, 之后 ...int64) (map[string][]模型包.消息, error) {
	下限 := int64(0)
	if len(之后) > 0 {
		下限 = 之后[0]
	}
	return 当前仓储.读取执行轨迹(执行上下文, 标识, 下限, 8<<20, 256, 0, "")
}

// 上下文执行轨迹 为压缩准备读取有界的未覆盖轨迹，最终模型请求仍遵守原上限。
func (当前仓储 *仓储) 上下文执行轨迹(执行上下文 上下文包.上下文, 标识 string, 下限 int64, 之前 ...int64) (map[string][]模型包.消息, error) {
	上界 := int64(0)
	if len(之前) > 0 {
		上界 = 之前[0]
	}
	return 当前仓储.读取执行轨迹(执行上下文, 标识, 下限, 16<<20, 512, 上界, "")
}

// 继续执行轨迹 读取当前执行已保存的轨迹，包含已生效的引导和已有的压缩检查点。
func (当前仓储 *仓储) 继续执行轨迹(执行上下文 上下文包.上下文, 会话标识, 执行标识 string) ([]模型包.消息, error) {
	轨迹集, 错误 := 当前仓储.读取执行轨迹(执行上下文, 会话标识, 0, 16<<20, 512, 0, 执行标识)
	return 轨迹集[执行标识], 错误
}

func 继续步骤集(数据库 *GORM包.数据库) *GORM包.数据库 {
	return 数据库.模型(&表结构包.步骤{}).条件("type='model_call' AND json_type(input,'$.resume')='object'")
}

func (当前仓储 *仓储) 读取执行轨迹(执行上下文 上下文包.上下文, 标识 string, 下限 int64, 最大字节 int64, 最大消息集 int, 之前 int64, 继续执行标识 string) (map[string][]模型包.消息, error) {
	基准 := func() *GORM包.数据库 {
		查询 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).
			关联("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id").
			关联("JOIN conversation_messages m ON m.run_id=r.id AND m.role='user'").
			条件("conversation_run_steps.conversation_id=? AND m.conversation_id=? AND m.sequence>?", 标识, 标识, 下限)
		if 继续执行标识 != "" {
			return 查询.条件("r.id=? AND r.status='running'", 继续执行标识)
		}
		查询 = 查询.条件("r.status IN ('completed','failed','cancelled','interrupted')")
		if 之前 > 0 {
			查询 = 查询.条件("m.sequence<?", 之前)
		}
		return 查询
	}
	// 新步骤缺失事实属于损坏记录，不能沿用旧契约而静默丢失操作。
	var 无效 int64
	if 错误 := 基准().条件(`conversation_run_steps.type='tool_call'
		AND json_extract(conversation_run_steps.input,'$.operation_version')=1 AND (
			COALESCE(json_extract(conversation_run_steps.output,'$.operation.version'),0)<>1
			OR COALESCE(json_extract(conversation_run_steps.output,'$.operation.outcome'),'') NOT IN ('none','not_applied','applied','partially_applied','unknown')
			OR COALESCE(json_type(conversation_run_steps.output,'$.context_message'),'')<>'object'
		)`).数量(&无效).错误; 错误 != nil {
		return nil, 错误
	}
	if 无效 != 0 {
		return nil, 模型包.无效响应错误
	}
	可见 := func() *GORM包.数据库 {
		if 继续执行标识 != "" {
			return 基准()
		}
		影响集 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).表("conversation_run_steps AS t").选择("1").
			条件("t.run_id=r.id AND t.type=?", "tool_call").
			条件("json_extract(t.input,'$.operation_version')=1 AND json_extract(t.output,'$.operation.version')=1").
			条件("json_extract(t.output,'$.operation.outcome') IN ?", []string{"applied", "partially_applied", "unknown"})
		已继续 := 当前仓储.数据库.带有上下文(执行上下文).表("conversation_messages AS u").选择("1").
			关联("JOIN conversation_run_steps AS next ON next.run_id=u.run_id AND next.sequence=1").
			条件("u.conversation_id=r.conversation_id AND u.role='user' AND u.sequence>m.sequence").
			条件("json_extract(next.input,'$.continued_from_run_id')=r.id")
		已继续 := 继续步骤集(当前仓储.数据库.带有上下文(执行上下文)).选择("1").条件("run_id=r.id")
		return 基准().条件("(r.status=? OR EXISTS (?) OR EXISTS (?) OR EXISTS (?))", "completed", 影响集, 已继续, 已继续)
	}
	var 执行标识集 []string
	if 错误 := 可见().不同("r.id").上限(最大消息集+1).提取列("r.id", &执行标识集).错误; 错误 != nil {
		return nil, 错误
	}
	if len(执行标识集) > 最大消息集 {
		return nil, &模型包.输入上限错误详情{尺寸: "历史执行数量", 实际: int64(len(执行标识集)), 上限: int64(最大消息集)}
	}
	var 边界集 []struct {
		执行标识 string
		序号   int64
	}
	if len(执行标识集) > 0 {
		if 错误 := 继续步骤集(当前仓储.数据库.带有上下文(执行上下文)).条件("run_id IN ?", 执行标识集).
			选择("run_id,MAX(sequence) AS sequence").组("run_id").扫描(&边界集).错误; 错误 != nil {
			return nil, 错误
		}
	}
	已继续位于 := make(map[string]int64, len(边界集))
	for _, 行 := range 边界集 {
		已继续位于[行.执行标识] = 行.序号
	}
	检查点集, 错误 := 当前仓储.读取检查点集(执行上下文, 标识, 执行标识集, 最大字节)
	if 错误 != nil {
		return nil, 错误
	}
	查询 := func() *GORM包.数据库 {
		查询 := 可见().条件("json_type(conversation_run_steps.output,'$.context_message')='object'")
		// 已验证的固定范围直接参与查询，不对每条消息再关联扫描全部压缩步骤。
		for 执行标识, 检查点 := range 检查点集 {
			查询 = 查询.条件("(conversation_run_steps.run_id<>? OR conversation_run_steps.sequence>?)", 执行标识, 检查点.通过)
		}
		return 查询
	}
	var 边界 struct{ 大小, 数量 int64 }
	if 错误 := 查询().选择("COALESCE(SUM(length(CAST(json_extract(conversation_run_steps.output,'$.context_message') AS BLOB))),0) AS size,COUNT(*) AS count").扫描(&边界).错误; 错误 != nil {
		return nil, 错误
	}
	for _, 检查点 := range 检查点集 {
		原始, 错误 := JSON包.序列化(检查点.消息())
		if 错误 != nil {
			return nil, 错误
		}
		边界.大小 += int64(len(原始))
		边界.数量++
	}
	引导 := func() *GORM包.数据库 {
		查询 := 当前仓储.数据库.带有上下文(执行上下文).表("conversation_followups AS f").
			关联("JOIN conversation_runs r ON r.id=f.expected_run_id").
			关联("JOIN conversation_messages m ON m.run_id=r.id AND m.role='user'").
			关联("JOIN conversation_run_steps s ON s.id=f.applied_step_id AND s.run_id=r.id").
			条件("f.conversation_id=? AND m.conversation_id=? AND m.sequence>? AND f.status='applied'", 标识, 标识, 下限)
		if 继续执行标识 != "" {
			return 查询.条件("r.id=? AND r.status='running'", 继续执行标识)
		}
		查询 = 查询.条件("r.finished_at IS NOT NULL")
		if 之前 > 0 {
			查询 = 查询.条件("m.sequence<?", 之前)
		}
		return 查询
	}
	var 引导边界 struct{ 大小, 数量 int64 }
	if 错误 := 引导().选择("COALESCE(SUM(length(CAST(f.content AS BLOB))),0) AS size, COUNT(*) AS count").扫描(&引导边界).错误; 错误 != nil {
		return nil, 错误
	}
	边界.大小 += 引导边界.大小
	边界.数量 += 引导边界.数量
	if 边界.大小 > 最大字节 {
		return nil, &模型包.输入上限错误详情{尺寸: "历史轨迹字节", 实际: 边界.大小, 上限: 最大字节}
	}
	if 边界.数量 > int64(最大消息集) {
		return nil, &模型包.输入上限错误详情{尺寸: "历史轨迹消息数量", 实际: 边界.数量, 上限: int64(最大消息集)}
	}
	var 行集 []执行轨迹行
	if 错误 := 查询().选择("conversation_run_steps.id,conversation_run_steps.run_id,r.status AS run_status,conversation_run_steps.sequence,conversation_run_steps.parent_step_id,conversation_run_steps.tool_call_id,json_extract(conversation_run_steps.output,'$.context_message') AS message,json_extract(conversation_run_steps.output,'$.operation') AS operation").
		顺序("r.created_at").顺序("r.id").顺序("conversation_run_steps.sequence").扫描(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	var 引导行集 []struct {
		标识, 执行标识, 执行状态, 内容 string
		序号                 int64
	}
	if 错误 := 引导().选择("f.id,r.id AS run_id,r.status AS run_status,f.content,s.sequence").
		顺序("f.position, f.id").扫描(&引导行集).错误; 错误 != nil {
		return nil, 错误
	}
	for _, 行 := range 引导行集 {
		var 内容 []模型包.内容块
		if 错误 := JSON包.反序列化([]byte(行.内容), &内容); 错误 != nil {
			return nil, 错误
		}
		原始, 错误 := JSON包.序列化(模型包.消息{角色: 模型包.角色用户, 内容块集: 历史包.模型内容(内容)})
		if 错误 != nil {
			return nil, 错误
		}
		行集 = append(行集, 执行轨迹行{标识: 行.标识, 执行标识: 行.执行标识, 执行状态: 行.执行状态, 序号: 行.序号, 引导: true, 消息: string(原始)})
	}
	// 相同序号时，用户引导位于实际使用该消息的模型响应之前。
	切片包.排序稳定函数(行集, func(当前执行轨迹行, 当前执行轨迹行 执行轨迹行) int {
		if 当前执行轨迹行.执行标识 != 当前执行轨迹行.执行标识 {
			if 当前执行轨迹行.执行标识 < 当前执行轨迹行.执行标识 {
				return -1
			}
			return 1
		}
		if 当前执行轨迹行.序号 != 当前执行轨迹行.序号 {
			if 当前执行轨迹行.序号 < 当前执行轨迹行.序号 {
				return -1
			}
			return 1
		}
		if 当前执行轨迹行.引导 == 当前执行轨迹行.引导 {
			return 0
		}
		if 当前执行轨迹行.引导 {
			return -1
		}
		return 1
	})
	输出 := map[string][]模型包.消息{}
	for 执行标识, 检查点 := range 检查点集 {
		输出[执行标识] = []模型包.消息{检查点.消息()}
	}
	数量 := len(检查点集)
	for 启动 := 0; 启动 < len(行集); {
		结束 := 启动 + 1
		for 结束 < len(行集) && 行集[结束].执行标识 == 行集[启动].执行标识 {
			结束++
		}
		执行标识 := 行集[启动].执行标识
		// 之前停止的模型批次可能只有部分工具结果。即使最终成功，也按中断规则恢复这些批次。
		拆分 := 启动
		for 拆分 < 结束 && 行集[拆分].序号 < 已继续位于[执行标识] {
			拆分++
		}
		已停止, 错误 := 恢复执行轨迹(业务域包.已取消, 行集[启动:拆分])
		if 错误 != nil {
			return nil, 错误
		}
		消息集, 错误 := 恢复执行轨迹(行集[启动].执行状态, 行集[拆分:结束])
		if 错误 != nil {
			return nil, 错误
		}
		数量 += len(已停止) + len(消息集)
		if 数量 > 最大消息集 {
			return nil, &模型包.输入上限错误详情{尺寸: "历史轨迹消息数量", 实际: int64(数量), 上限: int64(最大消息集)}
		}
		输出[执行标识] = append(append(输出[执行标识], 已停止...), 消息集...)
		启动 = 结束
	}
	已编码, 错误 := JSON包.序列化(输出)
	if 错误 != nil {
		return nil, 错误
	}
	if int64(len(已编码)) > 最大字节 {
		return nil, &模型包.输入上限错误详情{尺寸: "历史轨迹字节", 实际: int64(len(已编码)), 上限: 最大字节}
	}
	return 输出, nil
}

// 恢复执行轨迹 仅将查询投影转换为业务输入，恢复规则由 执行轨迹 统一实现。
func 恢复执行轨迹(状态 string, 行集 []执行轨迹行) ([]模型包.消息, error) {
	条目集 := make([]执行轨迹包.条目, len(行集))
	for 索引, 行 := range 行集 {
		条目集[索引] = 执行轨迹包.条目{步骤标识: 行.标识, 父级步骤标识: 行.父级步骤标识, 工具调用标识: 行.工具调用标识, 消息: JSON包.原始JSON消息(行.消息)}
		if 行.操作 != nil {
			条目集[索引].操作 = JSON包.原始JSON消息(*行.操作)
		}
	}
	return 执行轨迹包.恢复(状态, 条目集)
}
```
