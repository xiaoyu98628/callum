package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/stdio"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

// Run 持有数据库、HTTP 连接池和输入输出，初始化失败或运行结束时均负责清理。
// cfg 必须由配置模块计算，ctx 必须是服务运行上下文。
func Run(ctx context.Context, cfg config.Config, in io.ReadCloser, out io.WriteCloser, logger *slog.Logger) (retErr error) {
	if in == nil || out == nil {
		return errors.New("输入输出不能为空")
	}
	owned := true
	defer func() {
		if owned {
			retErr = errors.Join(retErr, in.Close(), out.Close())
		}
	}()
	if logger == nil || cfg.MaxConcurrentRuns < 1 || cfg.MaxConcurrentRuns > 64 || cfg.CallTimeout <= 0 {
		return errors.New("应用运行配置无效")
	}
	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := sqlite.Open(initCtx, cfg.DatabasePath)
	initCancel()
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()
	repository, err := sqlite.NewCatalogRepository(db.DB)
	if err != nil {
		return err
	}
	providers, err := provider.NewService(repository)
	if err != nil {
		return err
	}
	client, transport := newHTTPClient(cfg.MaxConcurrentRuns)
	defer transport.CloseIdleConnections()
	resolve := newModelResolver(providers, client)
	connection, err := stdio.NewConnection(ctx, in, out)
	if err != nil {
		return err
	}
	owned = false
	defer func() { retErr = errors.Join(retErr, connection.Close()) }()
	runs, err := inference.New(connection.Context(), resolve, cfg.MaxConcurrentRuns, cfg.CallTimeout, connection.Emit)
	if err != nil {
		return err
	}
	// 依次停止任务、排空协议输出，再关闭 HTTP 连接池与数据库。
	defer runs.Close()
	server, err := stdio.New(providers, runs, logger)
	if err != nil {
		return err
	}
	return server.Serve(connection)
}
