package fileaccess_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

func TestFixedHandleOwnershipAndCancellation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note"), []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	scope := tool.Scope{ID: "w", RootPath: dir}
	for _, test := range []struct {
		name, path string
		kind       fileaccess.Kind
	}{
		{"file", "note", fileaccess.File},
		{"directory", ".", fileaccess.Directory},
	} {
		t.Run(test.name, func(t *testing.T) {
			handle, err := fileaccess.Prepare(t.Context(), scope, test.path, test.kind, tool.ProtectedData{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := handle.Close(); err != nil {
					t.Error(err)
				}
			})
			original := *handle.Target()
			handle.Target().Path = "changed"
			if *handle.Target() != original || !original.WithinWorkspace {
				t.Fatal("目标副本可修改固定身份，或工作区分类丢失")
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if err := handle.Check(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("目标检查未保留取消: %v", err)
			}
			if err := handle.Check(t.Context()); err != nil {
				t.Fatal(err)
			}
			if test.kind == fileaccess.File {
				if _, err := fileaccess.ReadText(ctx, handle.File()); !errors.Is(err, context.Canceled) {
					t.Fatalf("读取未保留取消: %v", err)
				}
				if _, err := handle.File().Stat(); err != nil {
					t.Fatalf("取消读取关闭了借用句柄: %v", err)
				}
			}
			if err := handle.Close(); err != nil {
				t.Fatal(err)
			}
			if err := handle.Close(); err != nil {
				t.Fatalf("重复关闭失败: %v", err)
			}
			if test.kind == fileaccess.File {
				if _, err := handle.File().Stat(); !errors.Is(err, os.ErrClosed) {
					t.Fatalf("固定文件未关闭: %v", err)
				}
			} else if _, err := handle.Root().Stat("."); err == nil {
				t.Fatal("固定目录未关闭")
			}
		})
	}
	for _, test := range []struct {
		path string
		kind fileaccess.Kind
	}{
		{"note", fileaccess.Directory},
		{".", fileaccess.File},
	} {
		handle, err := fileaccess.Prepare(t.Context(), scope, test.path, test.kind, tool.ProtectedData{})
		if handle != nil {
			if err := handle.Close(); err != nil {
				t.Error(err)
			}
		}
		if !errors.Is(err, tool.ErrUnavailable) {
			t.Fatalf("接受不匹配的目标类型: %s, %v", test.path, err)
		}
	}
}
