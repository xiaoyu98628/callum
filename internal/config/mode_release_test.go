//go:build release

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseDefaultDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(home, ".callum") || cfg.DatabasePath != filepath.Join(home, ".callum", "callum.db") {
		t.Fatalf("发布默认路径错误: %+v", cfg)
	}
	if _, err := os.Stat(cfg.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("路径解析不应创建数据目录: %v", err)
	}
}

func TestReleaseMissingHomeReturnsError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Chdir(t.TempDir())
	if _, err := Load(""); err == nil {
		t.Fatal("家目录缺失时应返回错误")
	}
}

func TestReleaseRejectsRelativeHome(t *testing.T) {
	t.Setenv("HOME", "relative-home")
	t.Setenv("USERPROFILE", "relative-home")
	t.Chdir(t.TempDir())
	if _, err := Load(""); err == nil {
		t.Fatal("家目录为相对路径时应返回错误")
	}
}
