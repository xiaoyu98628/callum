package migrations_test

import (
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestVersionNumberDoesNotHideMissingConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("DROP INDEX uq_active_conversation").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := sqlite.Open(t.Context(), path); err == nil {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("仅凭版本号接受缺少互斥约束的数据库")
	}
}
