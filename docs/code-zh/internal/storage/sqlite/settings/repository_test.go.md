```go
package 设置测试

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	错误判断包 "errors"
	文件路径包 "path/filepath"
	同步包 "sync"
	测试包 "testing"

	UUID包 "github.com/google/uuid"

	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	服务包 "github.com/xiaoyu98628/callum/internal/settings/service"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	会话仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	存储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/settings"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func 测试夹具(测试器 *测试包.测试器) (*SQLite包.数据库, *服务包.服务) {
	测试器.标记测试辅助函数()
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(func() {
		if 错误 := 数据库.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	})
	仓储, 错误 := 存储包.新建仓储(数据库.数据库)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前状态, 错误 := 服务包.新建(仓储)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 数据库, 当前状态
}

func Test旧版设置兼容性并且沙箱修订号(测试器 *测试包.测试器) {
	数据库, 当前状态 := 测试夹具(测试器)
	旧版 := `{"approval_policy":"on-request","approvals_reviewer":"user"}`
	if 错误 := 数据库.数据库.表("app_settings").条件("key=?", 设置包.执行键).更新("value", 旧版).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前, 错误 := 当前状态.获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil || 当前.修订号 != 1 {
		测试器.格式化报告致命错误("旧设置读取失败: %+v %v", 当前, 错误)
	}
	快照, 错误 := 设置包.执行快照(当前)
	if 错误 != nil || 快照.沙箱模式 != 工具包.沙箱工作区写入 {
		测试器.格式化报告致命错误("旧配置没有返回明确默认值: %+v %v", 快照, 错误)
	}
	var 已存储 string
	if 错误 := 数据库.数据库.表("app_settings").条件("key=?", 设置包.执行键).选择("value").扫描(&已存储).错误; 错误 != nil || 已存储 != 旧版 {
		测试器.格式化报告致命错误("查询改写了旧存储: %s %v", 已存储, 错误)
	}
	已保存, 错误 := 当前状态.更新(测试器.上下文(), 设置包.执行键, 1, JSON包.原始JSON消息(`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":"read-only"}`))
	if 错误 != nil || 已保存.修订号 != 2 {
		测试器.格式化报告致命错误("沙箱设置保存失败: %+v %v", 已保存, 错误)
	}
	已保存, 错误 = 当前状态.更新(测试器.上下文(), 设置包.执行键, 2, JSON包.原始JSON消息(旧版))
	if 错误 != nil || 已保存.修订号 != 3 {
		测试器.格式化报告致命错误("旧客户端更新失败: %+v %v", 已保存, 错误)
	}
	快照, 错误 = 设置包.执行快照(已保存)
	if 错误 != nil || 快照.沙箱模式 != 工具包.沙箱只读 {
		测试器.格式化报告致命错误("旧客户端更新扩大了权限: %+v %v", 快照, 错误)
	}
	if _, 错误 := 当前状态.更新(测试器.上下文(), 设置包.执行键, 2, JSON包.原始JSON消息(旧版)); !错误判断包.是否属于(错误, 设置包.冲突错误) {
		测试器.格式化报告致命错误("旧版本覆盖了沙箱设置: %v", 错误)
	}
}

func Test并发设置更新并且回滚(测试器 *测试包.测试器) {
	数据库, 当前状态 := 测试夹具(测试器)
	当前, 错误 := 当前状态.获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	标识, 错误 := UUID包.解析(当前.标识)
	if 错误 != nil || 标识.版本() != 7 || 当前.修订号 != 1 {
		测试器.格式化报告致命错误("默认设置不正确: %+v", 当前)
	}
	var 等待组 同步包.等待组
	结果集 := make(chan error, 2)
	for _, 选项 := range []string{"always", "never"} {
		等待组.Go(func() {
			_, 错误 := 当前状态.更新(测试器.上下文(), 设置包.执行键, 当前.修订号, JSON包.原始JSON消息(`{"approval_policy":"`+选项+`","approvals_reviewer":"user"}`))
			结果集 <- 错误
		})
	}
	等待组.等待()
	close(结果集)
	成功, 冲突集 := 0, 0
	for 错误 := range 结果集 {
		if 错误 == nil {
			成功++
		} else if 错误判断包.是否属于(错误, 设置包.冲突错误) {
			冲突集++
		} else {
			测试器.报告致命错误(错误)
		}
	}
	if 成功 != 1 || 冲突集 != 1 {
		测试器.格式化报告致命错误("并发更新没有互斥: success=%d conflicts=%d", 成功, 冲突集)
	}
	之前, 错误 := 当前状态.获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil || 之前.修订号 != 2 {
		测试器.格式化报告致命错误("设置版本不正确: %+v %v", 之前, 错误)
	}
	// 只在临时数据库注入写入失败，验证版本和值一起回滚，没有内存配置提前生效。
	if 错误 := 数据库.数据库.进程执行(`CREATE TRIGGER reject_settings BEFORE UPDATE ON app_settings BEGIN SELECT RAISE(ABORT,'simulated failure'); END`).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 当前状态.更新(测试器.上下文(), 设置包.执行键, 之前.修订号, JSON包.原始JSON消息(`{"approval_policy":"on-request","approvals_reviewer":"user"}`)); !错误判断包.是否属于(错误, 设置包.存储错误) {
		测试器.格式化报告致命错误("没有返回持久化失败: %v", 错误)
	}
	之后, 错误 := 当前状态.获取(测试器.上下文(), 设置包.执行键)
	if 错误 != nil || 之后.修订号 != 之前.修订号 || !字节包.相等(之后.值, 之前.值) {
		测试器.格式化报告致命错误("失败写入改动了设置: %+v %v", 之后, 错误)
	}
	if 错误 := 数据库.数据库.进程执行("DROP TRIGGER reject_settings").错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 当前状态.更新(测试器.上下文(), 设置包.执行键, 之前.修订号, JSON包.原始JSON消息(`{"approval_policy":"on-request","approvals_reviewer":"user"}`)); 错误 != nil {
		测试器.报告致命错误(错误)
	}
}

func Test受理使用事务性设置快照(测试器 *测试包.测试器) {
	数据库, 当前状态 := 测试夹具(测试器)
	会话集, 错误 := 会话仓储包.新建仓储(数据库.数据库)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	新建标识 := func() string { return UUID包.必须(UUID包.新建UUIDv7()).字符串() }
	输入 := func() 会话包.开始 {
		return 会话包.开始{创建: true, 会话标识: 新建标识(), 执行标识: 新建标识(), 步骤标识: 新建标识(), 用户消息标识: 新建标识(), 助手消息标识: 新建标识(),
			模型: 模型包.快照{标识: 新建标识(), 远程模型标识: "test", 供应商: 模型包.供应商快照{标识: 新建标识(), 名称: "test"}}, 协议: "chatcompletions", 内容: []模型包.内容块{{类型: "text", 文本: "问题"}}}
	}
	首个, 错误 := 会话集.开始(测试器.上下文(), 输入())
	if 错误 != nil || 首个.执行.设置快照 == nil || 首个.执行.设置快照.修订号 != 1 {
		测试器.格式化报告致命错误("受理未保存快照: %+v %v", 首个.执行, 错误)
	}
	if _, 错误 := 当前状态.更新(测试器.上下文(), 设置包.执行键, 1, JSON包.原始JSON消息(`{"approval_policy":"always","approvals_reviewer":"user"}`)); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	秒, 错误 := 会话集.开始(测试器.上下文(), 输入())
	if 错误 != nil || 秒.执行.设置快照 == nil || 秒.执行.设置快照.修订号 != 2 || 秒.执行.设置快照.审批策略 != "always" {
		测试器.格式化报告致命错误("新执行未使用已提交设置: %+v %v", 秒.执行, 错误)
	}
	if 首个.执行.设置快照.审批策略 != "on-request" {
		测试器.报告致命错误("旧执行快照跟随全局配置发生变化")
	}
	// 模拟数据库内容异常；受理失败不能留下会话、消息、运行或步骤。
	if 错误 := 数据库.数据库.表("app_settings").条件("key=?", 设置包.执行键).更新("value", `{"approval_policy":"invalid","approvals_reviewer":"user"}`).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	无效 := 输入()
	if _, 错误 := 会话集.开始(测试器.上下文(), 无效); !错误判断包.是否属于(错误, 设置包.存储错误) {
		测试器.格式化报告致命错误("坏配置没有阻止受理: %v", 错误)
	}
	for 表, 列 := range map[string]string{"conversations": "id", "conversation_messages": "conversation_id", "conversation_runs": "conversation_id", "conversation_run_steps": "conversation_id"} {
		var 数量 int64
		if 错误 := 数据库.数据库.表(表).条件(列+"=?", 无效.会话标识).数量(&数量).错误; 错误 != nil || 数量 != 0 {
			测试器.格式化报告致命错误("%s 留下不完整受理: %d %v", 表, 数量, 错误)
		}
	}
}
```
