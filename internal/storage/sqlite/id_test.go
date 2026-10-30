package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestUUIDPrimaryKeysAndConstraints(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	store, err := newService(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 8 {
		pid, err := store.CreateProvider(t.Context(), provider.ProviderInput{Name: "厂商", Protocol: "chatcompletions", BaseURL: "https://example.invalid"})
		if err != nil {
			t.Fatal(err)
		}
		id, err := store.CreateModel(t.Context(), provider.ModelInput{ProviderID: pid, DisplayName: "模型", RemoteModelID: "remote", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{pid, id} {
			if !provider.ValidID(value) || seen[value] {
				t.Fatalf("UUID 版本或唯一性错误: %s", value)
			}
			seen[value] = true
		}
		if _, err := store.Resolve(t.Context(), id); err != nil {
			t.Fatalf("UUID 关联失败: %v", err)
		}
	}
	for _, bad := range []string{"1", "", "01900000-0000-4000-8000-000000000001", "01900000-0000-7000-c000-000000000001", "01900000-0000-7000-8000-00000000000G"} {
		if provider.ValidID(bad) {
			t.Fatalf("接受了非规范 UUIDv7: %q", bad)
		}
		if err := db.DB.Exec("INSERT INTO model_providers(id,name,protocol,base_url,api_key,created_at,updated_at) VALUES(?,'p','chatcompletions','https://example.invalid','',0,0)", bad).Error; err == nil {
			t.Fatalf("数据库未约束 UUIDv7: %q", bad)
		}
	}
}
