package attachment

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/model"
)

func TestAccessCopiesCurrentConversationAndCleansUp(t *testing.T) {
	ctx := t.Context()
	data := t.TempDir()
	store, err := Open(ctx, data, func(context.Context, string, string) (bool, error) { return true, nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	conv, run := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	original := filepath.Join(t.TempDir(), "report.csv")
	if err := os.WriteFile(original, []byte("name,total\nA,42\n"), 0600); err != nil {
		t.Fatal(err)
	}
	blocks := []model.Block{{Type: "file", Path: original, Name: "report.csv"}}
	batch, err := store.Stage(ctx, conv, run, blocks)
	if err != nil {
		t.Fatal(err)
	}
	batch.Finish(ctx, true)
	current := true
	resolve := func(_ context.Context, id, ref string) (model.Block, error) {
		if current && id == conv && ref == blocks[0].AttachmentRef {
			return blocks[0], nil
		}
		return model.Block{}, model.ErrAttachment
	}
	access, err := OpenAccess(ctx, data, store, resolve)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := access.Close(); err != nil {
			t.Error(err)
		}
	})
	files, err := access.Begin(ctx, conv, run)
	if err != nil {
		t.Fatal(err)
	}
	block, pinned, err := access.Metadata(ctx, conv, run, blocks[0].AttachmentRef)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pinned.Close(); err != nil {
			t.Error(err)
		}
	})
	path, work, err := access.Export(ctx, conv, run, block, pinned)
	if err != nil || work != files.Work || filepath.Base(path) != "report.csv" {
		t.Fatalf("副本导出失败: %s %s %v", path, work, err)
	}
	if err := os.Remove(original); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "42") {
		t.Fatalf("副本不独立: %s %v", content, err)
	}
	other := uuid.Must(uuid.NewV7()).String()
	if _, file, err := access.Metadata(ctx, other, run, block.AttachmentRef); err == nil {
		if file != nil {
			_ = file.Close()
		}
		t.Fatal("跨会话附件被允许")
	}
	current = false
	if _, _, err := access.Export(ctx, conv, run, block, pinned); err == nil {
		t.Fatal("被替换消息仍可导出")
	}
	generated, err := access.SaveGenerated(ctx, conv, run, "file", "result.txt", []byte("retained result"), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := access.SaveGenerated(ctx, other, run, "file", "result.txt", nil, true); err == nil {
		t.Fatal("跨会话生成物被允许")
	}
	if _, err := access.SaveGenerated(ctx, conv, run, "file", "../result.txt", nil, true); err == nil {
		t.Fatal("非法交付名称被允许")
	}
	if value, ok := access.Files(conv, run); !ok || len(value.Artifacts) != 1 {
		t.Fatalf("交付记录丢失: %+v", value)
	}
	if err := access.End(run); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(work); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("结束后未回收: %v", err)
	}
	if err := access.End(run); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "attachments", filepath.FromSlash(block.AttachmentRef))); err != nil {
		t.Fatalf("原件被清理: %v", err)
	}
	if data, _, err := store.Read(ctx, generated.AttachmentRef); err != nil || string(data) != "retained result" {
		t.Fatalf("结束后交付物不可读: %q %v", data, err)
	}
}

func TestAccessRecoversOnlyRecordedPrivateDirectories(t *testing.T) {
	data := t.TempDir()
	store, err := Open(t.Context(), data, func(context.Context, string, string) (bool, error) { return true, nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	resolve := func(context.Context, string, string) (model.Block, error) { return model.Block{}, model.ErrAttachment }
	first, err := OpenAccess(t.Context(), data, store, resolve)
	if err != nil {
		t.Fatal(err)
	}
	conv, run := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	files, err := first.Begin(t.Context(), conv, run)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenAccess(t.Context(), data, store, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(files.Work); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("异常退出的工作目录未恢复: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
}
