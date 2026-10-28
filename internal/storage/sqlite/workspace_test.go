package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/workspace"
)

func workspaceTestStore(t *testing.T, path string) (*sql.DB, *WorkspaceStore) {
	t.Helper()
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := Migrate(t.Context(), db, discardMigrationLogger()); err != nil {
		t.Fatal(err)
	}
	orm, err := NewORM(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewWorkspaceStore(orm)
	if err != nil {
		t.Fatal(err)
	}
	return db, store
}

func TestWorkspaceStoreDuplicateAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.db")
	db, store := workspaceTestStore(t, path)
	pinned, archived := int64(10), int64(20)
	original := workspace.Workspace{ID: "original", Name: "保留的名称", RootPath: t.TempDir(),
		PinnedAt: &pinned, ArchivedAt: &archived, CreatedAt: 100, UpdatedAt: 200}
	if _, err := store.Create(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	duplicate := workspace.Workspace{ID: "another", Name: "新名称", RootPath: original.RootPath, CreatedAt: 300, UpdatedAt: 300}
	got, err := store.Create(t.Context(), duplicate)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("duplicate changed original: %+v, %v", got, err)
	}
	duplicate.ID, duplicate.RootPath = original.ID, t.TempDir()
	if _, err := store.Create(t.Context(), duplicate); err == nil {
		t.Fatal("ID collision was silently treated as a path conflict")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, reopened := workspaceTestStore(t, path)
	items, err := reopened.List(t.Context(), workspace.ListOptions{IncludeArchived: true})
	if err != nil || !reflect.DeepEqual(items, []workspace.Workspace{original}) {
		t.Fatalf("reopened data differs: %+v, %v", items, err)
	}
}

func TestWorkspaceStorePreservesExplicitTimes(t *testing.T) {
	db, store := workspaceTestStore(t, filepath.Join(t.TempDir(), "callum.db"))
	original := workspace.Workspace{ID: "zero-times", Name: "零时间", RootPath: t.TempDir()}
	got, err := store.Create(t.Context(), original)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("ORM changed explicit timestamp values: %+v, %v", got, err)
	}
	var created, updated int64
	if err := db.QueryRowContext(t.Context(), "SELECT created_at, updated_at FROM workspaces WHERE id = ?", original.ID).
		Scan(&created, &updated); err != nil || created != 0 || updated != 0 {
		t.Fatalf("ORM assigned automatic timestamps: created=%d, updated=%d, error=%v", created, updated, err)
	}
}

func TestWorkspaceStoreListOrder(t *testing.T) {
	_, store := workspaceTestStore(t, filepath.Join(t.TempDir(), "callum.db"))
	items, err := store.List(t.Context(), workspace.ListOptions{})
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty list: %+v, %v", items, err)
	}
	high, low, zero, archived := int64(300), int64(100), int64(0), int64(1)
	items = []workspace.Workspace{
		{ID: "old", UpdatedAt: 100},
		{ID: "b", UpdatedAt: 500},
		{ID: "a", UpdatedAt: 500},
		{ID: "pinned-old", PinnedAt: &low, UpdatedAt: 999},
		{ID: "pinned-new", PinnedAt: &high, UpdatedAt: 1},
		{ID: "pinned-zero", PinnedAt: &zero, UpdatedAt: 9999},
		{ID: "archived", ArchivedAt: &archived, UpdatedAt: 700},
	}
	root := t.TempDir()
	for _, item := range items {
		item.Name, item.RootPath, item.CreatedAt = item.ID, filepath.Join(root, item.ID), 1
		if _, err := store.Create(t.Context(), item); err != nil {
			t.Fatal(err)
		}
	}
	for _, includeArchived := range []bool{false, true} {
		got, err := store.List(t.Context(), workspace.ListOptions{IncludeArchived: includeArchived})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, item := range got {
			ids = append(ids, item.ID)
		}
		want := []string{"pinned-new", "pinned-old", "pinned-zero", "a", "b", "old"}
		if includeArchived {
			want = []string{"pinned-new", "pinned-old", "pinned-zero", "archived", "a", "b", "old"}
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("includeArchived=%v: got %v, want %v", includeArchived, ids, want)
		}
	}
}

func TestWorkspaceStoreCancellation(t *testing.T) {
	db, store := workspaceTestStore(t, filepath.Join(t.TempDir(), "callum.db"))
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if _, err := store.Create(ctx, workspace.Workspace{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pool wait did not cancel: %v", err)
	}
	if _, err := store.List(ctx, workspace.ListOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("list ignored cancellation: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(t.Context(), workspace.ListOptions{}); err != nil {
		t.Fatalf("pool did not recover: %v", err)
	}
}

func TestWorkspaceCreateProcess(t *testing.T) {
	if os.Getenv("CALLUM_WORKSPACE_HELPER") != "1" {
		return
	}
	args := os.Args[len(os.Args)-2:]
	db, err := Open(t.Context(), args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	orm, err := NewORM(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewWorkspaceStore(orm)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 8 {
		candidate := workspace.Workspace{ID: fmt.Sprintf("%d-%d", os.Getpid(), index), Name: "project",
			RootPath: args[1], CreatedAt: 1, UpdatedAt: 1}
		if _, err := store.Create(t.Context(), candidate); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceStoreAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.db")
	_, store := workspaceTestStore(t, path)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkspaceCreateProcess$", "--", path, root)
			cmd.Env = append(os.Environ(), "CALLUM_WORKSPACE_HELPER=1")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("workspace process: %v\n%s", err, output)
			}
		})
	}
	workers.Wait()
	items, err := store.List(t.Context(), workspace.ListOptions{})
	if err != nil || len(items) != 1 || items[0].RootPath != root {
		t.Fatalf("concurrent creates did not converge: %+v, %v", items, err)
	}
}
