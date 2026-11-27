```go
package 迁移集测试

import (
	SQL包 "database/sql"
	格式化包 "fmt"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"

	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

// 通过存储包的公开入口验证真实表映射，不在迁移测试中重复定义产品表结构。
func 测试夹具数据库(测试器 *测试包.测试器) (string, *SQL包.数据库) {
	测试器.标记测试辅助函数()
	目录 := 文件路径包.合并(测试器.创建临时目录(), "data")
	if 错误 := 操作系统包.创建目录(目录, 0700); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	路径 := 文件路径包.合并(目录, "callum.db")
	文件, 错误 := 操作系统包.打开文件(路径, 操作系统包.创建文件标志|操作系统包.独占创建标志|操作系统包.读写打开标志, 0600)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 文件.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	池, 错误 := SQL包.打开("sqlite", 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	池.设置最大打开连接集(1)
	测试器.注册清理函数(func() {
		if 错误 := 池.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	})
	return 路径, 池
}

func Test迁移回滚(测试器 *测试包.测试器) {
	路径, 池 := 测试夹具数据库(测试器)
	// 冲突发生在建表之后的索引创建阶段，必须撤销之前已经创建的表。
	if _, 错误 := 池.进程执行上下文(测试器.上下文(), "CREATE TABLE occupied (id INTEGER)"); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 池.进程执行上下文(测试器.上下文(), "CREATE INDEX uq_provider_remote_model ON occupied(id)"); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已打开, 打开错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 已打开 != nil {
		if 错误 := 已打开.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	}
	if 打开错误 == nil || !字符串包.包含(打开错误.错误文本(), "uq_provider_remote_model") {
		测试器.格式化报告致命错误("迁移未在预期的索引创建阶段失败: %v", 打开错误)
	}
	var 版本, 表集 int
	if 错误 := 池.查询行上下文(测试器.上下文(), "PRAGMA user_version").扫描(&版本); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 池.查询行上下文(测试器.上下文(), "SELECT count(*) FROM sqlite_schema WHERE name IN ('model_providers', 'models')").扫描(&表集); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 版本 != 0 || 表集 != 0 {
		测试器.格式化报告致命错误("迁移未回滚: version=%d tables=%d", 版本, 表集)
	}
	// 移除测试制造的索引冲突后，具体迁移应能重新执行并完整提交。
	if _, 错误 := 池.进程执行上下文(测试器.上下文(), "DROP INDEX uq_provider_remote_model"); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已重试, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.格式化报告致命错误("移除冲突后无法重新初始化: %v", 错误)
	}
	测试器.注册清理函数(func() {
		if 错误 := 已重试.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	})
	if 错误 := 池.查询行上下文(测试器.上下文(), "PRAGMA user_version").扫描(&版本); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 池.查询行上下文(测试器.上下文(), "SELECT count(*) FROM sqlite_schema WHERE name IN ('model_providers', 'models')").扫描(&表集); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 版本 != 8 || 表集 != 2 {
		测试器.格式化报告致命错误("具体迁移未完整提交: version=%d tables=%d", 版本, 表集)
	}
}

func Test不支持版本集保留数据(测试器 *测试包.测试器) {
	for _, 已拒绝版本 := range []int{5, 9, 99} {
		测试器.执行(格式化包.格式化字符串("version_%d", 已拒绝版本), func(测试器 *测试包.测试器) {
			路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
			数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			测试器.注册清理函数(func() {
				if 错误 := 数据库.关闭(); 错误 != nil {
					测试器.错误文本(错误)
				}
			})
			var 已初始化版本 int
			if 错误 := 数据库.数据库.原始("PRAGMA user_version").扫描(&已初始化版本).错误; 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 已初始化版本 != 8 {
				测试器.格式化报告致命错误("新库结构版本错误: %d", 已初始化版本)
			}
			if 错误 := 数据库.数据库.带有上下文(测试器.上下文()).进程执行(格式化包.格式化字符串("PRAGMA user_version = %d", 已拒绝版本)).错误; 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 错误 := 数据库.关闭(); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			已打开, 打开错误 := SQLite包.打开(测试器.上下文(), 路径)
			if 已打开 != nil {
				if 错误 := 已打开.关闭(); 错误 != nil {
					测试器.错误文本(错误)
				}
			}
			if 打开错误 == nil || !字符串包.包含(打开错误.错误文本(), 格式化包.格式化字符串("不支持数据库结构版本 %d", 已拒绝版本)) {
				测试器.格式化报告致命错误("未明确拒绝不支持的版本: %v", 打开错误)
			}
			池, 错误 := SQL包.打开("sqlite", 路径)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			defer func() {
				if 错误 := 池.关闭(); 错误 != nil {
					测试器.错误文本(错误)
				}
			}()
			var 版本 int
			if 错误 := 池.查询行上下文(测试器.上下文(), "PRAGMA user_version").扫描(&版本); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 版本 != 已拒绝版本 {
				测试器.报告致命错误("不支持的版本被修改")
			}
		})
	}
}

func Test不支持已有表结构保留数据(测试器 *测试包.测试器) {
	for _, 测试 := range []struct {
		名称   string
		版本   int
		预期错误 string
	}{
		{名称: "缺少结构版本", 版本: 0, 预期错误: "缺少结构版本"},
		{名称: "整数主键结构", 版本: 1, 预期错误: "数据库结构与版本 1 不匹配"},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			路径, 池 := 测试夹具数据库(测试器)
			// 相同版本号的旧整数主键库必须被拒绝，并保留原有结构和数据。
			const 原始表结构 = "CREATE TABLE model_providers (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"
			if _, 错误 := 池.进程执行上下文(测试器.上下文(), 原始表结构); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if _, 错误 := 池.进程执行上下文(测试器.上下文(), "INSERT INTO model_providers (id,name) VALUES (?,?)", 8, "保留配置"); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if _, 错误 := 池.进程执行上下文(测试器.上下文(), 格式化包.格式化字符串("PRAGMA user_version = %d", 测试.版本)); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			已打开, 打开错误 := SQLite包.打开(测试器.上下文(), 路径)
			if 已打开 != nil {
				if 错误 := 已打开.关闭(); 错误 != nil {
					测试器.错误文本(错误)
				}
			}
			if 打开错误 == nil || !字符串包.包含(打开错误.错误文本(), 测试.预期错误) {
				测试器.格式化报告致命错误("未明确拒绝旧结构: %v", 打开错误)
			}
			var 版本 int
			if 错误 := 池.查询行上下文(测试器.上下文(), "PRAGMA user_version").扫描(&版本); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var 名称, 表结构 string
			if 错误 := 池.查询行上下文(测试器.上下文(), "SELECT name FROM model_providers WHERE id = ?", 8).扫描(&名称); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 错误 := 池.查询行上下文(测试器.上下文(), "SELECT sql FROM sqlite_schema WHERE name = 'model_providers'").扫描(&表结构); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var 模型表集 int
			if 错误 := 池.查询行上下文(测试器.上下文(), "SELECT count(*) FROM sqlite_schema WHERE name = 'models'").扫描(&模型表集); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 版本 != 测试.版本 || 名称 != "保留配置" || 表结构 != 原始表结构 || 模型表集 != 0 {
				测试器.格式化报告致命错误("拒绝打开后原数据或结构被改写: version=%d name=%q schema=%q models=%d", 版本, 名称, 表结构, 模型表集)
			}
		})
	}
}

func Test后续消息迁移保留版本四个(测试器 *测试包.测试器) {
	路径, 池 := 测试夹具数据库(测试器)
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 查询 := range []string{
		"DROP TABLE conversation_followups",
		"CREATE TABLE v4_marker (payload TEXT NOT NULL) STRICT",
		"INSERT INTO v4_marker VALUES ('保留')",
		"PRAGMA user_version = 4",
	} {
		if _, 错误 := 池.进程执行上下文(测试器.上下文(), 查询); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	数据库, 错误 = SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(func() {
		if 错误 := 数据库.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	})
	var 版本, 条目集 int
	var 标记 string
	for _, 检查 := range []struct {
		SQL string
		目标  any
	}{
		{"PRAGMA user_version", &版本},
		{"SELECT COUNT(*) FROM conversation_followups", &条目集},
		{"SELECT payload FROM v4_marker", &标记},
	} {
		if 错误 := 池.查询行上下文(测试器.上下文(), 检查.SQL).扫描(检查.目标); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	if 版本 != 8 || 条目集 != 0 || 标记 != "保留" {
		测试器.格式化报告致命错误("后续消息迁移状态异常: %d, %d, %q", 版本, 条目集, 标记)
	}
}
```
