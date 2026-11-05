package migrations_test

import (
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestTokenCompactionMigrationPreservesHistoryAndRollsBack(t *testing.T) {
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
	old := `{"enabled":false,"message_threshold":64,"byte_threshold":2048,"retain_turns":2}`
	var oldSnapshot settings.CompactionSnapshot
	if err := json.Unmarshal([]byte(`{"revision":7,"enabled":false,"message_threshold":64,"byte_threshold":2048,"retain_turns":2}`), &oldSnapshot); err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	identity := model.Snapshot{ID: id(), RemoteModelID: "historical", Provider: model.ProviderSnapshot{ID: id(), Name: "厂商"}}
	conv := schema.Conversation{ID: id(), Title: "历史会话", TitleSource: "manual", Model: identity, CreatedAt: 1, UpdatedAt: 1}
	if err := db.DB.Omit("Workspace").Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	run := schema.Run{ID: id(), ConversationID: conv.ID, Model: identity, SettingsSnapshot: &settings.Snapshot{Revision: 3, Execution: settings.DefaultExecution(), Compaction: &oldSnapshot}, Status: "completed", CreatedAt: 1, UpdatedAt: 2, FinishedAt: new(int64(2))}
	if err := db.DB.Omit("Conversation", "Replaces").Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	var original string
	if err := db.DB.Raw("SELECT settings_snapshot FROM conversation_runs WHERE id=?", run.ID).Scan(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("UPDATE app_settings SET value=?,revision=? WHERE key=?", old, int64(math.MaxInt64), settings.CompactionKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("PRAGMA user_version=5").Error; err != nil {
		t.Fatal(err)
	}
	if opened, err := sqlite.Open(t.Context(), path); err == nil {
		opened.Close()
		t.Fatal("不可递增设置版本仍完成迁移")
	}
	var version int
	if err := db.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	var row schema.Settings
	if err := db.DB.Where("key=?", settings.CompactionKey).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if version != 5 || string(row.Value) != old || row.Revision != math.MaxInt64 {
		t.Fatal("失败迁移改写了版本或旧设置")
	}
	if err := db.DB.Exec("UPDATE app_settings SET revision=7 WHERE key=?", settings.CompactionKey).Error; err != nil {
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
	if err := opened.DB.Where("key=?", settings.CompactionKey).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.DecodeCompaction(row.Value)
	if err != nil || cfg.Enabled || cfg.RetainTurns != 2 || cfg.TokenBudget != 0 || row.Revision != 8 {
		t.Fatalf("旧设置升级错误: %s %v", row.Value, err)
	}
	if err := opened.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil || version != 6 {
		t.Fatal("数据库版本没有升级")
	}
	// 同时核对物理 JSON 和业务解码，不把新策略倒填到旧执行。
	var preserved string
	if err := opened.DB.Raw("SELECT settings_snapshot FROM conversation_runs WHERE id=?", run.ID).Scan(&preserved).Error; err != nil {
		t.Fatal(err)
	}
	var saved schema.Run
	if err := opened.DB.Where("id=?", run.ID).Take(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if preserved != original || saved.SettingsSnapshot == nil || !saved.SettingsSnapshot.Valid() || saved.SettingsSnapshot.Compaction.Revision != 7 {
		t.Fatal("历史执行快照被改写或不可读取")
	}
}
