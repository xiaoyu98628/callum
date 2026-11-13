//go:build !windows

package skills

import (
	"os"
	"syscall"
)

// 元数据在检查后被替换为管道时也不能阻塞请求。
const readFlags = os.O_RDONLY | syscall.O_NONBLOCK | syscall.O_NOFOLLOW
