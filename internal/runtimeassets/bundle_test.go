package runtimeassets_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
)

func fixture(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "runtime with spaces")
	for _, name := range []string{"python/bin/python3", "node/bin/node", "python/lib/stdlib.py"} {
		full := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("fixture-content"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	files, err := runtimeassets.Inventory(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeassets.WriteManifest(directory, runtimeassets.Manifest{FormatVersion: 1, Platform: runtime.GOOS, Arch: runtime.GOARCH, Python: runtimeassets.Executable{Version: "3.14.10", Path: "python/bin/python3"}, Node: runtimeassets.Executable{Version: "26.13.0", Path: "node/bin/node"}, Files: files}); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestBundleRejectsMissingChangedAndAdditionalResources(t *testing.T) {
	for _, change := range []string{"library", "executable", "missing", "additional"} {
		t.Run(change, func(t *testing.T) {
			directory := fixture(t)
			bundle, err := runtimeassets.Load(t.Context(), directory)
			if err != nil {
				t.Fatal(err)
			}
			if err := bundle.Check(t.Context()); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"python", "python3", "node"} {
				entry, ok := bundle.Resolve(name)
				if !ok || !filepath.IsAbs(entry) {
					t.Fatalf("入口没有固定为绝对路径: %q", entry)
				}
			}
			if _, ok := bundle.Resolve("./python"); ok {
				t.Fatal("显式路径被裸命令映射覆盖")
			}
			name := "python/lib/stdlib.py"
			if change == "executable" {
				name = "python/bin/python3"
			} else if change == "additional" {
				name = "python/lib/sitecustomize.py"
			}
			path := filepath.Join(directory, filepath.FromSlash(name))
			if change == "missing" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, []byte("changed-content"), 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtimeassets.Load(t.Context(), directory); err == nil {
				t.Fatal("资源变更后仍接受清单")
			}
			if change == "executable" && bundle.Check(t.Context()) == nil {
				t.Fatal("调用前未发现解释器被改写")
			}
		})
	}
}

func TestBundleContextAndEscapingLink(t *testing.T) {
	directory := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := runtimeassets.Load(ctx, directory); !errors.Is(err, context.Canceled) {
		t.Fatalf("忽略初始化取消: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows 符号链接权限由原生验收覆盖")
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "python", "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeassets.Inventory(t.Context(), directory); err == nil {
		t.Fatal("接受了资源外链接")
	}
}

func TestBundleRejectsDifferentArchitecture(t *testing.T) {
	directory := fixture(t)
	path := filepath.Join(directory, "manifest.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, decodeErr := runtimeassets.Decode(file)
	if err := errors.Join(decodeErr, file.Close()); err != nil {
		t.Fatal(err)
	}
	manifest.Arch = "arm64"
	if runtime.GOARCH == "arm64" {
		manifest.Arch = "amd64"
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtimeassets.WriteManifest(directory, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeassets.Load(t.Context(), directory); err == nil {
		t.Fatal("接受了不同架构的资源")
	}
}
