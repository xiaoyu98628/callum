```go
package 服务

import (
	错误判断包 "errors"

	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
)

// 服务 负责厂商和模型配置的业务校验、标识分配及启用规则。
// 存储由调用方注入并管理生命周期，业务包不依赖数据库驱动或 ORM。
type 服务 struct{ 仓储 供应商包.仓储 }

// 新建 使用现有存储创建配置服务，不创建或关闭外部资源。
func 新建(仓储 供应商包.仓储) (*服务, error) {
	if 仓储 == nil {
		return nil, 错误判断包.新建("配置存储不能为空")
	}
	return &服务{仓储: 仓储}, nil
}
```
