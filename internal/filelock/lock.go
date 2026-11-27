// Package filelock 提供随文件句柄和进程退出释放的跨进程排他锁。
package filelock

import (
	"errors"
	"os"
)

// ErrLocked 表示另一个句柄已持有排他锁，不代表路径或权限错误。
var ErrLocked = errors.New("文件锁已被占用")

// Try 非阻塞地锁定普通文件。调用者持有文件，关闭句柄即释放锁；
// 不删除锁文件，避免等待者仍持有旧 inode 时另一个实例创建第二把锁。
func Try(file *os.File) error {
	if file == nil {
		return os.ErrInvalid
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrInvalid
	}
	return lock(file)
}
