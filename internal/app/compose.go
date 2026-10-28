package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/transport/stdio"
	"github.com/xiaoyu98628/callum/internal/workspace"
)

// Compose 初始化数据库、工作空间业务和通信处理；数据库由返回的 App 持有并关闭。
func Compose(ctx context.Context, cfg config.Config, logger *slog.Logger) (_ *App, composeErr error) {
	if logger == nil {
		return nil, errors.New("应用日志不能为空")
	}
	db, err := sqlite.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			composeErr = errors.Join(composeErr, db.Close())
		}
	}()
	if err := sqlite.Migrate(ctx, db, logger.With("database", cfg.DatabasePath())); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	orm, err := sqlite.NewORM(ctx, db)
	if err != nil {
		return nil, err
	}
	store, err := sqlite.NewWorkspaceStore(orm)
	if err != nil {
		return nil, err
	}
	workspaces, err := workspace.NewService(store, logger)
	if err != nil {
		return nil, err
	}
	server, err := stdio.NewServer(workspaces, logger)
	if err != nil {
		return nil, err
	}
	success = true
	return &App{db: db, server: server}, nil
}
