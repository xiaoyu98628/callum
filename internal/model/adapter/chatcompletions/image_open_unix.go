//go:build unix

package chatcompletions

import (
	"os"
	"syscall"
)

func openImageFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
