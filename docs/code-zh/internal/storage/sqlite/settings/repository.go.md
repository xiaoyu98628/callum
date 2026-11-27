```go
package 设置

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	数学包 "math"
	时间包 "time"

	GORM包 "gorm.io/gorm"

	业务域包 "github.com/xiaoyu98628/callum/internal/settings"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// 仓储 借用 GORM 实例；传入受理事务时，获取 使用该事务而不另占连接。
type 仓储 struct{ 数据库 *GORM包.数据库 }

var _ 业务域包.仓储 = (*仓储)(nil)

func 新建仓储(数据库 *GORM包.数据库) (*仓储, error) {
	if 数据库 == nil {
		return nil, 业务域包.无效错误
	}
	return &仓储{数据库: 数据库}, nil
}

func 记录(行 表结构包.设置) 业务域包.记录 {
	return 业务域包.记录{标识: 行.标识, 键: 行.键, 值: 行.值, 修订号: 行.修订号, 创建时间: 行.创建时间, 更新时间: 行.更新时间}
}

func (当前仓储 *仓储) 获取(执行上下文 上下文包.上下文, 键 string) (业务域包.记录, error) {
	var 行 表结构包.设置
	错误 := 当前仓储.数据库.带有上下文(执行上下文).条件("key = ?", 键).取出(&行).错误
	if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
		return 业务域包.记录{}, 业务域包.未找到错误
	}
	if 错误 != nil {
		return 业务域包.记录{}, 错误判断包.合并(业务域包.存储错误, 错误)
	}
	return 记录(行), nil
}

func (当前仓储 *仓储) 更新(执行上下文 上下文包.上下文, 键 string, 修订号 int64, 值 JSON包.原始JSON消息) (输出 业务域包.记录, 返回错误 error) {
	if 修订号 < 1 || 修订号 == 数学包.最大整数64 || len(值) > 业务域包.最大值字节 {
		return 输出, 业务域包.无效错误
	}
	错误 := 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.设置
		if 错误 := 事务.条件("key = ?", 键).取出(&行).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.未找到错误
			}
			return 错误
		}
		if 行.修订号 != 修订号 {
			return 业务域包.冲突错误
		}
		行.值, 行.修订号, 行.更新时间 = 值, 修订号+1, 时间包.当前时间().Unix毫秒()
		结果 := 事务.模型(&表结构包.设置{}).条件("id = ? AND revision = ?", 行.标识, 修订号).选择("Value", "Revision", "UpdatedAt").更新集(&行)
		if 结果.错误 != nil {
			return 结果.错误
		}
		if 结果.行集受影响 != 1 {
			return 业务域包.冲突错误
		}
		输出 = 记录(行)
		return nil
	})
	if 错误 != nil && !错误判断包.是否属于(错误, 业务域包.冲突错误) && !错误判断包.是否属于(错误, 业务域包.未找到错误) {
		return 业务域包.记录{}, 错误判断包.合并(业务域包.存储错误, 错误)
	}
	return 输出, 错误
}
```
