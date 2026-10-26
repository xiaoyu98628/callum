package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
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
	application, err := app.Compose(startupCtx, cfg)
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

	logger.Info("Callum 已启动", "data_dir", cfg.DataDir, "database", cfg.DatabasePath())

	err = application.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
