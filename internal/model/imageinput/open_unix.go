//go:build unix

package imageinput

import (
	"os"
	"syscall"
)

// 检查后路径可能被替换为命名管道，打开不得等待写端。
const readFlags = os.O_RDONLY | syscall.O_NONBLOCK
