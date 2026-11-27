//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package filelock

import (
	"errors"
	"os"
)

func lock(*os.File) error { return errors.New("当前操作系统不支持安全的文件锁") }
