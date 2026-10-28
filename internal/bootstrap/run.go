package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xiaoyu98628/callum/internal/app"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/logging"
)

func Run(parent context.Context, args []string) (runErr error) {
	fallback := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("pid", os.Getpid())
	logger := fallback
	var closeLog func() error
	defer func() {
		if runErr != nil {
			logger.Error("Callum 运行失败", "error", runErr)
		}
		if closeLog != nil {
			if err := closeLog(); err != nil {
				fallback.Error("关闭日志文件失败", "error", err)
				runErr = errors.Join(runErr, err)
			}
		}
	}()

	if parent == nil {
		return errors.New("启动上下文不能为空")
	}
	if err := parent.Err(); err != nil {
		return err
	}
	cfg, err := config.Load(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(
		parent,
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	configuredLogger, cleanup, err := logging.New(cfg.LogPath(), cfg.LogLevel, os.Stderr)
	if err != nil {
		return err
	}
	logger, closeLog = configuredLogger, cleanup

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	application, err := app.Compose(startupCtx, cfg, logger)
	cancel()
	if err != nil {
		return fmt.Errorf("业务装配失败: %w", err)
	}
	defer func() {
		closeErr := application.Close()
		if closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("关闭数据库失败: %w", closeErr))
		}
		logger.Info("Callum 已停止")
	}()

	// 接管 SIGPIPE，让管道断开以写入错误返回，仍然经过数据库和日志清理。
	brokenPipe := make(chan os.Signal, 1)
	signal.Notify(brokenPipe, syscall.SIGPIPE)
	defer signal.Stop(brokenPipe)
	input, err := prepareStandardStream(os.Stdin, func(enabled bool) error {
		return syscall.SetNonblock(syscall.Stdin, enabled)
	})
	if err != nil {
		return err
	}
	output, err := prepareStandardStream(os.Stdout, func(enabled bool) error {
		return syscall.SetNonblock(syscall.Stdout, enabled)
	})
	if err != nil {
		return errors.Join(err, input.Close())
	}
	logger.Info("Callum 已启动", "data_dir", cfg.DataDir, "database", cfg.DatabasePath())
	err = application.Run(ctx, input, output)
	// 只有单独的取消是正常退出，与取消合并的管道清理错误仍需返回。
	if err == context.Canceled {
		return nil
	}
	return err
}

// 继承的阻塞标准管道不一定注册了 Go 的 I/O 轮询，直接 Close 可能等待读写。
// 重新包装可轮询的描述符，让上下文取消能够中断正在进行的管道操作。
func prepareStandardStream(file *os.File, setNonblock func(bool) error) (io.ReadWriteCloser, error) {
	if err := file.SetDeadline(time.Time{}); err == nil {
		return file, nil
	} else if !errors.Is(err, os.ErrNoDeadline) {
		return nil, fmt.Errorf("准备标准管道失败: %w", err)
	}
	fd := file.Fd()
	if err := setNonblock(true); err != nil {
		return nil, fmt.Errorf("设置标准管道非阻塞模式失败: %w", err)
	}
	stream := os.NewFile(fd, file.Name())
	if stream == nil {
		return nil, errors.Join(errors.New("包装标准管道失败"), setNonblock(false))
	}
	return &standardStream{File: stream, restore: func() error { return setNonblock(false) }}, nil
}

type standardStream struct {
	*os.File
	restore func() error
}

func (s *standardStream) Close() error {
	// 先停止轮询读写，再恢复继承的阻塞模式，避免影响父进程共享的终端描述符。
	err := s.SetDeadline(time.Now())
	if errors.Is(err, os.ErrNoDeadline) {
		err = nil
	}
	return errors.Join(err, s.restore(), s.File.Close())
}
