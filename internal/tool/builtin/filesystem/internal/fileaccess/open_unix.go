//go:build unix

package fileaccess

import "syscall"

const nonblock = syscall.O_NONBLOCK
