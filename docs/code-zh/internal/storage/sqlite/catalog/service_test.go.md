```go
package 目录管理测试

import (
	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	凭据测试包 "github.com/xiaoyu98628/callum/internal/credentials/credentialstest"
	供应商服务包 "github.com/xiaoyu98628/callum/internal/model/provider/service"
	目录管理包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	GORM包 "gorm.io/gorm"
)

func 新建服务(数据库 *GORM包.数据库) (*供应商服务包.服务, error) {
	密码算法, 错误 := 凭据包.新建(凭据测试包.系统凭据库{}, "catalog test database")
	if 错误 != nil {
		return nil, 错误
	}
	仓储, 错误 := 目录管理包.新建仓储(数据库, 密码算法)
	if 错误 != nil {
		return nil, 错误
	}
	return 供应商服务包.新建(仓储)
}
```
