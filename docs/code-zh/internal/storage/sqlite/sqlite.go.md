```go
package SQLite

import (
	上下文包 "context"
	SQL包 "database/sql"
	错误判断包 "errors"
	格式化包 "fmt"
	URL包 "net/url"
	文件路径包 "path/filepath"
	字符串包 "strings"

	GORM数据库驱动包 "gorm.io/driver/sqlite"
	GORM包 "gorm.io/gorm"
	日志记录器包 "gorm.io/gorm/logger"
	_ "modernc.org/sqlite"

	迁移集包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/migrations"
)

// 数据库 持有 GORM 实例及其底层连接池；创建者负责调用 关闭。
type 数据库 struct {
	数据库 *GORM包.数据库
	池   *SQL包.数据库
}

// 关闭 关闭底层连接池，可重复调用。
func (当前数据库 *数据库) 关闭() error { return 当前数据库.池.关闭() }

// 打开 使用绝对路径打开并迁移数据库，失败时关闭已创建的连接池。
// 初始化上下文只用于本次初始化；后续操作必须传递各自的调用上下文。
func 打开(执行上下文 上下文包.上下文, 路径 string) (*数据库, error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	if !文件路径包.是否绝对路径(路径) {
		return nil, 错误判断包.新建("数据库路径必须是绝对路径")
	}
	if 错误 := 准备文件(路径); 错误 != nil {
		return nil, 错误
	}
	池, 错误 := SQL包.打开("sqlite", 数据库URI(文件路径包.目标斜杠(路径)))
	if 错误 != nil {
		return nil, 格式化包.格式化创建错误("打开数据库: %w", 错误)
	}
	池.设置最大打开连接集(1)
	池.设置最大空闲连接集(1)
	if 错误 := 池.探测连通上下文(执行上下文); 错误 != nil {
		return nil, 错误判断包.合并(错误, 池.关闭())
	}
	// 使用官方 SQLite 方言，实际连接由 modernc 驱动提供。
	数据库, 错误 := GORM包.打开(GORM数据库驱动包.新建(GORM数据库驱动包.配置{驱动名称: "sqlite", 连接: 池}), &GORM包.配置{
		停用自动探测连通: true,
		// 错误交给调用边界处理，避免 ORM 默认日志打印 SQL 参数中的凭据。
		日志记录器: 日志记录器包.默认.日志模式(日志记录器包.静默),
	})
	if 错误 != nil {
		return nil, 错误判断包.合并(错误, 池.关闭())
	}
	if 错误 := 迁移集包.执行(执行上下文, 数据库); 错误 != nil {
		return nil, 错误判断包.合并(错误, 池.关闭())
	}
	// 不将初始化上下文绑定在返回的 GORM 实例上。
	return &数据库{数据库: 数据库, 池: 池}, nil
}

func 数据库URI(斜杠路径 string) string {
	// Windows 盘符属于 URI 路径。补上前导斜杠，避免 SQLite 将盘符解析为主机名。
	if !字符串包.存在前缀(斜杠路径, "/") {
		斜杠路径 = "/" + 斜杠路径
	}
	地址 := URL包.URL{方案: "file", 路径: 斜杠路径}
	查询 := URL包.值集{}
	查询.添加("_pragma", "foreign_keys(1)")
	查询.添加("_pragma", "busy_timeout(5000)")
	查询.添加("_pragma", "secure_delete(1)")
	查询.设置("_txlock", "immediate")
	地址.原始查询 = 查询.编码()
	return 地址.字符串()
}
```
