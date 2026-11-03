package migrations_test

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestSettingsMigrationPreservesOldRunAndRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// 从当前映射去掉第四次迁移新增结构，得到真实的版本 3 表结构。
	if err := db.DB.Migrator().DropTable(&schema.Settings{}); err != nil {
		t.Fatal(err)
	}
	var indexes []struct{ SQL string }
	if err := db.DB.Raw("SELECT sql FROM sqlite_schema WHERE type='index' AND tbl_name='conversation_runs' AND sql IS NOT NULL").Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	// 仅还原空测试库时关闭外键；GORM 重建自关联表后恢复索引和外键检查。
	if err := db.DB.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Migrator().DropConstraint(&schema.Run{}, "ck_run_settings"); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Migrator().DropColumn(&schema.Run{}, "SettingsSnapshot"); err != nil {
		t.Fatal(err)
	}
	// GORM 的测试用重建会丢失 STRICT，使用其生成的旧字段 DDL 恢复空表属性。
	var ddl string
	if err := db.DB.Raw("SELECT sql FROM sqlite_schema WHERE type='table' AND name='conversation_runs'").Scan(&ddl).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("DROP TABLE conversation_runs").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec(ddl + " STRICT").Error; err != nil {
		t.Fatal(err)
	}
	for _, index := range indexes {
		if err := db.DB.Exec(index.SQL).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DB.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("PRAGMA user_version=3").Error; err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	snapshot := model.Snapshot{ID: id(), RemoteModelID: "old", Provider: model.ProviderSnapshot{ID: id(), Name: "旧厂商"}}
	conversation := schema.Conversation{ID: id(), Title: "保留会话", TitleSource: "manual", Model: snapshot, CreatedAt: 1, UpdatedAt: 1}
	if err := db.DB.Omit("Workspace").Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	run := schema.Run{ID: id(), ConversationID: conversation.ID, Model: snapshot, Status: "completed", CreatedAt: 1, UpdatedAt: 2, FinishedAt: new(int64(2))}
	if err := db.DB.Omit("Conversation", "Replaces", "SettingsSnapshot").Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"CREATE TABLE occupied_settings(id TEXT)", "CREATE UNIQUE INDEX uq_settings_key ON occupied_settings(id)"} {
		if err := db.DB.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if failed, err := sqlite.Open(t.Context(), path); err == nil {
		failed.Close()
		t.Fatal("迁移忽略了索引冲突")
	}
	var version int
	if err := db.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 3 || db.DB.Migrator().HasTable(&schema.Settings{}) || db.DB.Migrator().HasColumn(&schema.Run{}, "SettingsSnapshot") {
		t.Fatal("迁移失败没有回滚完整结构")
	}
	if err := db.DB.Exec("DROP INDEX uq_settings_key").Error; err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	repository, err := store.NewRepository(upgraded.DB)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repository.Get(t.Context(), run.ID)
	if err != nil || saved.SettingsSnapshot != nil || saved.Model.RemoteModelID != "old" || saved.Status != "completed" {
		t.Fatalf("升级改写或伪造了旧执行: %+v %v", saved, err)
	}
	var record schema.Settings
	if err := upgraded.DB.Where("key=?", settings.ExecutionKey).Take(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.Revision != 1 {
		t.Fatalf("默认设置版本错误: %+v", record)
	}
	parsed, err := settings.DecodeExecution(record.Value)
	if err != nil || parsed != settings.DefaultExecution() {
		t.Fatalf("默认配置不正确: %+v %v", parsed, err)
	}
	for _, raw := range []string{`{}`, `[]`, `{"revision":0,"approval_policy":"always","approvals_reviewer":"user"}`, `{"revision":1,"approval_policy":"always","approvals_reviewer":"auto_review"}`} {
		if err := upgraded.DB.Model(&schema.Run{}).Where("id=?", run.ID).Update("settings_snapshot", raw).Error; err == nil {
			t.Fatalf("升级的执行表缺少快照约束: %s", raw)
		}
	}
	if err := upgraded.DB.Model(&schema.Run{}).Where("id=?", run.ID).Update("settings_snapshot", `{"revision":1,"approval_policy":"always","approvals_reviewer":"user"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := upgraded.DB.Exec("DROP INDEX uq_settings_key").Error; err != nil {
		t.Fatal(err)
	}
	if broken, err := sqlite.Open(t.Context(), path); err == nil {
		broken.Close()
		t.Fatal("当前版本接受了缺少唯一索引的设置表")
	}
}

func TestSettingsAndSnapshotConstraints(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, change := range []map[string]any{{"revision": 0}, {"value": "[]"}, {"value": "not-json"}, {"id": "invalid"}} {
		if err := db.DB.Model(&schema.Settings{}).Where("key=?", settings.ExecutionKey).Updates(change).Error; err == nil {
			t.Fatalf("设置表接受了无效数据: %v", change)
		}
	}
	var row schema.Settings
	if err := db.DB.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	row.ID = uuid.Must(uuid.NewV7()).String()
	if err := db.DB.Create(&row).Error; err == nil {
		t.Fatal("允许同一配置组存在多条记录")
	}
}
