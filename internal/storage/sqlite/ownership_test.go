package sqlite

import (
	"path/filepath"
	"testing"
)

func TestOwnershipExcludesSecondRuntimeAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	first, err := AcquireOwnership(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	if other, err := AcquireOwnership(path); err == nil {
		other.Close()
		t.Fatal("第二个服务实例取得运行权")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireOwnership(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
