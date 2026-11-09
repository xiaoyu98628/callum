package workspace_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/workspace"
	"github.com/xiaoyu98628/callum/internal/workspace"
)

func TestWorkspaceFoldersRollback(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repo, err := store.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	root, other := t.TempDir(), t.TempDir()
	input := workspace.Workspace{ID: uuid.Must(uuid.NewV7()).String(), Name: "项目", RootPath: root, OtherPaths: []string{other, other}}
	if _, err := repo.Create(ctx, input); err == nil {
		t.Fatal("项目目录唯一约束未生效")
	}
	if _, err := repo.Get(ctx, input.ID); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("目录写入失败后项目仍然存在: %v", err)
	}
	input.OtherPaths = []string{other}
	if _, err := repo.Create(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Update(ctx, input.ID, workspace.Change{Deleted: new(true)}); err != nil {
		t.Fatal(err)
	}
	input.Name = "恢复名称"
	input.OtherPaths = []string{other, other}
	if _, err := repo.Create(ctx, input); err == nil {
		t.Fatal("恢复时接受了重复目录")
	}
	value, err := repo.Get(ctx, input.ID)
	if err != nil || value.DeletedAt == nil || value.Name != "项目" || !slices.Equal(value.OtherPaths, []string{other}) {
		t.Fatalf("恢复失败后状态发生变化: %+v %v", value, err)
	}
}
