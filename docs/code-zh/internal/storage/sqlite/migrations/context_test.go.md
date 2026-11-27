```go
package 迁移集测试

import (
	JSON包 "encoding/json"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"

	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func Test旧版JSON摘要表结构需要新建数据目录(测试器 *测试包.测试器) {
	路径, 池 := 测试夹具数据库(测试器)
	_, 错误 := 池.进程执行上下文(测试器.上下文(), `CREATE TABLE conversation_context_versions (
		summary TEXT NOT NULL CONSTRAINT "ck_context_summary" CHECK (summary = '' OR (json_valid(summary) AND json_type(summary) = 'object'))
	) STRICT`)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 池.进程执行上下文(测试器.上下文(), "PRAGMA user_version = 4"); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	const 前一个 = `{"facts":["保留旧数据"]}`
	if _, 错误 := 池.进程执行上下文(测试器.上下文(), "INSERT INTO conversation_context_versions VALUES (?)", 前一个); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已打开, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 已打开 != nil {
		if 关闭错误 := 已打开.关闭(); 关闭错误 != nil {
			测试器.错误文本(关闭错误)
		}
	}
	if 错误 == nil || !字符串包.包含(错误.错误文本(), "旧版上下文摘要结构，请使用独立的新数据目录") {
		测试器.格式化报告致命错误("未提示旧摘要结构不受支持: %v", 错误)
	}
	var 已保存 string
	if 错误 := 池.查询行上下文(测试器.上下文(), "SELECT summary FROM conversation_context_versions").扫描(&已保存); 错误 != nil || 已保存 != 前一个 {
		测试器.格式化报告致命错误("拒绝启动时改写了旧数据: %q %v", 已保存, 错误)
	}
}

func Test上下文设置初始化并且校验(测试器 *测试包.测试器) {
	自定义 := 设置包.默认压缩()
	自定义.已启用, 自定义.词元预算, 自定义.保留轮次集 = false, 3000, 2
	已编码, 错误 := JSON包.序列化(自定义)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 测试 := range []struct {
		名称 string
		值  string
		有效 bool
	}{
		{名称: "保留用户设置", 值: string(已编码), 有效: true},
		{名称: "缺少设置"},
		{名称: "缺少字段", 值: `{}`},
		{名称: "旧阈值格式", 值: `{"enabled":true,"message_threshold":128,"byte_threshold":4194304,"retain_turns":4}`},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
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
			var 行 表结构包.设置
			if 错误 := 数据库.数据库.条件("key=?", 设置包.压缩键).取出(&行).错误; 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置, 错误 := 设置包.解码压缩(行.值)
			if 错误 != nil || 启动配置 != 设置包.默认压缩() || 行.修订号 != 1 {
				测试器.格式化报告致命错误("新库没有直接初始化 Token 设置: %+v %v", 行, 错误)
			}
			if 测试.值 == "" {
				错误 = 数据库.数据库.进程执行("DELETE FROM app_settings WHERE key=?", 设置包.压缩键).错误
			} else {
				错误 = 数据库.数据库.进程执行("UPDATE app_settings SET value=?,revision=7 WHERE key=?", 测试.值, 设置包.压缩键).错误
			}
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 错误 := 数据库.关闭(); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			已打开, 错误 := SQLite包.打开(测试器.上下文(), 路径)
			if 已打开 != nil {
				测试器.注册清理函数(func() {
					if 错误 := 已打开.关闭(); 错误 != nil {
						测试器.错误文本(错误)
					}
				})
			}
			if !测试.有效 {
				if 错误 == nil {
					测试器.报告致命错误("打开数据库接受了无效压缩设置")
				}
				return
			}
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 错误 := 已打开.数据库.条件("key=?", 设置包.压缩键).取出(&行).错误; 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 行.修订号 != 7 || string(行.值) != 测试.值 {
				测试器.格式化报告致命错误("重复打开改写了用户设置: %+v", 行)
			}
		})
	}
}
```
