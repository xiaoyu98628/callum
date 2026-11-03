//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package sqlite

import (
	"errors"
	"os"
)

func lockOwnership(*os.File) error {
	return errors.New("当前操作系统不支持安全的服务实例锁")
}
