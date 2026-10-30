//go:build !darwin && !linux

package main

import (
	"io"
	"os"
)

// Windows 的管道关闭由 Go 的文件实现取消对应 I/O，不设置 Unix 文件状态标志。
func openStdio() (io.ReadCloser, io.WriteCloser, error) { return os.Stdin, os.Stdout, nil }
