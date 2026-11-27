//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package filelock

import (
	"errors"
	"os"
	"syscall"
)

func lock(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errors.Join(ErrLocked, err)
	}
	return err
}
