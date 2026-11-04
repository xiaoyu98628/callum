package chatcompletions

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/xiaoyu98628/callum/internal/model"
)

const (
	// MaxImageBytes 限制单张本地图片读取量。
	MaxImageBytes = 4 << 20
	// MaxInputBytes 限制包含内联图片的实际请求 JSON，不是 Token 估算。
	MaxInputBytes = 12 << 20
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

func imageData(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		return "", model.ErrImageRead
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.Join(model.ErrImageRead, err)
	}
	if !info.Mode().IsRegular() {
		return "", model.ErrImageRead
	}
	if info.Size() > MaxImageBytes {
		return "", model.ErrInputLimit
	}
	content, err := readImageFile(ctx, path, info)
	if err != nil {
		return "", err
	}
	mime := http.DetectContentType(content)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return "", model.ErrImageFormat
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content), nil
}

// readImageFile 打开并核对先前检查的普通文件，读取及关闭均只使用这一个句柄。
// Unix 打开时不等待命名管道，避免路径在检查后被替换而阻塞取消和应用关闭。
func readImageFile(ctx context.Context, path string, expected os.FileInfo) (content []byte, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := openImageFile(path)
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
	content, err = io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: file}, MaxImageBytes+1))
	if err != nil {
		return nil, errors.Join(model.ErrImageRead, err)
	}
	if len(content) > MaxImageBytes {
		return nil, model.ErrInputLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return content, nil
}
