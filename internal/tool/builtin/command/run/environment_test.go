package run

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSandboxCachesUsePrivateDirectory(t *testing.T) {
	directory := t.TempDir()
	source := []string{"HOME=user-home", "TMPDIR=old", "GOCACHE=old", "npm_config_cache=old", "PIP_CACHE_DIR=old"}
	if runtime.GOOS == "windows" {
		source = append(source, "gocache=lowercase-old", "tmpdir=lowercase-old")
	}
	environment := sandboxEnvironment(source, directory)
	for _, key := range []string{"TMPDIR", "TMP", "TEMP", "GOCACHE", "GOTMPDIR", "npm_config_cache", "PIP_CACHE_DIR"} {
		want := directory
		if key == "GOCACHE" {
			want = filepath.Join(directory, "go-build")
		}
		if key == "npm_config_cache" {
			want = filepath.Join(directory, "npm-cache")
		}
		count := 0
		for _, entry := range environment {
			name, value, _ := strings.Cut(entry, "=")
			matches := name == key || (runtime.GOOS == "windows" && strings.EqualFold(name, key))
			if matches {
				count++
				if value != want {
					t.Fatalf("缓存仍指向非私有目录: %s", entry)
				}
			}
		}
		if count != 1 {
			t.Fatalf("缓存配置重复或缺失: %s %d", key, count)
		}
	}
	if environmentValue(environment, "HOME") != "user-home" {
		t.Fatal("非缓存环境被改写")
	}
}
