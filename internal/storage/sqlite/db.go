package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Open 返回可复用的连接池，调用者负责 Close。
// 每个进程的连接数限制为 1；不同进程之间由 SQLite 自身协调文件锁。
func Open(ctx context.Context, path string) (_ *sql.DB, openErr error) {
	if ctx == nil {
		return nil, errors.New("数据库上下文不能为空")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("数据库路径必须是绝对路径")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("数据库路径必须指向普通文件")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("检查数据库文件失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}
	// 明确新数据库文件的访问权限，不改变已有文件的权限。
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("打开数据库文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	query := url.Values{}
	// DSN 配置会应用到每一个新连接，包括连接池重新建立的连接。
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(FULL)")
	uri.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("初始化 SQLite 失败: %w", err)
	}
	success := false
	defer func() {
		if !success {
			openErr = errors.Join(openErr, db.Close())
		}
	}()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("连接 SQLite 失败: %w", err)
	}
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return nil, fmt.Errorf("检查 SQLite 日志模式失败: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		return nil, fmt.Errorf("SQLite 未启用 WAL 模式，实际为 %s", mode)
	}
	success = true
	return db, nil
}
