package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"
)

// New 同时向 stderr 和轮转文件写 JSON 日志，不接管 stderr 的生命周期。
// 多个进程可以使用同一路径，文件操作由同一跨进程锁串行保护。
func New(path string, level slog.Level, stderr io.Writer) (*slog.Logger, func() error, error) {
	return newLogger(path, level, stderr, rotationPolicy{
		maxBytes:    10 << 20,
		maxBackups:  5,
		maxAge:      14 * 24 * time.Hour,
		lockTimeout: 5 * time.Second,
	})
}

func newLogger(path string, level slog.Level, stderr io.Writer, policy rotationPolicy) (*slog.Logger, func() error, error) {
	if stderr == nil {
		return nil, nil, os.ErrInvalid
	}
	file, err := newRotatingWriter(path, policy)
	if err != nil {
		return nil, nil, err
	}
	options := &slog.HandlerOptions{Level: level}
	fallback := slog.New(slog.NewJSONHandler(stderr, nil)).With("pid", os.Getpid())
	logger := slog.New(slog.NewMultiHandler(
		slog.NewJSONHandler(stderr, options),
		&reportingHandler{Handler: slog.NewJSONHandler(file, options), fallback: fallback},
	)).With("pid", os.Getpid())
	return logger, file.Close, nil
}

// slog 默认忽略 Handler 的写入错误，额外向 stderr 报告文件写入故障。
type reportingHandler struct {
	slog.Handler
	fallback *slog.Logger
}

func (h *reportingHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.Handler.Handle(ctx, record)
	if err != nil {
		h.fallback.ErrorContext(ctx, "写入日志文件失败", "error", err)
	}
	return err
}

func (h *reportingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &reportingHandler{Handler: h.Handler.WithAttrs(attrs), fallback: h.fallback}
}

func (h *reportingHandler) WithGroup(name string) slog.Handler {
	return &reportingHandler{Handler: h.Handler.WithGroup(name), fallback: h.fallback}
}
