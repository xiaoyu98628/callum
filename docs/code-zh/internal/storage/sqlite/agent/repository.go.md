```go
package 智能体

import (
	上下文包 "context"
	SQL包 "database/sql"
	JSON包 "encoding/json"
	错误判断包 "errors"
	时间包 "time"
	UTF8包 "unicode/utf8"

	GORM包 "gorm.io/gorm"

	业务域包 "github.com/xiaoyu98628/callum/internal/agent"
	执行轨迹包 "github.com/xiaoyu98628/callum/internal/agent/transcript"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 仓储 持久化运行状态，数据库由 应用模块 创建及关闭。
type 仓储 struct{ 数据库 *GORM包.数据库 }

var _ 业务域包.仓储 = (*仓储)(nil)

func 新建仓储(数据库 *GORM包.数据库) (*仓储, error) {
	if 数据库 == nil {
		return nil, 业务域包.无效错误
	}
	return &仓储{数据库: 数据库}, nil
}

func 未找到(错误 error) error {
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 业务域包.未找到错误
	}
	return 错误
}

func (当前仓储 *仓储) 获取(执行上下文 上下文包.上下文, 标识 string) (业务域包.执行, error) {
	var 行 执行行
	错误 := 带有继续时间(当前仓储.数据库.带有上下文(执行上下文)).条件("id = ?", 标识).取出(&行).错误
	return 执行(行), 未找到(错误)
}

func (当前仓储 *仓储) 列表(执行上下文 上下文包.上下文, 标识 string, 上限, 偏移 int) ([]业务域包.执行, error) {
	if 上限 < 1 || 上限 > 200 || 偏移 < 0 {
		return nil, 业务域包.无效错误
	}
	var 行集 []执行行
	if 错误 := 带有继续时间(当前仓储.数据库.带有上下文(执行上下文)).条件("conversation_id = ?", 标识).顺序("created_at DESC").顺序("id DESC").上限(上限).偏移(偏移).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]业务域包.执行, 0, len(行集))
	for _, 行 := range 行集 {
		输出 = append(输出, 执行(行))
	}
	return 输出, nil
}

func (当前仓储 *仓储) 获取步骤(执行上下文 上下文包.上下文, 标识 string) (业务域包.步骤, error) {
	var 行 表结构包.步骤
	错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("id = ?", 标识).取出(&行).错误
	return 步骤(行), 未找到(错误)
}

func (当前仓储 *仓储) 步骤集(执行上下文 上下文包.上下文, 标识 string, 之后 int64, 上限 int) ([]业务域包.步骤, error) {
	if 上限 < 1 || 上限 > 200 || 之后 < 0 {
		return nil, 业务域包.无效错误
	}
	if _, 错误 := 当前仓储.获取(执行上下文, 标识); 错误 != nil {
		return nil, 错误
	}
	var 行集 []表结构包.步骤
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("run_id = ? AND sequence > ?", 标识, 之后).顺序("sequence").上限(上限).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]业务域包.步骤, 0, len(行集))
	for _, 行 := range 行集 {
		输出 = append(输出, 步骤(行))
	}
	return 输出, nil
}

type 步骤摘要行 struct {
	表结构包.步骤
	输出字节 int
	用量字节 int
	定义字节 int
}

func (行 步骤摘要行) 摘要() 业务域包.步骤摘要 {
	return 业务域包.步骤摘要{步骤: 步骤(行.步骤), 输出字节: 行.输出字节,
		用量字节: 行.用量字节, 定义字节: 行.定义字节}
}

// JSON文本 与 JSON 仓储映射一致，去掉旧记录顶层 JSON 值周围的空白，保留值内部字节。
func JSON文本(列 string) string {
	return "trim(" + 列 + ", char(9, 10, 13, 32))"
}

func 步骤摘要查询(数据库 *GORM包.数据库) *GORM包.数据库 {
	return 数据库.模型(&表结构包.步骤{}).选择(
		"id", "conversation_id", "run_id", "parent_step_id", "sequence", "type", "name", "status", "input",
		"model", "provider_request_id", "tool_call_id", "error_kind", "error_message", "created_at", "started_at",
		"updated_at", "finished_at",
		"COALESCE(length(CAST("+JSON文本("output")+" AS BLOB)), 0) AS output_bytes",
		"COALESCE(length(CAST("+JSON文本("usage")+" AS BLOB)), 0) AS usage_bytes",
		"COALESCE(length(CAST("+JSON文本("definition")+" AS BLOB)), 0) AS definition_bytes")
}

// 获取步骤摘要 读取步骤元数据和各字段大小，避免将大型结果传入 Go。
func (当前仓储 *仓储) 获取步骤摘要(执行上下文 上下文包.上下文, 标识 string) (业务域包.步骤摘要, error) {
	var 行 步骤摘要行
	错误 := 步骤摘要查询(当前仓储.数据库.带有上下文(执行上下文)).条件("id = ?", 标识).取出(&行).错误
	return 行.摘要(), 未找到(错误)
}

// 步骤摘要集 按现有步骤游标和条数读取有界的元数据页。
func (当前仓储 *仓储) 步骤摘要集(执行上下文 上下文包.上下文, 标识 string, 之后 int64, 上限 int) ([]业务域包.步骤摘要, error) {
	if 上限 < 1 || 上限 > 200 || 之后 < 0 {
		return nil, 业务域包.无效错误
	}
	if _, 错误 := 当前仓储.获取(执行上下文, 标识); 错误 != nil {
		return nil, 错误
	}
	var 行集 []步骤摘要行
	if 错误 := 步骤摘要查询(当前仓储.数据库.带有上下文(执行上下文)).条件("run_id = ? AND sequence > ?", 标识, 之后).
		顺序("sequence").上限(上限).查找(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	输出 := make([]业务域包.步骤摘要, 0, len(行集))
	for _, 行 := range 行集 {
		输出 = append(输出, 行.摘要())
	}
	return 输出, nil
}

// 步骤数据 将所选值的当前长度和字节窗口一起读取，避免活动步骤更新时混用不同版本。
func (当前仓储 *仓储) 步骤数据(执行上下文 上下文包.上下文, 标识, 字段 string, 偏移, 上限 int) (输出 业务域包.步骤数据, 错误 error) {
	if 偏移 < 0 || 上限 < 4 || 上限 > 64<<10 {
		return 输出, 业务域包.无效错误
	}
	var 值 string
	无效 := "0"
	switch 字段 {
	case "input", "output", "usage", "definition":
		值 = JSON文本(字段)
	case "text":
		值 = `CASE WHEN type='model_call' AND json_extract(output, '$.context_message.role')='assistant'
			AND json_type(output, '$.context_message.content')='text'
			THEN json_extract(output, '$.context_message.content') END`
	case "reasoning":
		值 = "CASE WHEN type='model_call' AND json_type(output, '$.reasoning')='text' THEN json_extract(output, '$.reasoning') END"
		无效 = `CASE WHEN type='model_call' AND (
			json_type(output) NOT IN ('object','null') OR json_type(output, '$.reasoning') NOT IN ('text','null')
		) THEN 1 ELSE 0 END`
	default:
		return 输出, 业务域包.无效错误
	}
	数据库 := 当前仓储.数据库.带有上下文(执行上下文)
	已选择 := 数据库.模型(&表结构包.步骤{}).条件("id = ?", 标识).
		选择("CAST(" + 值 + " AS BLOB) AS data, " + 无效 + " AS invalid")
	var 大小 *int
	var 窗口 []byte
	var 无效推理 bool
	// 多读一个字节以检查右侧边界。先比较长度，再计算 SQLite 的起始位置。
	错误 = 数据库.表("(?) AS step_field", 已选择).
		选择("length(data), CASE WHEN ? < length(data) THEN substr(data, ? + 1, ? + 1) END, invalid", 偏移, 偏移, 上限).
		行().扫描(&大小, &窗口, &无效推理)
	if 错误判断包.是否属于(错误, SQL包.无行集错误) {
		return 输出, 业务域包.未找到错误
	}
	if 错误 != nil {
		return 输出, 错误
	}
	if 无效推理 {
		return 输出, 业务域包.步骤推理错误
	}
	if 大小 != nil {
		输出.存在, 输出.总计字节 = true, *大小
	}
	if 偏移 > 输出.总计字节 || (len(窗口) > 0 && !UTF8包.字符开始(窗口[0])) {
		return 业务域包.步骤数据{}, 业务域包.无效错误
	}
	结束 := min(上限, len(窗口))
	for 结束 < len(窗口) && !UTF8包.字符开始(窗口[结束]) {
		结束--
	}
	输出.下一个偏移, 输出.片段 = 偏移+结束, string(窗口[:结束])
	return 输出, nil
}

func (当前仓储 *仓储) 创建步骤(执行上下文 上下文包.上下文, 输入 业务域包.步骤) (输出 业务域包.步骤, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 父级 表结构包.执行
		if 错误 := 事务.条件("id = ? AND conversation_id = ? AND status = 'running'", 输入.执行标识, 输入.会话标识).取出(&父级).错误; 错误 != nil {
			return 未找到(错误)
		}
		if 输入.父级步骤标识 != nil {
			var 已有 表结构包.步骤
			if 错误 := 事务.条件("id = ? AND run_id = ? AND conversation_id = ?", *输入.父级步骤标识, 输入.执行标识, 输入.会话标识).取出(&已有).错误; 错误 != nil {
				return 未找到(错误)
			}
		}
		var 序号 int64
		if 错误 := 事务.模型(&表结构包.步骤{}).条件("run_id = ?", 输入.执行标识).选择("COALESCE(MAX(sequence),0)").扫描(&序号).错误; 错误 != nil {
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
	return 输出, 错误
}

func (当前仓储 *仓储) 保存步骤(执行上下文 上下文包.上下文, 输入 业务域包.步骤) error {
	查询 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).
		条件("id = ? AND run_id = ? AND status IN ?", 输入.标识, 输入.执行标识, []string{"pending", "running", "awaiting_approval", "awaiting_user_input"})
	结果 := 查询.更新集(map[string]any{
		// 首个步骤的引导位置由受理事务写入。使用数据库中的值，避免后续流式保存用旧快照覆盖。
		"input": GORM包.表达式(`CASE WHEN sequence=1 AND json_type(input, '$.steering_receipts')='object'
			THEN json_set(?, '$.steering_receipts', json_extract(input, '$.steering_receipts'))
			ELSE ? END`, string(输入.输入), string(输入.输入)),
		"definition": 可空步骤JSON(输入.定义), "status": 输入.状态,
		"output": 可空步骤JSON(输入.输出), "usage": 可空步骤JSON(输入.用量),
		"provider_request_id": 输入.供应商请求标识, "error_kind": 输入.错误类别, "error_message": 输入.错误消息,
		"started_at": 输入.开始时间, "updated_at": 输入.更新时间, "finished_at": 输入.结束时间,
	})
	if 结果.错误 != nil {
		return 结果.错误
	}
	if 结果.行集受影响 != 1 {
		return 业务域包.未找到错误
	}
	return nil
}

func 可空步骤JSON(值 JSON包.原始JSON消息) any {
	if len(值) == 0 || string(值) == "null" {
		return nil
	}
	return string(值)
}

func (当前仓储 *仓储) 保存消息(执行上下文 上下文包.上下文, 标识 string, 内容块集 []模型包.内容块, 状态 string) error {
	已编码, 错误 := JSON包.序列化(内容块集)
	if 错误 != nil {
		return 错误
	}
	当前时间 := 时间包.当前时间().Unix毫秒()
	var 已完成 *int64
	if 状态 == 业务域包.已完成 {
		已完成 = &当前时间
	}
	// 单次 更新 本身是原子的，不需要额外的隐式事务；取消后的自动回滚
	// 否则可能用 事务完成错误 覆盖原上下文错误。调用方已有的事务仍继续沿用。
	结果 := 当前仓储.数据库.会话连接(&GORM包.会话连接{跳过默认事务: true}).带有上下文(执行上下文).模型(&表结构包.消息{}).条件("id = ? AND role = 'assistant' AND status IN ?", 标识, []string{"running", "awaiting_approval", "awaiting_user_input"}).更新集(map[string]any{"content": string(已编码), "status": 状态, "updated_at": 当前时间, "completed_at": 已完成})
	if 结果.错误 != nil {
		return 结果.错误
	}
	if 结果.行集受影响 != 1 {
		return 业务域包.未找到错误
	}
	return nil
}

func (当前仓储 *仓储) 结束(执行上下文 上下文包.上下文, 输入 业务域包.执行) error {
	return 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		行 := 表结构包.执行{状态: 输入.状态, 错误类别: 输入.错误类别, 错误消息: 输入.错误消息, 更新时间: 输入.更新时间, 结束时间: 输入.结束时间}
		结果 := 事务.模型(&表结构包.执行{}).条件("id = ? AND status IN ?", 输入.标识, []string{"running", "awaiting_approval", "awaiting_user_input"}).选择("Status", "ErrorKind", "ErrorMessage", "UpdatedAt", "FinishedAt").更新集(&行)
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.未找到错误
		}
		if 错误 := 返回未投递引导(事务.条件("expected_run_id=?", 输入.标识)); 错误 != nil {
			return 错误
		}
		步骤集 := 事务.模型(&表结构包.步骤{}).选择("id").条件("run_id=?", 输入.标识)
		if 错误 := 事务.模型(&表结构包.上下文版本{}).条件("status=? AND generated_by_step_id IN (?)", "building", 步骤集).更新集(map[string]any{"status": "failed", "updated_at": 输入.更新时间}).错误; 错误 != nil {
			return 错误
		}
		if 错误 := 取消审批集(事务, 输入.标识, 输入.更新时间); 错误 != nil {
			return 错误
		}
		if 错误 := 结束工具结果集(事务, 输入.标识); 错误 != nil {
			return 错误
		}
		状态 := 输入.状态
		if 状态 == 业务域包.已完成 {
			状态 = 业务域包.已取消
		}
		if 错误 := 事务.模型(&表结构包.步骤{}).条件("run_id = ? AND status IN ?", 输入.标识, []string{"pending", "running", "awaiting_approval", "awaiting_user_input"}).更新集(map[string]any{"status": 状态, "finished_at": 输入.结束时间, "updated_at": 输入.更新时间}).错误; 错误 != nil {
			return 错误
		}
		var 已完成 *int64
		if 输入.状态 == 业务域包.已完成 {
			已完成 = 输入.结束时间
		}
		return 事务.模型(&表结构包.消息{}).条件("run_id = ? AND role = 'assistant' AND status IN ?", 输入.标识, []string{"running", "awaiting_approval", "awaiting_user_input"}).更新集(map[string]any{"status": 输入.状态, "completed_at": 已完成, "updated_at": 输入.更新时间}).错误
	})
}

// 恢复 标记中断、保留内容并确定新工具步骤的保守事实，不重试模型或工具操作。
// 已结束位于 为恢复时确认终态的时刻，不推测进程实际退出时间。
func (当前仓储 *仓储) 恢复(执行上下文 上下文包.上下文) error {
	return 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		当前时间 := 时间包.当前时间().Unix毫秒()
		if 错误 := 事务.模型(&表结构包.上下文版本{}).条件("status='building'").更新集(map[string]any{"status": "failed", "updated_at": 当前时间}).错误; 错误 != nil {
			return 错误
		}
		if 错误 := 取消审批集(事务, "", 当前时间); 错误 != nil {
			return 错误
		}
		if 错误 := 结束工具结果集(事务, ""); 错误 != nil {
			return 错误
		}
		活动 := []string{"pending", "running", "awaiting_approval", "awaiting_user_input"}
		if 错误 := 恢复已发布交付物集(事务, append(活动, 业务域包.已中断)); 错误 != nil {
			return 错误
		}
		for _, 表 := range []any{&表结构包.步骤{}, &表结构包.执行{}} {
			if 错误 := 事务.模型(表).条件("status IN ?", 活动).更新集(map[string]any{"status": 业务域包.已中断, "finished_at": 当前时间, "updated_at": 当前时间, "error_kind": "interrupted", "error_message": "服务退出导致执行中断"}).错误; 错误 != nil {
				return 错误
			}
		}
		if 错误 := 返回未投递引导(事务); 错误 != nil {
			return 错误
		}
		return 事务.模型(&表结构包.消息{}).条件("status IN ?", 活动).更新集(map[string]any{"status": 业务域包.已中断, "completed_at": nil, "updated_at": 当前时间}).错误
	})
}

// 结束工具结果集 与执行终态在同一事务中落盘。只处理明确标记新契约的步骤，
// 不根据旧记录的状态推测副作用；分批读取，避免恢复时无界加载工具输出。
func 结束工具结果集(事务 *GORM包.数据库, 执行标识 string) error {
	var 之后 string
	for {
		查询 := 事务.模型(&表结构包.步骤{}).条件("type='tool_call' AND status IN ? AND id>? AND json_extract(input,'$.operation_version')=?",
			[]string{业务域包.待处理, 业务域包.运行中, 业务域包.等待中审批, "awaiting_user_input"}, 之后, 工具包.操作版本)
		if 执行标识 != "" {
			查询 = 查询.条件("run_id=?", 执行标识)
		}
		var 行集 []表结构包.步骤
		if 错误 := 查询.选择("id", "input", "output", "definition", "tool_call_id", "started_at").顺序("id").上限(64).查找(&行集).错误; 错误 != nil {
			return 错误
		}
		if len(行集) == 0 {
			return nil
		}
		for _, 行 := range 行集 {
			输出, 错误 := 执行轨迹包.恢复输出(步骤(行))
			if 错误 != nil {
				return 错误
			}
			if 错误 := 事务.模型(&表结构包.步骤{}).条件("id=?", 行.标识).更新("output", string(输出)).错误; 错误 != nil {
				return 错误
			}
		}
		之后 = 行集[len(行集)-1].标识
	}
}

func 返回未投递引导(事务 *GORM包.数据库) error {
	return 事务.模型(&表结构包.后续消息{}).条件("status='steering'").
		更新集(map[string]any{"status": "queued", "revision": GORM包.表达式("revision+1")}).错误
}
```
