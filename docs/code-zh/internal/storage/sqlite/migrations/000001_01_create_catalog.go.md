```go
package 迁移集

import (
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

func 目录管理表集() []any {
	return []any{&表结构包.供应商{}, &表结构包.模型{}}
}

func 校验目录管理(事务 *GORM包.数据库) error {
	return 校验唯一索引(事务, "models", "uq_provider_remote_model", []string{"provider_id", "remote_model_id"})
}
```
