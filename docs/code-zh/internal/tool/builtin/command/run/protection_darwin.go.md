```go
package 运行

import (
	系统调用包 "syscall"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// Seatbelt 按路径判断，已存在的硬链接可从另一个路径访问同一文件。
// 存在此类别名时拒绝启动受管命令，不能把数据库可读误判为已隔离。
func 检查SQLite链接集(保护 工具包.受保护数据) error {
	文件集, 错误 := 保护.SQLite文件集()
	if 错误 != nil {
		return 错误
	}
	for _, 文件 := range 文件集 {
		状态信息, 成功 := 文件.系统().(*系统调用包.文件状态结构)
		if !成功 || 状态信息.硬链接数量 != 1 {
			return 工具包.已拒绝错误
		}
	}
	return nil
}
```
