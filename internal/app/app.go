package app

import (
	"context"
	"database/sql"
	"io"
	"sync"

	"github.com/xiaoyu98628/callum/internal/transport/stdio"
)

// App 持有业务装配创建的资源。
type App struct {
	db        *sql.DB
	server    *stdio.Server
	closeOnce sync.Once
	closeErr  error
}

// Run 处理 stdio 请求，接管输入输出的关闭责任；数据库仍由 Close 关闭。
func (a *App) Run(ctx context.Context, input io.ReadCloser, output io.WriteCloser) error {
	return a.server.Run(ctx, input, output)
}

func (a *App) Close() error {
	a.closeOnce.Do(func() { a.closeErr = a.db.Close() })
	return a.closeErr
}
