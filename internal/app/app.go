package app

import (
	"context"
	"database/sql"
	"sync"
)

// App 持有业务装配创建的资源。
type App struct {
	db        *sql.DB
	closeOnce sync.Once
	closeErr  error
}

func (a *App) Run(ctx context.Context) error {
	// 当前只等待取消，业务运行逻辑随功能逐步接入。
	<-ctx.Done()
	return ctx.Err()
}

func (a *App) Close() error {
	a.closeOnce.Do(func() { a.closeErr = a.db.Close() })
	return a.closeErr
}
