package migrations_test

import (
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestDirectoryMigrationRollbackAndVersionFiveUpgrade(t *testing.T) {
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
	for _, sql := range []string{"DROP TABLE conversation_directory_grants", "PRAGMA user_version=5", "CREATE TABLE occupied_directory(id TEXT)", "CREATE UNIQUE INDEX uq_directory_grant_approval ON occupied_directory(id)"} {
		if err := db.DB.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if opened, err := sqlite.Open(t.Context(), path); err == nil {
		opened.Close()
		t.Fatal("目录迁移忽略索引冲突")
	}
	var version int
	if err := db.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 5 || db.DB.Migrator().HasTable(&schema.DirectoryGrant{}) {
		t.Fatal("目录迁移失败未回滚")
	}
	if err := db.DB.Exec("DROP INDEX uq_directory_grant_approval").Error; err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := upgraded.Close(); err != nil {
			t.Error(err)
		}
	}()
	var count int64
	if err := upgraded.DB.Model(&schema.DirectoryGrant{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("旧批准被隐式转换: %d %v", count, err)
	}
	if err := upgraded.DB.Exec("DROP INDEX uq_directory_grant_approval").Error; err != nil {
		t.Fatal(err)
	}
	if opened, err := sqlite.Open(t.Context(), path); err == nil {
		opened.Close()
		t.Fatal("接受损坏目录授权索引")
	}
}
