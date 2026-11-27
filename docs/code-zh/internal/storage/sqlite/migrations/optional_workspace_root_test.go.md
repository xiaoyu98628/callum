```go
package 迁移集测试

import (
	SQL包 "database/sql"
	文件路径包 "path/filepath"
	正则表达式包 "regexp"
	字符串包 "strings"
	同步包 "sync"
	测试包 "testing"

	UUID包 "github.com/google/uuid"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	存储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/workspace"
	工作区包 "github.com/xiaoyu98628/callum/internal/workspace"
)

func 版本一个项目(测试器 *测试包.测试器) (string, *SQL包.数据库, string) {
	测试器.标记测试辅助函数()
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
	仓储, 错误 := 存储包.新建仓储(数据库.数据库)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	项目, 错误 := 仓储.创建(测试器.上下文(), 工作区包.工作区{标识: UUID包.必须(UUID包.新建UUIDv7()).字符串(), 名称: "保留项目", 根路径: 测试器.创建临时目录(), 其他路径集: []string{测试器.创建临时目录()}})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	会话 := 表结构包.会话{标识: UUID包.必须(UUID包.新建UUIDv7()).字符串(), 工作区标识: &项目.标识, 标题: "保留会话"}
	运行 := 表结构包.执行{标识: UUID包.必须(UUID包.新建UUIDv7()).字符串(), 会话标识: 会话.标识, 状态: "completed", 结束时间: new(int64(1))}
	消息 := 表结构包.消息{标识: UUID包.必须(UUID包.新建UUIDv7()).字符串(), 会话标识: 会话.标识, 执行标识: 运行.标识, 序号: 1, 角色: "user", 内容: []模型包.内容块{{类型: "text", 文本: "迁移必须保留的消息"}}, 状态: "completed", 已完成位于: new(int64(1))}
	for _, 值 := range []any{&会话, &运行, &消息} {
		if 错误 := 数据库.数据库.带有上下文(测试器.上下文()).创建(值).错误; 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	if 错误 := 数据库.数据库.进程执行("CREATE TRIGGER preserved_project_trigger AFTER UPDATE OF name ON workspaces BEGIN SELECT 1; END").错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 建表语句 string
	if 错误 := 数据库.数据库.原始("SELECT sql FROM sqlite_schema WHERE name='workspaces'").扫描(&建表语句).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 语句集 []string
	if 错误 := 数据库.数据库.原始("SELECT sql FROM sqlite_schema WHERE tbl_name='workspaces' AND type IN ('index','trigger') AND sql IS NOT NULL").扫描(&语句集).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
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
	// 仅将当前真实表映射的主目录改回 版本1 的 不 空，其他结构和已有数据保持一致。
	列 := 正则表达式包.必须编译("(?i)([`\"]root_path[`\"]\\s+text)\\b")
	旧版 := 列.替换全部字符串(建表语句, "${1} NOT NULL")
	if 旧版 == 建表语句 {
		测试器.报告致命错误("未生成 v1 主目录约束")
	}
	旧版 = 字符串包.替换(旧版, "workspaces", "workspaces_v1", 1)
	事务, 错误 := 池.开始事务(测试器.上下文(), nil)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer func() {
		if 错误 := 事务.回滚(); 错误 != nil && 错误 != SQL包.事务完成错误 {
			测试器.错误文本(错误)
		}
	}()
	查询集 := []string{旧版, "INSERT INTO workspaces_v1 SELECT * FROM workspaces", "DROP TABLE workspaces", "ALTER TABLE workspaces_v1 RENAME TO workspaces"}
	查询集 = append(查询集, 语句集...)
	查询集 = append(查询集, "PRAGMA user_version=1")
	for _, 查询 := range 查询集 {
		if _, 错误 := 事务.进程执行上下文(测试器.上下文(), 查询); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	if 错误 := 事务.提交(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 路径, 池, 项目.标识
}

func Test可选工作区根迁移保留数据(测试器 *测试包.测试器) {
	for _, 无效 := range []bool{false, true} {
		名称 := "upgrade"
		if 无效 {
			名称 = "rollback_and_retry"
		}
		测试器.执行(名称, func(测试器 *测试包.测试器) {
			路径, 池, 项目标识 := 版本一个项目(测试器)
			孤立 := UUID包.必须(UUID包.新建UUIDv7()).字符串()
			if 无效 {
				if _, 错误 := 池.进程执行上下文(测试器.上下文(), "INSERT INTO workspace_additional_folders(workspace_id,path) VALUES (?,?)", 孤立, 测试器.创建临时目录()); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				已打开, 错误 := SQLite包.打开(测试器.上下文(), 路径)
				if 已打开 != nil {
					if 关闭错误 := 已打开.关闭(); 关闭错误 != nil {
						测试器.错误文本(关闭错误)
					}
				}
				if 错误 == nil || !字符串包.包含(错误.错误文本(), "外键校验失败") {
					测试器.格式化报告致命错误("未在迁移后校验外键: %v", 错误)
				}
				var 版本, 必需, 临时 int
				for 查询, 目标 := range map[string]*int{"PRAGMA user_version": &版本, `SELECT "notnull" FROM pragma_table_info('workspaces') WHERE name='root_path'`: &必需, "SELECT count(*) FROM sqlite_schema WHERE name='workspaces_v2'": &临时} {
					if 错误 := 池.查询行上下文(测试器.上下文(), 查询).扫描(目标); 错误 != nil {
						测试器.报告致命错误(错误)
					}
				}
				if 版本 != 1 || 必需 != 1 || 临时 != 0 {
					测试器.格式化报告致命错误("失败后未回滚原结构: version=%d required=%d temporary=%d", 版本, 必需, 临时)
				}
				if _, 错误 := 池.进程执行上下文(测试器.上下文(), "DELETE FROM workspace_additional_folders WHERE workspace_id=?", 孤立); 错误 != nil {
					测试器.报告致命错误(错误)
				}
			}
			已打开, 错误 := SQLite包.打开(测试器.上下文(), 路径)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			测试器.注册清理函数(func() {
				if 错误 := 已打开.关闭(); 错误 != nil {
					测试器.错误文本(错误)
				}
			})
			var 版本, 外键键集, 消息集, 文件夹集, 触发器集 int
			for 查询, 目标 := range map[string]*int{"PRAGMA user_version": &版本, "PRAGMA foreign_keys": &外键键集, "SELECT count(*) FROM conversation_messages WHERE content LIKE '%迁移必须保留的消息%'": &消息集, "SELECT count(*) FROM workspace_additional_folders": &文件夹集, "SELECT count(*) FROM sqlite_schema WHERE name='preserved_project_trigger'": &触发器集} {
				if 错误 := 已打开.数据库.原始(查询).扫描(目标).错误; 错误 != nil {
					测试器.报告致命错误(错误)
				}
			}
			if 版本 != 8 || 外键键集 != 1 || 消息集 != 1 || 文件夹集 != 1 || 触发器集 != 1 {
				测试器.格式化报告致命错误("升级后丢失数据、触发器或外键设置: %d %d %d %d %d", 版本, 外键键集, 消息集, 文件夹集, 触发器集)
			}
			var 工作区标识 string
			if 错误 := 已打开.数据库.原始("SELECT workspace_id FROM conversations WHERE title='保留会话'").扫描(&工作区标识).错误; 错误 != nil || 工作区标识 != 项目标识 {
				测试器.格式化报告致命错误("会话项目归属发生变化: %s %v", 工作区标识, 错误)
			}
		})
	}
}

func Test并发工作区初始化(测试器 *测试包.测试器) {
	for _, 旧版 := range []bool{false, true} {
		名称 := "empty"
		if 旧版 {
			名称 = "version_one"
		}
		测试器.执行(名称, func(测试器 *测试包.测试器) {
			路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
			if 旧版 {
				路径, _, _ = 版本一个项目(测试器)
			}
			错误判断 := make(chan error, 4)
			var 等待 同步包.等待组
			for range 4 {
				等待.Go(func() {
					数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
					if 错误 == nil {
						错误 = 数据库.关闭()
					}
					错误判断 <- 错误
				})
			}
			等待.等待()
			close(错误判断)
			for 错误 := range 错误判断 {
				if 错误 != nil {
					测试器.错误文本(错误)
				}
			}
		})
	}
}
```
