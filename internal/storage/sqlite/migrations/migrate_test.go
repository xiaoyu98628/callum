package migrations_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

// 通过存储包的公开入口验证真实表映射，不在迁移测试中重复定义产品表结构。
func fixtureDatabase(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "callum.db")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	pool, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	return path, pool
}

func TestMigrationRollback(t *testing.T) {
	path, pool := fixtureDatabase(t)
	// 冲突发生在建表之后的索引创建阶段，必须撤销之前已经创建的表。
	if _, err := pool.ExecContext(t.Context(), "CREATE TABLE occupied (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.ExecContext(t.Context(), "CREATE INDEX uq_provider_remote_model ON occupied(id)"); err != nil {
		t.Fatal(err)
	}
	opened, openErr := sqlite.Open(t.Context(), path)
	if opened != nil {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	}
	if openErr == nil || !strings.Contains(openErr.Error(), "uq_provider_remote_model") {
		t.Fatalf("迁移未在预期的索引创建阶段失败: %v", openErr)
	}
	var version, tables int
	if err := pool.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE name IN ('model_providers', 'models')").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if version != 0 || tables != 0 {
		t.Fatalf("迁移未回滚: version=%d tables=%d", version, tables)
	}
	// 移除测试制造的索引冲突后，具体迁移应能重新执行并完整提交。
	if _, err := pool.ExecContext(t.Context(), "DROP INDEX uq_provider_remote_model"); err != nil {
		t.Fatal(err)
	}
	retried, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("移除冲突后无法重新初始化: %v", err)
	}
	t.Cleanup(func() {
		if err := retried.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := pool.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE name IN ('model_providers', 'models')").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if version != 2 || tables != 2 {
		t.Fatalf("具体迁移未完整提交: version=%d tables=%d", version, tables)
	}
}

func TestFutureVersionPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	var initializedVersion int
	if err := db.DB.Raw("PRAGMA user_version").Scan(&initializedVersion).Error; err != nil {
		t.Fatal(err)
	}
	if initializedVersion != 2 {
		t.Fatalf("新库结构版本错误: %d", initializedVersion)
	}
	if err := db.DB.WithContext(t.Context()).Exec("PRAGMA user_version = 3").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlite.Open(t.Context(), path); err == nil {
		t.Fatal("接受了更高版本")
	}
	pool, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	}()
	var version int
	if err := pool.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatal("更高版本被修改")
	}
}

func TestUnsupportedExistingSchemaPreservesData(t *testing.T) {
	for _, test := range []struct {
		name      string
		version   int
		wantError string
	}{
		{name: "缺少结构版本", version: 0, wantError: "缺少结构版本"},
		{name: "整数主键结构", version: 1, wantError: "不支持数据库结构版本 1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, pool := fixtureDatabase(t)
			// 仅构造版本检查所需的旧结构和数据，不保留旧结构升级逻辑。
			const originalSchema = "CREATE TABLE model_providers (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"
			if _, err := pool.ExecContext(t.Context(), originalSchema); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.ExecContext(t.Context(), "INSERT INTO model_providers (id,name) VALUES (?,?)", 8, "保留配置"); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.ExecContext(t.Context(), fmt.Sprintf("PRAGMA user_version = %d", test.version)); err != nil {
				t.Fatal(err)
			}
			opened, openErr := sqlite.Open(t.Context(), path)
			if opened != nil {
				if err := opened.Close(); err != nil {
					t.Error(err)
				}
			}
			if openErr == nil || !strings.Contains(openErr.Error(), test.wantError) {
				t.Fatalf("未明确拒绝旧结构: %v", openErr)
			}
			var version int
			if err := pool.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
				t.Fatal(err)
			}
			var name, schema string
			if err := pool.QueryRowContext(t.Context(), "SELECT name FROM model_providers WHERE id = ?", 8).Scan(&name); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRowContext(t.Context(), "SELECT sql FROM sqlite_schema WHERE name = 'model_providers'").Scan(&schema); err != nil {
				t.Fatal(err)
			}
			var modelTables int
			if err := pool.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE name = 'models'").Scan(&modelTables); err != nil {
				t.Fatal(err)
			}
			if version != test.version || name != "保留配置" || schema != originalSchema || modelTables != 0 {
				t.Fatalf("拒绝打开后原数据或结构被改写: version=%d name=%q schema=%q models=%d", version, name, schema, modelTables)
			}
		})
	}
}
