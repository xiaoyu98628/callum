package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sqliteDialect "github.com/libtnb/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// NewORM 复用已经由 Open 初始化的连接池，不负责创建或关闭数据库。
// 表结构继续由 Migrate 管理；业务操作需要通过 WithContext 传递各自的上下文。
func NewORM(ctx context.Context, db *sql.DB) (*gorm.DB, error) {
	if ctx == nil || db == nil {
		return nil, errors.New("ORM 上下文和数据库不能为空")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	initialPool := ormInitPool{ConnPool: db, ctx: ctx}
	dialect := &sqliteDialect.Dialector{Conn: initialPool}
	orm, err := gorm.Open(dialect, &gorm.Config{
		// Open 已完成受上下文约束的连接检查，避免 GORM 再执行不带上下文的 Ping。
		DisableAutomaticPing: true,
		// SQL 和错误继续由应用日志边界管理，禁止 ORM 默认输出污染 stdio。
		Logger: gormlogger.Discard,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 ORM 失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 初始化上下文只约束初始化，后续请求使用原连接池及各自的上下文。
	// 同时恢复 *sql.DB 的事务能力，事务内部仍由 GORM 使用对应的 *sql.Tx。
	dialect.Conn = db
	orm.ConnPool = db
	orm.Statement.ConnPool = db
	return orm, nil
}

// 适配器的版本查询使用 Background；初始化期间转接为调用方的上下文。
// 只暴露 ConnPool 方法，避免 GORM 初始化失败时取得并关闭调用者的 *sql.DB。
type ormInitPool struct {
	gorm.ConnPool
	ctx context.Context
}

func (p ormInitPool) QueryRowContext(_ context.Context, query string, args ...any) *sql.Row {
	return p.ConnPool.QueryRowContext(p.ctx, query, args...)
}
