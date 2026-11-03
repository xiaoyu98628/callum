//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package sqlite

import (
	"errors"
	"os"
	"syscall"
)

func lockOwnership(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.Join(errors.New("该数据目录已有运行中的 Callum 服务"), err)
	}
	return nil
}
