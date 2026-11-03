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

func imageData(ctx context.Context, path string) (data string, retErr error) {
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
	file, err := os.Open(path)
	if err != nil {
		return "", errors.Join(model.ErrImageRead, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			retErr = errors.Join(retErr, model.ErrImageRead, err)
		}
	}()
	opened, err := file.Stat()
	if err != nil {
		return "", errors.Join(model.ErrImageRead, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return "", model.ErrImageRead
	}
	content, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: file}, MaxImageBytes+1))
	if err != nil {
		return "", errors.Join(model.ErrImageRead, err)
	}
	if len(content) > MaxImageBytes {
		return "", model.ErrInputLimit
	}
	mime := http.DetectContentType(content)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return "", model.ErrImageFormat
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content), nil
}
