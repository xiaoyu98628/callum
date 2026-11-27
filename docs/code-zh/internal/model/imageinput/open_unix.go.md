```go
//go:build unix

package 图片输入

import (
	操作系统包 "os"
	系统调用包 "syscall"
)

// 检查后路径可能被替换为命名管道，打开不得等待写端。
const 读取参数集 = 操作系统包.只读打开标志 | 系统调用包.非阻塞标志
```
