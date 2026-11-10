package workspace_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
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

func TestWorkspacePrimaryDirectoryTransaction(t *testing.T) {
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
	project, err := repo.Create(ctx, workspace.Workspace{ID: uuid.Must(uuid.NewV7()).String(), Name: "项目", RootPath: root, OtherPaths: []string{other}})
	if err != nil {
		t.Fatal(err)
	}
	conv := schema.Conversation{ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: &project.ID, Title: "保留的会话"}
	if err := db.DB.WithContext(ctx).Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	run := schema.Run{ID: uuid.Must(uuid.NewV7()).String(), ConversationID: conv.ID, Status: "running"}
	if err := db.DB.WithContext(ctx).Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"running", "awaiting_approval", "awaiting_user_input"} {
		if err := db.DB.WithContext(ctx).Model(&run).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Update(ctx, project.ID, workspace.Change{Name: new("不得保存"), RootPath: &other}); !errors.Is(err, workspace.ErrBusy) {
			t.Fatalf("%s 时没有阻止主目录切换: %v", status, err)
		}
		if _, err := repo.Update(ctx, project.ID, workspace.Change{RootPath: &root, OtherPaths: new([]string{}), ExpectedFolders: new([]string{root, other})}); !errors.Is(err, workspace.ErrBusy) {
			t.Fatalf("%s 时允许增删源目录: %v", status, err)
		}
	}
	// 未改变主目录的保存仍可在执行期间改名。
	if _, err := repo.Update(ctx, project.ID, workspace.Change{Name: new("已改名"), RootPath: &root}); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.WithContext(ctx).Model(&run).Updates(map[string]any{"status": "cancelled", "finished_at": 1}).Error; err != nil {
		t.Fatal(err)
	}
	value, err := repo.Update(ctx, project.ID, workspace.Change{RootPath: &other})
	if err != nil || value.ID != project.ID || value.Name != "已改名" || value.RootPath != other || !slices.Equal(value.OtherPaths, []string{root}) {
		t.Fatalf("切换主目录未保留项目及完整目录集合: %+v %v", value, err)
	}
	// 在附加目录更新后强制主表写入失败，验证两表及名称同时回滚。
	if err := db.DB.WithContext(ctx).Exec(`CREATE TRIGGER reject_root_change BEFORE UPDATE OF root_path ON workspaces BEGIN SELECT RAISE(ABORT, 'controlled failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Update(ctx, project.ID, workspace.Change{Name: new("回滚名称"), RootPath: &root}); err == nil {
		t.Fatal("主表写入失败未返回错误")
	}
	value, err = repo.Get(ctx, project.ID)
	if err != nil || value.Name != "已改名" || value.RootPath != other || !slices.Equal(value.OtherPaths, []string{root}) {
		t.Fatalf("失败后主目录、附加目录或名称发生变化: %+v %v", value, err)
	}
}
