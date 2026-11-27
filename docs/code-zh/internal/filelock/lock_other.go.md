```go
//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package 文件锁

import (
	错误判断包 "errors"
	操作系统包 "os"
)

func 加锁(*操作系统包.文件) error { return 错误判断包.新建("当前操作系统不支持安全的文件锁") }
```
