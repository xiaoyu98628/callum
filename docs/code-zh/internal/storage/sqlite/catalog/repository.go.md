```go
package 目录管理

import (
	错误判断包 "errors"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	GORM包 "gorm.io/gorm"
)

// 仓储 使用 GORM 实现厂商与模型配置存储，数据库所有权仍属于调用方。
type 仓储 struct {
	数据库  *GORM包.数据库
	密码算法 凭据包.密码算法
}

var _ 供应商包.仓储 = (*仓储)(nil)

// 新建仓储 创建配置存储实现，不创建或关闭数据库。
func 新建仓储(数据库 *GORM包.数据库, 密码算法 凭据包.密码算法) (*仓储, error) {
	if 数据库 == nil || 密码算法 == nil {
		return nil, 错误判断包.新建("数据库连接不能为空")
	}
	return &仓储{数据库: 数据库, 密码算法: 密码算法}, nil
}
```
