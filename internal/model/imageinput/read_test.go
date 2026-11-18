package imageinput

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model"
)

func TestImageFileReadRejectsReplacementAndCancellation(t *testing.T) {
	dir := t.TempDir()
	path, replacement := filepath.Join(dir, "input.png"), filepath.Join(dir, "replacement.png")
	for _, name := range []string{path, replacement} {
		if err := os.WriteFile(name, []byte("image-content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if content, err := readFile(t.Context(), func() (*os.File, error) { return os.OpenFile(path, readFlags, 0) }, expected); !errors.Is(err, model.ErrImageRead) || content != nil {
		t.Fatalf("替换后的文件仍被读取: bytes=%d err=%v", len(content), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readFile(ctx, func() (*os.File, error) { return os.OpenFile(path, readFlags, 0) }, expected); !errors.Is(err, context.Canceled) {
		t.Fatalf("打开图片前忽略取消: %v", err)
	}
}

func TestImageFileGrowthPreservesLimitError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.png")
	if err := os.WriteFile(path, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// 同一文件在大小检查后增长，仍由实际读取上限拒绝，不改成普通读取错误。
	if err := os.WriteFile(path, make([]byte, MaxSourceBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if content, err := readFile(t.Context(), func() (*os.File, error) { return os.OpenFile(path, readFlags, 0) }, expected); !errors.Is(err, model.ErrImageLimit) || errors.Is(err, model.ErrImageRead) || content != nil {
		t.Fatalf("文件增长后未保留输入限额错误: bytes=%d err=%v", len(content), err)
	}
}
