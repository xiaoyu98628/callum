// Package imageinput 负责有界读取及准备本地图片。
package imageinput

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/xiaoyu98628/callum/internal/model"
)

const (
	MaxSourceBytes      = 16 << 20
	MaxSourceBatchBytes = 32 << 20
	MaxImages           = 8
	MaxBytes            = 4 << 20
)

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Read 读取规范绝对路径。打开后复核同一个普通文件。
func Read(ctx context.Context, path string) ([]byte, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, "", model.ErrImageRead
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", errors.Join(model.ErrImageRead, err)
	}
	if !info.Mode().IsRegular() {
		return nil, "", model.ErrImageRead
	}
	if info.Size() > MaxSourceBytes {
		return nil, "", model.ErrImageLimit
	}
	content, err := readFile(ctx, path, info)
	if err != nil {
		return nil, "", err
	}
	mime := http.DetectContentType(content)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return content, mime, nil
	default:
		return nil, "", model.ErrImageFormat
	}
}

func readFile(ctx context.Context, path string, expected os.FileInfo) (_ []byte, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, readFlags, 0)
	if err != nil {
		return nil, errors.Join(model.ErrImageRead, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			retErr = errors.Join(retErr, model.ErrImageRead, err)
		}
	}()
	opened, err := file.Stat()
	if err != nil {
		return nil, errors.Join(model.ErrImageRead, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(expected, opened) {
		return nil, model.ErrImageRead
	}
	content, err := io.ReadAll(io.LimitReader(contextReader{ctx, file}, MaxSourceBytes+1))
	if err != nil {
		return nil, errors.Join(model.ErrImageRead, err)
	}
	if len(content) > MaxSourceBytes {
		return nil, model.ErrImageLimit
	}
	return content, ctx.Err()
}
