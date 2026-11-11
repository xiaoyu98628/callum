// prepare-runtimes 维护固定版本归档；默认离线，显式 --download 才允许下载。
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	"github.com/xiaoyu98628/callum/internal/runtimeassets/prepare"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("module", "runtimeassets.prepare")
	err := prepare.RunCommand(ctx, os.Args[1:], logger, false)
	cancel()
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		logger.Error("运行时资源准备失败", "error", err)
		os.Exit(1)
	}
}
