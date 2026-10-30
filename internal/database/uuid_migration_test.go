package database

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func legacyDatabase(t *testing.T) (string, *gorm.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	if err := prepareFile(path); err != nil {
		t.Fatal(err)
	}
	pool, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	db, err := gorm.Open(gormsqlite.New(gormsqlite.Config{DriverName: "sqlite", Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE model_providers (id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,protocol TEXT NOT NULL,base_url TEXT NOT NULL,api_key TEXT NOT NULL,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL) STRICT`,
		`CREATE TABLE models (id INTEGER PRIMARY KEY AUTOINCREMENT,provider_id INTEGER NOT NULL REFERENCES model_providers(id) ON DELETE RESTRICT,display_name TEXT NOT NULL,remote_model_id TEXT NOT NULL,enabled INTEGER NOT NULL,context_window INTEGER,max_output_tokens INTEGER,capabilities TEXT,model_type TEXT NOT NULL,reasoning_levels TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL) STRICT`,
		`CREATE UNIQUE INDEX uq_provider_remote_model ON models(provider_id,remote_model_id)`,
		`INSERT INTO model_providers VALUES(8,'旧连接','chatcompletions','https://example.invalid/v1','fake-legacy-key',0,100)`,
		`INSERT INTO models VALUES(12,8,'旧模型','legacy-remote',0,NULL,512,'{ "tools": false }','[]',NULL,0,200)`,
		`INSERT INTO models VALUES(13,8,'空元数据','other-remote',1,2048,NULL,'{}','[]','[]',10,300)`,
		`PRAGMA user_version = 1`,
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	return path, db, pool
}

func TestUUIDMigrationPreservesDataAndRelationships(t *testing.T) {
	path, db, pool := legacyDatabase(t)
	if err := migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, err := provider.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := store.ListProviders(t.Context())
	if err != nil || len(providers) != 1 {
		t.Fatalf("厂商丢失: %v", err)
	}
	pid := providers[0].ID
	if !provider.ValidID(pid) || providers[0].CreatedAt != 0 || providers[0].UpdatedAt != 100 {
		t.Fatalf("厂商迁移错误: %+v", providers)
	}
	models, err := store.ListModels(t.Context(), false)
	if err != nil || len(models) != 2 {
		t.Fatalf("模型丢失: %v", err)
	}
	var legacyID string
	for _, m := range models {
		if !provider.ValidID(m.ID) || m.ProviderID != pid {
			t.Fatalf("关联错误: %+v", m)
		}
		if m.RemoteModelID == "legacy-remote" {
			legacyID = m.ID
			if m.Enabled || m.CreatedAt != 0 || m.UpdatedAt != 200 || m.ContextWindow != nil || m.MaxOutputTokens == nil || *m.MaxOutputTokens != 512 || m.ReasoningLevels != nil {
				t.Fatalf("配置被改变: %+v", m)
			}
		} else if m.Capabilities == nil || m.ReasoningLevels == nil {
			t.Fatalf("空元数据被改变: %+v", m)
		}
	}
	var raw string
	if err := db.Raw("SELECT capabilities FROM models WHERE id = ?", legacyID).Scan(&raw).Error; err != nil || raw != `{ "tools": false }` {
		t.Fatalf("JSON 文本未原样保留: %q %v", raw, err)
	}
	if err := store.SetModelEnabled(t.Context(), legacyID, true); err != nil {
		t.Fatal(err)
	}
	runtime, err := store.Resolve(t.Context(), legacyID)
	if err != nil || runtime.APIKey != "fake-legacy-key" || runtime.Model.RemoteModelID != "legacy-remote" {
		t.Fatalf("调用配置丢失: %v", err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	var stable string
	if err := reopened.DB.Raw("SELECT id FROM model_providers").Scan(&stable).Error; err != nil || stable != pid {
		t.Fatalf("重开后 UUID 改变: %v", err)
	}
}

func TestUUIDMigrationRollbackRestoresIntegerSchema(t *testing.T) {
	_, db, _ := legacyDatabase(t)
	// 模拟已有异常数据，使失败发生在新表和厂商记录已创建之后。
	if err := db.Exec("UPDATE models SET context_window = 0 WHERE id = 12").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate(t.Context(), db); err == nil {
		t.Fatal("迁移应拒绝异常数据")
	}
	var version int
	if err := db.Raw("PRAGMA user_version").Scan(&version).Error; err != nil || version != 1 {
		t.Fatalf("版本未回滚: %d %v", version, err)
	}
	var id, providerID int64
	if err := db.Raw("SELECT id,provider_id FROM models WHERE id=12").Row().Scan(&id, &providerID); err != nil || id != 12 || providerID != 8 {
		t.Fatalf("旧记录被改变: %v", err)
	}
	var extras int
	if err := db.Raw("SELECT count(*) FROM sqlite_schema WHERE name IN ('callum_models_v1','callum_model_providers_v1')").Scan(&extras).Error; err != nil || extras != 0 {
		t.Fatalf("留下了迁移临时表: %v", err)
	}
	if !db.Migrator().HasIndex("models", "uq_provider_remote_model") {
		t.Fatal("旧索引未恢复")
	}
	if err := db.Exec("UPDATE models SET context_window = NULL WHERE id=12").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate(t.Context(), db); err != nil {
		t.Fatalf("修复数据后无法重试: %v", err)
	}
}

func TestConcurrentUUIDMigration(t *testing.T) {
	path, _, pool := legacyDatabase(t)
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			db, err := Open(t.Context(), path)
			if err != nil {
				t.Error(err)
				return
			}
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	var count int
	if err := db.DB.Raw("SELECT count(*) FROM models").Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("并发迁移改变记录数: %d %v", count, err)
	}
}
