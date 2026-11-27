```go
//go:build !windows

package 运行

import 上下文包 "context"

type 沙箱状态 struct{}

func (*沙箱状态) 检查(上下文包.上下文) error { return nil }
func (*沙箱状态) close() error      { return nil }
func (*沙箱状态) 可读根目录集() []string  { return nil }

// 恢复沙箱 清理平台沙箱遗留状态；当前平台的沙箱由系统管理。
func 恢复沙箱(上下文包.上下文, string) error { return nil }
```
