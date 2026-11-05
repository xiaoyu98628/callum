package migrations_test

import (
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
)

func TestApprovalUpgradeRollbackAndPreserve(t *testing.T) {
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
	repo, err := catalog.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	service, err := providerservice.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := service.CreateProvider(t.Context(), provider.ProviderInput{Name: "保留厂商", Protocol: "chatcompletions", BaseURL: "https://example.test/v1", APIKey: new("fake-key")})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"DROP TABLE conversation_directory_grants", "DROP TABLE conversation_context_versions", "DROP TABLE app_settings", "DROP TABLE conversation_run_approvals", "DROP INDEX uq_step_run", "PRAGMA user_version=2", "CREATE TABLE occupied(id TEXT)", "CREATE UNIQUE INDEX uq_approval_step ON occupied(id)"} {
		if err := db.DB.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if failed, err := sqlite.Open(t.Context(), path); err == nil {
		failed.Close()
		t.Fatal("索引冲突未回滚")
	}
	var version int
	if err := db.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 2 || db.DB.Migrator().HasTable("conversation_run_approvals") || db.DB.Migrator().HasIndex("conversation_run_steps", "uq_step_run") {
		t.Fatal("审批迁移留下半套结构")
	}
	if err := db.DB.Exec("DROP INDEX uq_approval_step").Error; err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if err := upgraded.DB.Raw("PRAGMA user_version").Scan(&version).Error; err != nil || version != 6 {
		t.Fatalf("迁移未完成: %d %v", version, err)
	}
	if upgraded.DB.Migrator().HasColumn("conversation_run_approvals", "expires_at") {
		t.Fatal("新建审批表仍包含过期字段")
	}
	var count int64
	if err := upgraded.DB.Table("model_providers").Where("id=?", pid).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("升级丢失配置")
	}
	if err := upgraded.DB.Exec("DROP INDEX uq_approval_step").Error; err != nil {
		t.Fatal(err)
	}
	if broken, err := sqlite.Open(t.Context(), path); err == nil {
		broken.Close()
		t.Fatal("版本 3 接受缺少唯一约束的审批表")
	}
	if err := upgraded.DB.Exec("CREATE INDEX uq_approval_step ON conversation_run_approvals(step_id)").Error; err != nil {
		t.Fatal(err)
	}
	if broken, err := sqlite.Open(t.Context(), path); err == nil {
		broken.Close()
		t.Fatal("接受同名但非唯一的审批索引")
	}

}
