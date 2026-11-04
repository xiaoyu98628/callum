package fileaccess

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// OpenFile 为固定目录内的打开操作添加平台非阻塞标志，避免特殊文件阻塞取消。
// 调用方负责检查目标类型并关闭返回的文件；底层打开错误保持原样。
func OpenFile(root *os.Root, path string, flags int, perm os.FileMode) (*os.File, error) {
	return root.OpenFile(path, flags|nonblock, perm)
}

// OpenRead 使用非阻塞标志打开路径，避免命名管道阻塞取消；只读普通文件和目录才可使用。
func OpenRead(root *os.Root, path string) (*os.File, error) {
	f, err := OpenFile(root, path, os.O_RDONLY, 0)
	if err != nil {
		return nil, tool.ErrDenied
	}
	return f, nil
}

// ReadTextFile 读取固定目录内的文本，并在返回前关闭本次打开的文件。
func ReadTextFile(ctx context.Context, root *os.Root, path string) (data []byte, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := OpenRead(root, path)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	return ReadText(ctx, f)
}

// ReadText 从借用句柄的当前位置读取不超过 1 MiB 的 UTF-8 文本，不关闭或重置句柄。
func ReadText(ctx context.Context, f *os.File) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	if !stat.Mode().IsRegular() {
		return nil, tool.ErrUnavailable
	}
	if stat.Size() > 1<<20 {
		return nil, tool.ErrLimit
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	if len(data) > 1<<20 {
		return nil, tool.ErrLimit
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, tool.ErrUnavailable
	}
	return data, ctx.Err()
}
