//go:build !release

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentDefaultDataDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module \"github.com/xiaoyu98628/callum\" // Callum\n"), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "internal", "config")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	// 开发默认路径无需访问用户家目录。
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(root, ".callum", "dev") || cfg.DatabasePath != filepath.Join(root, ".callum", "dev", "callum.db") {
		t.Fatalf("开发默认路径错误: %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(root, ".callum")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("路径计算不应创建目录: %v", err)
	}
}

func TestDevelopmentUsesCallumModuleRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/xiaoyu98628/callum\n"), 0600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "nested")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "go.mod"), []byte("module example.com/other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(other)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(root, ".callum", "dev") {
		t.Fatalf("错误地使用了其他模块的目录: %+v", cfg)
	}
}

func TestDevelopmentMissingRootDoesNotFallBackToHome(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "go.mod"), []byte("module example.com/other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(outside)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, err := Load(""); err == nil {
		t.Fatal("未找到 Callum 根目录时应返回错误")
	}
	if _, err := os.Stat(filepath.Join(home, ".callum")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("开发路径失败后不应创建家目录数据: %v", err)
	}
}
