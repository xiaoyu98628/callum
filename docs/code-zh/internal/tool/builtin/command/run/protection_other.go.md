```go
//go:build !darwin && !windows

package 运行

import 工具包 "github.com/xiaoyu98628/callum/internal/tool"

// 其他平台的命令当前按沙箱外边界审批。
func 检查SQLite链接集(工具包.受保护数据) error { return nil }
```
