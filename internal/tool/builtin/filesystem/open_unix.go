//go:build unix

package filesystem

import "syscall"

const nonblock = syscall.O_NONBLOCK
