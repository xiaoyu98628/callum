```go
//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package 文件锁

import (
	错误判断包 "errors"
	操作系统包 "os"
	系统调用包 "syscall"
)

func 加锁(文件 *操作系统包.文件) error {
	错误 := 系统调用包.文件锁(int(文件.文件描述符()), 系统调用包.加锁本次执行|系统调用包.加锁非阻塞)
	if 错误判断包.是否属于(错误, 系统调用包.操作将阻塞错误) || 错误判断包.是否属于(错误, 系统调用包.资源暂不可用错误) {
		return 错误判断包.合并(已加锁错误, 错误)
	}
	return 错误
}
```
