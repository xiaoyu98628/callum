package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaoyu98628/callum/internal/app"
	"github.com/xiaoyu98628/callum/internal/config"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := config.LoadArgs(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		config.WriteUsage(os.Stderr)
		return 0
	}
	if err != nil {
		logger.Error("读取启动配置失败", "module", "main", "error", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// stdout 管道中断时通过 Write 错误进入资源清理，不由 SIGPIPE 直接终止进程。
	signal.Ignore(syscall.SIGPIPE)
	in, out, err := openStdio()
	if err != nil {
		logger.Error("初始化标准输入输出失败", "module", "main", "error", err)
		return 1
	}
	if err := app.Run(ctx, cfg, in, out, logger); err != nil {
		logger.Error("服务退出失败", "module", "main", "error", err)
		return 1
	}
	return 0
}
