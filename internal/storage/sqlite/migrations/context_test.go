package migrations_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestContextMigrationPreservesSettingsAndRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	original := []byte(`{"approval_policy":"never","approvals_reviewer":"user"}`)
	if err := db.DB.Exec("UPDATE app_settings SET value=?,revision=7 WHERE key='execution'", string(original)).Error; err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"DROP TABLE conversation_context_versions",
		"DELETE FROM app_settings WHERE key='context_compaction'",
		"PRAGMA user_version=4",
		"CREATE TABLE occupied_context(id TEXT)",
		"CREATE UNIQUE INDEX uq_context_version ON occupied_context(id)",
	} {
		if err := db.DB.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if opened, err := sqlite.Open(t.Context(), path); err == nil {
		opened.Close()
		t.Fatal("迁移忽略索引冲突")
	}
	var version int
	if err := db.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.DB.Model(&schema.Settings{}).Where("key=?", settings.CompactionKey).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if version != 4 || count != 0 || db.DB.Migrator().HasTable(&schema.ContextVersion{}) {
		t.Fatal("迁移失败未原子回滚")
	}
	if err := db.DB.Exec("DROP INDEX uq_context_version").Error; err != nil {
		t.Fatal(err)
	}
	opened, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	}()
	var old, added schema.Settings
	if err := opened.DB.Where("key=?", settings.ExecutionKey).Take(&old).Error; err != nil {
		t.Fatal(err)
	}
	if old.Revision != 7 || !bytes.Equal(old.Value, original) {
		t.Fatal("迁移改写权限配置或版本")
	}
	if err := opened.DB.Where("key=?", settings.CompactionKey).Take(&added).Error; err != nil {
		t.Fatal(err)
	}
	var cfg settings.Compaction
	if err := json.Unmarshal(added.Value, &cfg); err != nil || cfg != settings.DefaultCompaction() {
		t.Fatalf("压缩默认值错误: %+v %v", cfg, err)
	}
	if err := opened.DB.Exec("DROP INDEX uq_context_active").Error; err != nil {
		t.Fatal(err)
	}
	if invalid, err := sqlite.Open(t.Context(), path); err == nil {
		invalid.Close()
		t.Fatal("接受缺少有效版本唯一索引的数据库")
	}
}
