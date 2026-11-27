```go
//go:build !darwin && !windows

package 运行

import (
	上下文包 "context"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func (当前固定命令 *固定命令) 配置沙箱(当前参数 参数, 权限 工具包.执行权限, _ 工具包.受保护数据) error {
	项目写入, 已处理, 错误 := 当前固定命令.沙箱请求(当前参数, 权限)
	if 错误 != nil || 已处理 {
		return 错误
	}
	if 项目写入 {
		return 工具包.不可用错误
	}
	当前固定命令.边界.风险 = "sandbox_backend_unsupported"
	return nil
}

func (*固定命令) 初始化执行(上下文包.上下文) (func() error, error) {
	return func() error { return nil }, nil
}

func (*固定命令) 沙箱后端() string { return "" }
```
