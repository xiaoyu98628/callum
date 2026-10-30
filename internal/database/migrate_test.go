package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMigrationRollback(t *testing.T) {
	pool, err := sql.Open("sqlite", ":memory:?_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	defer func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	}()
	db, err := gorm.Open(gormsqlite.New(gormsqlite.Config{DriverName: "sqlite", Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	// 冲突发生在建表之后的索引创建阶段，必须撤销之前已经创建的表。
	if err := db.Exec("CREATE TABLE occupied (id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX uq_provider_remote_model ON occupied(id)").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate(t.Context(), db); err == nil {
		t.Fatal("迁移应失败")
	}
	var version, tables int
	if err := db.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE name IN ('model_providers', 'models')").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if version != 0 || tables != 0 {
		t.Fatalf("迁移未回滚: version=%d tables=%d", version, tables)
	}
}

func TestFutureVersionPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DB.WithContext(t.Context()).Exec("PRAGMA user_version = 3").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
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
