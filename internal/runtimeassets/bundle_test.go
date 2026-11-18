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
	for _, name := range []string{"python/bin/python3", "node/bin/node", "python/lib/stdlib.py", "git/bin/git", "git/etc/gitconfig", "git/libexec/git-core/helper", "git/share/templates/template"} {
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
	if err := runtimeassets.WriteManifest(directory, runtimeassets.Manifest{FormatVersion: runtimeassets.FormatVersion, Platform: runtime.GOOS, Arch: runtime.GOARCH, Python: runtimeassets.Executable{Version: "3.14.10", Path: "python/bin/python3"}, Node: runtimeassets.Executable{Version: "26.13.0", Path: "node/bin/node"}, Git: runtimeassets.GitRuntime{Executable: runtimeassets.Executable{Version: "fixed", Path: "git/bin/git"}, ExecPath: "git/libexec/git-core", TemplatePath: "git/share/templates", ConfigPath: "git/etc/gitconfig"}, Files: files}); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestBundleRejectsMissingChangedAndAdditionalResources(t *testing.T) {
	for _, change := range []string{"library", "executable", "missing", "additional", "git", "git-config", "git-helper"} {
		t.Run(change, func(t *testing.T) {
			directory := fixture(t)
			bundle, err := runtimeassets.Load(t.Context(), directory)
			if err != nil {
				t.Fatal(err)
			}
			if err := bundle.Check(t.Context()); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"python", "python3", "node", "git"} {
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
			switch change {
			case "git":
				name = "git/bin/git"
			case "git-config":
				name = "git/etc/gitconfig"
			case "git-helper":
				name = "git/libexec/git-core/helper"
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
			if (change == "executable" || change == "git" || change == "git-config") && bundle.Check(t.Context()) == nil {
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

func TestBundleRejectsInvalidGitLayoutAndOldFormat(t *testing.T) {
	for _, change := range []string{"outside", "missing", "file-as-directory", "old-format"} {
		t.Run(change, func(t *testing.T) {
			directory := fixture(t)
			file, err := os.Open(filepath.Join(directory, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			manifest, decodeErr := runtimeassets.Decode(file)
			if err := errors.Join(decodeErr, file.Close()); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "outside":
				manifest.Git.ExecPath = "../git-core"
			case "missing":
				manifest.Git.TemplatePath = "git/missing"
			case "file-as-directory":
				manifest.Git.ExecPath = manifest.Git.ConfigPath
				manifest.Git.TemplatePath = manifest.Git.ConfigPath
			case "old-format":
				manifest.FormatVersion = 1
			}
			if err := os.Chmod(filepath.Join(directory, "manifest.json"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := runtimeassets.WriteManifest(directory, manifest); err != nil {
				t.Fatal(err)
			}
			if _, err := runtimeassets.Load(t.Context(), directory); err == nil {
				t.Fatal("接受了无效 Git 配套路径或旧清单")
			}
		})
	}
}
