package sqlite

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

func lockOwnership(file *os.File) error {
	var overlapped syscall.Overlapped
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	result, _, err := proc.Call(file.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if result == 0 {
		return errors.Join(errors.New("该数据目录已有运行中的 Callum 服务"), err)
	}
	return nil
}
