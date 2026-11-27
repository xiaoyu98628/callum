```go
//go:build !windows

package 技能

import (
	操作系统包 "os"
	系统调用包 "syscall"
)

// 元数据在检查后被替换为管道时也不能阻塞请求。
const 读取参数集 = 操作系统包.只读打开标志 | 系统调用包.非阻塞标志 | 系统调用包.不跟随符号链接标志
```
