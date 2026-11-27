```go
//go:build !darwin && !linux

package 传输模块

import (
	输入输出包 "io"
	操作系统包 "os"
)

// 打开标准输入输出 返回由接收方关闭的标准流。
// Windows 的管道关闭由 Go 的文件实现取消对应 标识/打开标志，不设置 Unix 文件状态标志。
func 打开标准输入输出() (输入输出包.读取关闭器, 输入输出包.写入关闭器, error) { return 操作系统包.标准输入, 操作系统包.标准输出, nil }
```
