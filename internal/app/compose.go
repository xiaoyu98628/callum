package app

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

// Compose 创建应用级资源；数据库由返回的 App 持有并关闭。
func Compose(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := sqlite.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, err
	}
	return &App{db: db}, nil
}
