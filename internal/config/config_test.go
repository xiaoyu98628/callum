package config

import (
	"path/filepath"
	"testing"
)

func TestExplicitDataDirDoesNotReadHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Chdir(t.TempDir())
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != dir || cfg.DatabasePath != filepath.Join(dir, "callum.db") {
		t.Fatalf("路径错误: %+v", cfg)
	}
}

func TestExplicitRelativeDataDir(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	cfg, err := Load(filepath.Join(".callum", "custom"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(root, ".callum", "custom") || cfg.DatabasePath != filepath.Join(root, ".callum", "custom", "callum.db") {
		t.Fatalf("显式相对路径错误: %+v", cfg)
	}
}
