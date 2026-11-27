```go
package 目录管理

import (
	SQL包 "database/sql"
	错误判断包 "errors"
	格式化包 "fmt"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	GORM包 "gorm.io/gorm"
	SQLite引擎包 "modernc.org/sqlite"
	SQLite底层库包 "modernc.org/sqlite/lib"
)

func 存储错误(错误 error) error {
	if 错误 == nil {
		return nil
	}
	if 错误判断包.是否属于(错误, 凭据包.不可用错误) || 错误判断包.是否属于(错误, 凭据包.无效错误) {
		return 错误判断包.合并(供应商包.凭据错误, 错误)
	}
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) || 错误判断包.是否属于(错误, SQL包.无行集错误) {
		return 错误判断包.合并(供应商包.未找到错误, 错误)
	}
	if 当前条目, 成功 := 错误判断包.按类型匹配[*SQLite引擎包.错误](错误); 成功 {
		switch 当前条目.代码() {
		case SQLite底层库包.SQLite约束唯一:
			return 错误判断包.合并(供应商包.冲突错误, 错误)
		case SQLite底层库包.SQLite约束外键, SQLite底层库包.SQLite约束触发:
			// 当前结构没有自定义触发器；SQLite 的 RESTRICT 外键检查可能返回触发器约束码。
			return 错误判断包.合并(供应商包.被引用错误, 错误)
		case SQLite底层库包.SQLite约束检查, SQLite底层库包.SQLite约束非空, SQLite底层库包.SQLite约束数据类型:
			return 错误判断包.合并(供应商包.无效配置错误, 错误)
		}
	}
	return 格式化包.格式化创建错误("配置存储操作失败: %w", 错误)
}

func 变化通知(结果 *GORM包.数据库) error {
	if 结果.错误 != nil {
		return 存储错误(结果.错误)
	}
	if 结果.行集受影响 == 0 {
		return 供应商包.未找到错误
	}
	return nil
}
```
