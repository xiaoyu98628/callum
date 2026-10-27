package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

// Compose 打开数据库并完成迁移；数据库由返回的 App 持有并关闭。
func Compose(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		return nil, errors.New("应用日志不能为空")
	}
	db, err := sqlite.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, err
	}
	if err := sqlite.Migrate(ctx, db, logger.With("database", cfg.DatabasePath())); err != nil {
		return nil, errors.Join(fmt.Errorf("数据库迁移失败: %w", err), db.Close())
	}
	return &App{db: db}, nil
}
