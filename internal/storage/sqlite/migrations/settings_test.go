package migrations_test

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

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
