package prepare

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/filelock"
)

func TestPreparationLockIdentityAndRelease(t *testing.T) {
	directory := t.TempDir()
	first, err := acquirePreparationLock(directory, "target")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := acquirePreparationLock(directory, "target"); !errors.Is(err, filelock.ErrLocked) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("并发准备未互斥: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquirePreparationLock(directory, "target")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	info, err := os.Lstat(filepath.Join(directory, ".prepare-target.lock"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("锁文件未保留: %v", err)
	}
}

func TestPreparationPreservesLegacyDirectoryLock(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, ".prepare-target.lock")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if file, err := acquirePreparationLock(directory, "target"); err == nil || !strings.Contains(err.Error(), "旧格式") {
		if file != nil {
			file.Close()
		}
		t.Fatalf("旧锁被接受: %v", err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("旧锁被移除: %v", err)
	}
}
