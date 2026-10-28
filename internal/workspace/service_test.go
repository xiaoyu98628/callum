package workspace

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type recordingStore struct {
	saved    Workspace
	existing *Workspace
	options  ListOptions
	items    []Workspace
	err      error
	calls    int
}

func (s *recordingStore) Create(_ context.Context, item Workspace) (Workspace, error) {
	s.calls++
	s.saved = item
	if s.existing != nil {
		return *s.existing, s.err
	}
	return item, s.err
}

func (s *recordingStore) List(_ context.Context, options ListOptions) ([]Workspace, error) {
	s.calls++
	s.options = options
	return s.items, s.err
}

func testService(t *testing.T, store Store) *Service {
	t.Helper()
	service, err := NewService(store, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestCreateWorkspaceDirectory(t *testing.T) {
	root := t.TempDir()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingStore{}
	service := testService(t, store)
	before := time.Now().UnixMilli()
	item, err := service.Create(t.Context(), CreateInput{RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || item.RootPath != canonical || item.Name != filepath.Base(canonical) ||
		item.CreatedAt < before || item.CreatedAt > time.Now().UnixMilli() || item.UpdatedAt != item.CreatedAt ||
		item.PinnedAt != nil || item.ArchivedAt != nil {
		t.Fatalf("unexpected new workspace: %+v", item)
	}
	store.existing = &item
	again, err := service.Create(t.Context(), CreateInput{RootPath: root, Name: "不会覆盖原名称"})
	if err != nil || again != item {
		t.Fatalf("duplicate changed workspace: %+v, %v", again, err)
	}
	store.existing = nil
	named, err := service.Create(t.Context(), CreateInput{RootPath: root, Name: "  项目名称  "})
	if err != nil || named.Name != "项目名称" {
		t.Fatalf("explicit name: %+v, %v", named, err)
	}
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("无法创建符号链接: %v", err)
	}
	linked, err := service.Create(t.Context(), CreateInput{RootPath: link})
	if err != nil || linked.RootPath != canonical {
		t.Fatalf("symlink was not canonicalized: %+v, %v", linked, err)
	}
}

func TestCreateWorkspaceInvalidPath(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "relative/project", file, filepath.Join(root, "missing")} {
		t.Run(path, func(t *testing.T) {
			store := &recordingStore{}
			_, err := testService(t, store).Create(t.Context(), CreateInput{RootPath: path})
			if !errors.Is(err, ErrInvalidInput) || store.calls != 0 {
				t.Fatalf("invalid path reached storage: calls=%d, error=%v", store.calls, err)
			}
		})
	}
}

func TestWorkspaceContextAndStorageErrors(t *testing.T) {
	store := &recordingStore{}
	service := testService(t, store)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.Create(ctx, CreateInput{RootPath: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := service.List(ctx, ListOptions{}); !errors.Is(err, context.Canceled) || store.calls != 0 {
		t.Fatalf("cancelled operation reached storage: %v", err)
	}
	if _, err := service.List(nil, ListOptions{}); err == nil {
		t.Fatal("accepted nil context")
	}
	storageErr := errors.New("storage unavailable")
	store.err = storageErr
	if _, err := service.Create(t.Context(), CreateInput{RootPath: t.TempDir()}); !errors.Is(err, storageErr) {
		t.Fatalf("lost create error: %v", err)
	}
	if _, err := service.List(t.Context(), ListOptions{}); !errors.Is(err, storageErr) {
		t.Fatalf("lost list error: %v", err)
	}
	store.err = nil
	items, err := service.List(t.Context(), ListOptions{IncludeArchived: true})
	if err != nil || items == nil || len(items) != 0 || !store.options.IncludeArchived {
		t.Fatalf("empty list or options: items=%v, error=%v", items, err)
	}
}
