package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lock(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errors.Join(ErrLocked, err)
	}
	return err
}
