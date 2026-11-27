```go
package 迁移集测试

import (
	SQL包 "database/sql"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"

	UUID包 "github.com/google/uuid"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	策略包 "github.com/xiaoyu98628/callum/internal/tool/policy"
)

func Test自动审核方迁移保留执行并且审计(测试器 *测试包.测试器) {
	执行上下文 := 测试器.上下文()
	路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
	数据库, 错误 := SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 数据库.关闭()
	标识 := func() string { return UUID包.必须(UUID包.新建UUIDv7()).字符串() }
	会话 := 表结构包.会话{标识: 标识(), 标题: "已有会话"}
	快照 := 设置包.快照{修订号: 1, 执行: 设置包.默认执行()}
	运行 := 表结构包.执行{标识: 标识(), 会话标识: 会话.标识, 状态: "completed", 结束时间: new(int64(1)), 设置快照: &快照}
	步骤 := 表结构包.步骤{标识: 标识(), 会话标识: 会话.标识, 执行标识: 运行.标识, 序号: 1, 类型: "tool_call", 名称: "旧工具", 状态: "completed", 结束时间: new(int64(1))}
	审批 := 表结构包.审批{标识: 标识(), 执行标识: 运行.标识, 步骤标识: 步骤.标识, 请求: []byte(`{}`), 请求哈希: 字符串包.重复("a", 64), 状态: "approved", 已决定依据: new("local_user"), 已决定位于: new(int64(1))}
	for _, 行 := range []any{&会话, &运行, &步骤, &审批} {
		if 错误 := 数据库.数据库.创建(行).错误; 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	if 错误 := 数据库.数据库.进程执行("CREATE TRIGGER preserved_run_trigger AFTER UPDATE OF status ON conversation_runs BEGIN SELECT 1; END").错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 建表语句 string
	if 错误 := 数据库.数据库.原始("SELECT sql FROM sqlite_schema WHERE name='conversation_runs'").扫描(&建表语句).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 对象集 []string
	if 错误 := 数据库.数据库.原始("SELECT sql FROM sqlite_schema WHERE tbl_name='conversation_runs' AND type IN ('index','trigger') AND sql IS NOT NULL").扫描(&对象集).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	旧版 := 字符串包.替换(建表语句, "COALESCE(json_extract(settings_snapshot,'$.approvals_reviewer') IN ('user','auto_review'),0)", "json_extract(settings_snapshot,'$.approvals_reviewer') IS 'user'", 1)
	if 建表语句 == 旧版 {
		测试器.报告致命错误("缺少当前审批约束")
	}
	池, 错误 := SQL包.打开("sqlite", 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 池.关闭()
	旧版 = 字符串包.替换(旧版, "conversation_runs", "conversation_runs_v2", 1)
	查询集 := []string{旧版, "INSERT INTO conversation_runs_v2 SELECT * FROM conversation_runs", "DROP TABLE conversation_runs", "ALTER TABLE conversation_runs_v2 RENAME TO conversation_runs"}
	查询集 = append(查询集, 对象集...)
	查询集 = append(查询集, "PRAGMA user_version=2")
	for _, 查询 := range 查询集 {
		if _, 错误 := 池.进程执行上下文(执行上下文, 查询); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}

	// 插入已有版本的孤儿记录，验证重建表及版本在升级失败后一起回滚。
	孤立 := 标识()
	if _, 错误 := 池.进程执行上下文(执行上下文, "UPDATE conversation_runs SET conversation_id=? WHERE id=?", 孤立, 运行.标识); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 已拒绝, 错误 := SQLite包.打开(执行上下文, 路径); 错误 == nil {
		已拒绝.关闭()
		测试器.报告致命错误("允许外键损坏的升级")
	}
	var 版本 int
	if 错误 := 池.查询行上下文(执行上下文, "PRAGMA user_version").扫描(&版本); 错误 != nil || 版本 != 2 {
		测试器.格式化报告致命错误("版本未回滚: %d %v", 版本, 错误)
	}
	if _, 错误 := 池.进程执行上下文(执行上下文, "UPDATE conversation_runs SET conversation_id=? WHERE id=?", 会话.标识, 运行.标识); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已升级, 错误 := SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 已升级.关闭()
	var 旧 表结构包.执行
	if 错误 := 已升级.数据库.条件("id=?", 运行.标识).取出(&旧).错误; 错误 != nil || 旧.设置快照 == nil || 旧.设置快照.审批审核方 != 策略包.用户 {
		测试器.格式化报告致命错误("旧执行未保留: %+v %v", 旧, 错误)
	}
	var 数量 int
	if 错误 := 已升级.数据库.原始("SELECT count(*) FROM conversation_run_approvals WHERE id=? AND decided_by='local_user'", 审批.标识).扫描(&数量).错误; 错误 != nil || 数量 != 1 {
		测试器.格式化报告致命错误("旧审批未保留: %d %v", 数量, 错误)
	}
	if 错误 := 已升级.数据库.原始("SELECT count(*) FROM sqlite_schema WHERE name='preserved_run_trigger'").扫描(&数量).错误; 错误 != nil || 数量 != 1 {
		测试器.格式化报告致命错误("触发器未保留: %d %v", 数量, 错误)
	}
	快照.审批审核方 = 策略包.自动审核
	下一个 := 表结构包.执行{标识: 标识(), 会话标识: 会话.标识, 状态: "running", 设置快照: &快照}
	if 错误 := 已升级.数据库.创建(&下一个).错误; 错误 != nil {
		测试器.格式化报告致命错误("升级后不能保存 AI 审批快照: %v", 错误)
	}
}
```
