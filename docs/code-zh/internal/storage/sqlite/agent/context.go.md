```go
package 智能体

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	格式化包 "fmt"
	字符串包 "strings"

	GORM包 "gorm.io/gorm"

	业务域包 "github.com/xiaoyu98628/callum/internal/agent"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 命令历史从实际程序、参数、工作目录和权限边界识别同一操作。
// 环境摘要及读取目录包含服务端新建的私有路径，仅用于本次固定请求，不参与历史匹配。
// 这里同时读取旧快照与当前快照，输入只由服务端生成。格式参数固定为 SQL 列或表达式。
const 操作历史投影 = `CASE
	WHEN json_extract(%[1]s,'$.tool.name')='run_command'
		AND json_extract(%[1]s,'$.tool.source')='builtin'
		AND json_type(%[1]s,'$.arguments.command')='array'
		AND json_type(%[1]s,'$.details.executable_identity')='text'
		AND json_type(%[1]s,'$.details.cwd_identity')='text'
		AND json_type(%[1]s,'$.details.execution_boundary')='object'
	THEN 'command:' || json_array(
		json_remove(json_extract(%[1]s,'$.arguments.command'),'$[0]'),
		json_extract(%[1]s,'$.workspace.id','$.workspace.root_path',
			'$.details.executable','$.details.executable_identity',
			'$.details.executable_size','$.details.executable_modified_ns',
			'$.details.cwd','$.details.cwd_identity','$.details.execution_boundary',
			'$.details.sandbox_backend','$.details.writable_roots',
			'$.details.network_access','$.details.environment_profile',
			'$.details.environment_keys'))
	ELSE %[2]s END`

func 操作历史键(投影 string) string {
	if !字符串包.存在前缀(投影, "command:") {
		return 投影
	}
	摘要 := SHA256包.校验和256([]byte(投影))
	return "command:" + 十六进制包.编码目标字符串(摘要[:])
}

// 命令历史键 使用与旧记录查询相同的投影识别当前命令。实际审批仍使用完整请求哈希。
func (当前仓储 *仓储) 命令历史键(执行上下文 上下文包.上下文, 请求 []byte, 哈希 string) (string, error) {
	var 投影 string
	查询 := "SELECT " + 格式化包.格式化字符串(操作历史投影, "request", "request_hash") +
		" FROM (SELECT ? AS request, ? AS request_hash)"
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).原始(查询, string(请求), 哈希).扫描(&投影).错误; 错误 != nil {
		return "", 错误
	}
	return 操作历史键(投影), nil
}

// 已拒绝操作键集 恢复同一执行中已经拒绝的操作，继续任务不会重置这些决定。
func (当前仓储 *仓储) 已拒绝操作键集(执行上下文 上下文包.上下文, 执行标识 string) ([]string, error) {
	var 键集 []string
	投影 := 格式化包.格式化字符串(操作历史投影, "request", "request_hash")
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.审批{}).条件("run_id=? AND status=?", 执行标识, 业务域包.审批已拒绝).
		选择("DISTINCT " + 投影).上限(4097).扫描(&键集).错误; 错误 != nil {
		return nil, 错误
	}
	if len(键集) > 4096 {
		return nil, 模型包.输入上限错误
	}
	for 索引 := range 键集 {
		键集[索引] = 操作历史键(键集[索引])
	}
	return 键集, nil
}

// 操作事实集 从当前分支提取每个固定操作的最新待核实状态，摘要无权覆盖这些记录。
func (当前仓储 *仓储) 操作事实集(执行上下文 上下文包.上下文, 标识 string) ([]业务域包.操作事实, error) {
	基准 := func() *GORM包.数据库 {
		return 当前仓储.数据库.带有上下文(执行上下文).模型(&表结构包.步骤{}).表("conversation_run_steps AS s").
			关联("JOIN conversation_messages m ON m.run_id=s.run_id AND m.role='user'").
			条件("s.conversation_id=? AND m.conversation_id=? AND s.type=?", 标识, 标识, "tool_call")
	}
	var 无效 int64
	if 错误 := 基准().条件("json_extract(s.input,'$.operation_version')=?", 工具包.操作版本).
		条件(`(COALESCE(json_extract(s.output,'$.operation.version'),0)<>1
		OR COALESCE(json_extract(s.output,'$.operation.outcome'),'') NOT IN ('none','not_applied','applied','partially_applied','unknown')
		OR COALESCE(json_extract(s.output,'$.operation.request_hash'),'')<>COALESCE(json_extract(s.input,'$.request_hash'),'')
		OR length(COALESCE(json_extract(s.output,'$.operation.request_hash'),''))>128)`).数量(&无效).错误; 错误 != nil {
		return nil, 错误
	}
	if 无效 != 0 {
		return nil, 模型包.无效响应错误
	}
	var 行集 []struct {
		步骤标识, 执行标识, 工具, 请求哈希, 操作, 历史投影 string
	}
	投影 := 格式化包.格式化字符串(操作历史投影, "json_extract(s.input,'$.request')", "json_extract(s.output,'$.operation.request_hash')")
	// 命令跨临时目录使用同一历史匹配键，其他工具继续按完整请求哈希归组。
	// 先取最新的实际操作结果，确保已经确认成功的操作不再进入待核实集合。
	已排序 := 基准().选择(`s.id AS step_id,s.run_id,s.name AS tool,
			json_extract(s.output,'$.operation.request_hash') AS request_hash,
			json_extract(s.output,'$.operation') AS operation,
			json_extract(s.output,'$.operation.outcome') AS outcome,
			COALESCE(json_type(s.output,'$.error'),'null') AS error_type,
			`+投影+` AS history_projection,
			ROW_NUMBER() OVER (PARTITION BY `+投影+` ORDER BY m.sequence DESC,s.sequence DESC) AS rn`).
		条件("json_extract(s.output,'$.operation.version')=?", 工具包.操作版本).
		条件("COALESCE(json_extract(s.output,'$.operation.request_hash'),'')<>''").
		条件("json_extract(s.output,'$.operation.outcome')<>?", 工具包.操作结果不已生效).
		// 命令以退出码 0 确认成功；非零退出仍记录原样，但在历史匹配时不能消解旧的待核实事实。
		条件(`NOT (s.name='run_command' AND COALESCE(json_extract(s.input,'$.request.tool.source'),'')='builtin'
			AND json_extract(s.output,'$.operation.outcome')=? AND COALESCE(json_type(s.output,'$.error'),'null')='null'
			AND COALESCE(json_extract(s.output,'$.result.exit_code'),-1)<>0)`, 工具包.操作结果已生效)
	if 错误 := 当前仓储.数据库.带有上下文(执行上下文).表("(?) AS facts", 已排序).
		选择("step_id,run_id,tool,request_hash,operation,history_projection").条件("rn=?", 1).
		条件("(outcome IN ? OR (outcome=? AND error_type<>?))", []工具包.操作结果{工具包.操作结果未知, 工具包.操作结果部分}, 工具包.操作结果已生效, "null").
		顺序("step_id").上限(4097).扫描(&行集).错误; 错误 != nil {
		return nil, 错误
	}
	if len(行集) > 4096 {
		return nil, 模型包.输入上限错误
	}
	输出 := make([]业务域包.操作事实, 0, len(行集))
	for _, 行 := range 行集 {
		var 值 工具包.操作结果记录
		if JSON包.反序列化([]byte(行.操作), &值) != nil || !值.有效() || 值.请求哈希 != 行.请求哈希 {
			return nil, 模型包.无效响应错误
		}
		输出 = append(输出, 业务域包.操作事实{步骤标识: 行.步骤标识, 执行标识: 行.执行标识, 工具: 行.工具, 请求哈希: 行.请求哈希, 操作结果: string(值.操作结果), 历史键: 操作历史键(行.历史投影)})
	}
	return 输出, nil
}
```
