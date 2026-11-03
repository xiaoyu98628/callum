package migrations_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
)

func TestUpgradeConversationPreservesConfigAndRollsBack(t *testing.T) {
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
	repo, err := catalog.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	service, err := providerservice.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := service.CreateProvider(t.Context(), provider.ProviderInput{Name: "保留厂商", Protocol: "chatcompletions", BaseURL: "https://example.test/v1", APIKey: new("fake-preserved-key")})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := service.CreateModel(t.Context(), provider.ModelInput{ProviderID: pid, DisplayName: "保留模型", RemoteModelID: "remote", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	// 测试库还原为真实版本 1 的两张配置表，模拟已有用户配置。
	for _, table := range []string{"conversation_run_approvals", "conversation_run_steps", "conversation_messages", "conversation_runs", "conversations", "workspaces"} {
		if err := db.DB.Exec("DROP TABLE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DB.Exec("PRAGMA user_version=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("CREATE TABLE occupied (id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("CREATE INDEX uq_message_sequence ON occupied(id)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := sqlite.Open(t.Context(), path); err == nil {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("迁移索引冲突没有失败")
	}
	// 使用测试连接检查失败未留下半套业务表或改变旧版本。
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := check.Close(); err != nil {
			t.Error(err)
		}
	}()
	var version, count int
	if err := check.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := check.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sqlite_schema WHERE name IN ('workspaces','conversations','conversation_runs','conversation_messages','conversation_run_steps')").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if version != 1 || count != 0 {
		t.Fatalf("升级未回滚: version=%d tables=%d", version, count)
	}
	if _, err := check.ExecContext(t.Context(), "DROP INDEX uq_message_sequence"); err != nil {
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
	upgradedRepo, err := catalog.NewRepository(upgraded.DB)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := upgradedRepo.LoadRuntime(t.Context(), mid)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Model.ID != mid || runtime.Model.ProviderID != pid || runtime.ProviderName != "保留厂商" || runtime.APIKey != "fake-preserved-key" {
		t.Fatal("升级改写了已有模型配置")
	}
	if upgraded.DB.Migrator().HasColumn("conversation_messages", "parent_message_id") {
		t.Fatal("保留了已取消的父消息字段")
	}
	if err := upgraded.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("升级版本错误: %d", version)
	}
}
