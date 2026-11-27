```go
//go:build !darwin && !linux && !windows

package 补丁

import (
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	操作系统包 "os"
)

// 未具备固定句柄及权限保障的平台拒绝写入，读取工具仍可使用。
func 补丁文件就绪(*操作系统包.文件) error { return 工具包.不可用错误 }
func 保护补丁文件(*操作系统包.文件) error { return 工具包.不可用错误 }
func 保护补丁目录(*操作系统包.文件) error { return 工具包.不可用错误 }
```
