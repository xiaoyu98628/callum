```go
//go:build unix

package 附件

import (
	操作系统包 "os"
	系统调用包 "syscall"
)

const 读取参数集 = 操作系统包.只读打开标志 | 系统调用包.非阻塞标志
```
