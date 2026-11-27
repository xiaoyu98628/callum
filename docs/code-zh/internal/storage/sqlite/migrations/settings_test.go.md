```go
package 迁移集测试

import (
	文件路径包 "path/filepath"
	测试包 "testing"

	UUID包 "github.com/google/uuid"

	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func Test设置并且快照约束集(测试器 *测试包.测试器) {
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 数据库.关闭()
	for _, 变更 := range []map[string]any{{"revision": 0}, {"value": "[]"}, {"value": "not-json"}, {"id": "invalid"}} {
		if 错误 := 数据库.数据库.模型(&表结构包.设置{}).条件("key=?", 设置包.执行键).更新集(变更).错误; 错误 == nil {
			测试器.格式化报告致命错误("设置表接受了无效数据: %v", 变更)
		}
	}
	var 行 表结构包.设置
	if 错误 := 数据库.数据库.首个(&行).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	行.标识 = UUID包.必须(UUID包.新建UUIDv7()).字符串()
	if 错误 := 数据库.数据库.创建(&行).错误; 错误 == nil {
		测试器.报告致命错误("允许同一配置组存在多条记录")
	}
}

func Test常规设置种子已有数据库(测试器 *测试包.测试器) {
	路径, 池 := 测试夹具数据库(测试器)
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	执行上下文 := 测试器.上下文()
	if _, 错误 := 池.进程执行上下文(执行上下文, "DELETE FROM app_settings WHERE key=?", 设置包.常规键); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	数据库, 错误 = SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer func() {
		if 错误 := 数据库.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	}()
	var 版本, 修订号 int64
	var 值 string
	if 错误 := 池.查询行上下文(执行上下文, "PRAGMA user_version").扫描(&版本); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 池.查询行上下文(执行上下文, "SELECT value, revision FROM app_settings WHERE key=?", 设置包.常规键).扫描(&值, &修订号); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 版本 != 8 || 修订号 != 1 || 值 != `{"follow_up_mode":"queue"}` {
		测试器.格式化报告致命错误("既有数据库常规设置补齐异常: %d, %d, %s", 版本, 修订号, 值)
	}
}
```
