```go
package 迁移集测试

import (
	文件路径包 "path/filepath"
	测试包 "testing"

	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func Test版本数量不隐藏缺失约束(测试器 *测试包.测试器) {
	路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.数据库.进程执行("DROP INDEX uq_active_conversation").错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 已打开, 错误 := SQLite包.打开(测试器.上下文(), 路径); 错误 == nil {
		if 错误 := 已打开.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
		测试器.报告致命错误("仅凭版本号接受缺少互斥约束的数据库")
	}
}
```
