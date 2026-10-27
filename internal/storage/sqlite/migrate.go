package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version  int64
	name     string
	checksum string
	script   string
}

// Migrate 按编号执行程序内嵌的迁移，调用者继续持有并负责关闭 db。
// 待执行的 SQL 与版本记录在同一事务中提交；执行最多等待 10 秒，同时受 ctx 限制。
func Migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	return migrate(ctx, db, migrationFiles, logger)
}

func migrate(ctx context.Context, db *sql.DB, source fs.FS, logger *slog.Logger) error {
	if ctx == nil || db == nil || logger == nil {
		return errors.New("迁移上下文、数据库和日志不能为空")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	logger = logger.With("component", "sqlite.migration")
	started := time.Now()
	logger.InfoContext(ctx, "开始检查数据库迁移")
	migrations, err := loadMigrations(source)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for attempt := 1; ; attempt++ {
		applied, retry, err := migrateTransaction(ctx, db, migrations)
		if err == nil {
			// 事务整体提交后再报告成功，避免后续 SQL 失败回滚时误报前面的迁移。
			for _, migration := range migrations[len(migrations)-applied:] {
				logger.InfoContext(ctx, "数据库迁移已提交", "version", migration.version, "file", migration.name)
			}
			message := "数据库迁移完成"
			if applied == 0 {
				message = "数据库迁移无需更新"
			}
			logger.InfoContext(ctx, message,
				"current_version", migrations[len(migrations)-1].version,
				"applied_count", applied, "duration_ms", time.Since(started).Milliseconds(),
			)
			return nil
		}
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		if !retry {
			return err
		}
		logger.DebugContext(ctx, "数据库迁移锁冲突，等待重试", "attempt", attempt, "error", err)
		// WAL 的读事务升级为写事务可能直接返回 BUSY_SNAPSHOT，必须整笔回滚重试。
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
	}
}

func loadMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, fmt.Errorf("读取迁移目录失败: %w", err)
	}
	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()
		prefix, description, ok := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if !ok || description == "" || err != nil || version <= 0 || prefix != fmt.Sprintf("%03d", version) {
			return nil, fmt.Errorf("迁移文件名无效，应为 001_description.sql: %s", name)
		}
		content, err := fs.ReadFile(source, "migrations/"+name)
		if err != nil {
			return nil, fmt.Errorf("读取迁移文件 %s 失败: %w", name, err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return nil, fmt.Errorf("迁移文件为空: %s", name)
		}
		migrations = append(migrations, migration{
			version: version, name: name,
			checksum: fmt.Sprintf("%x", sha256.Sum256(content)), script: string(content),
		})
	}
	if len(migrations) == 0 {
		return nil, errors.New("没有可执行的迁移文件")
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for index, migration := range migrations {
		if migration.version != int64(index+1) {
			return nil, errors.New("迁移编号必须从 001 开始连续递增，不能重复或缺失")
		}
	}
	return migrations, nil
}

func migrateTransaction(ctx context.Context, db *sql.DB, migrations []migration) (applied int, retry bool, migrationErr error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, migrationBusy(err), fmt.Errorf("开始迁移事务失败: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			retry = false
			migrationErr = errors.Join(migrationErr, fmt.Errorf("回滚迁移失败: %w", err))
		}
	}()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER NOT NULL PRIMARY KEY CHECK (version > 0),
		name TEXT NOT NULL UNIQUE,
		checksum TEXT NOT NULL,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return 0, migrationBusy(err), fmt.Errorf("创建迁移记录表失败: %w", err)
	}
	// 即使没有待执行的迁移，也先取得 SQLite 写锁，再读取版本，串行协调各个进程。
	if _, err := tx.ExecContext(ctx, "UPDATE schema_migrations SET version = version WHERE 0"); err != nil {
		return 0, migrationBusy(err), fmt.Errorf("获取迁移写锁失败: %w", err)
	}
	previous, err := validateMigrationHistory(ctx, tx, migrations)
	if err != nil {
		return 0, migrationBusy(err), err
	}
	for _, migration := range migrations[previous:] {
		if _, err := tx.ExecContext(ctx, migration.script); err != nil {
			return 0, migrationBusy(err), fmt.Errorf("执行迁移 %s 失败: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (?, ?, ?, ?)",
			migration.version, migration.name, migration.checksum, time.Now().UnixMilli(),
		); err != nil {
			return 0, migrationBusy(err), fmt.Errorf("保存迁移 %s 的记录失败: %w", migration.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, migrationBusy(err), fmt.Errorf("提交迁移失败: %w", err)
	}
	return len(migrations) - previous, false, nil
}

func validateMigrationHistory(ctx context.Context, tx *sql.Tx, migrations []migration) (applied int, historyErr error) {
	rows, err := tx.QueryContext(ctx, "SELECT version, name, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return 0, fmt.Errorf("读取迁移记录失败: %w", err)
	}
	defer func() { historyErr = errors.Join(historyErr, rows.Close()) }()
	for rows.Next() {
		var version int64
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return 0, err
		}
		if applied >= len(migrations) || migrations[applied].version != version {
			return 0, fmt.Errorf("数据库迁移记录与当前程序不兼容，版本为 %d", version)
		}
		if migrations[applied].name != name || migrations[applied].checksum != checksum {
			return 0, fmt.Errorf("已执行的迁移文件被修改: %s", name)
		}
		applied++
	}
	return applied, rows.Err()
}

func migrationBusy(err error) bool {
	var sqliteErr *driver.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	// 扩展错误码的低 8 位是主错误码，包含 SQLITE_BUSY_SNAPSHOT 等情况。
	code := sqliteErr.Code() & 0xff
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}
