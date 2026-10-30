package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite/migrations"
	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite"
)

// Database 持有 GORM 实例及其底层连接池；创建者负责调用 Close。
type Database struct {
	DB   *gorm.DB
	pool *sql.DB
}

// Close 关闭底层连接池，可重复调用。
func (d *Database) Close() error { return d.pool.Close() }

// Open 使用绝对路径打开并迁移数据库，失败时关闭已创建的连接池。
// 初始化上下文只用于本次初始化；后续操作必须传递各自的调用上下文。
func Open(ctx context.Context, path string) (*Database, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("数据库路径必须是绝对路径")
	}
	if err := prepareFile(path); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	pool, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	if err := pool.PingContext(ctx); err != nil {
		return nil, errors.Join(err, pool.Close())
	}
	// 使用官方 SQLite 方言，实际连接由 modernc 驱动提供。
	db, err := gorm.Open(gormsqlite.New(gormsqlite.Config{DriverName: "sqlite", Conn: pool}), &gorm.Config{
		DisableAutomaticPing: true,
		// 错误交给调用边界处理，避免 ORM 默认日志打印 SQL 参数中的凭据。
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, errors.Join(err, pool.Close())
	}
	if err := migrations.Run(ctx, db, &providerRecord{}, &modelRecord{}); err != nil {
		return nil, errors.Join(err, pool.Close())
	}
	// 不将初始化上下文绑定在返回的 GORM 实例上。
	return &Database{DB: db, pool: pool}, nil
}
