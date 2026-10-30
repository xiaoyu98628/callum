//go:build !darwin && !linux

package stdio

import (
	"io"
	"os"
)

// OpenStandardStreams 返回由接收方关闭的标准流。
// Windows 的管道关闭由 Go 的文件实现取消对应 I/O，不设置 Unix 文件状态标志。
func OpenStandardStreams() (io.ReadCloser, io.WriteCloser, error) { return os.Stdin, os.Stdout, nil }
