package database

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestOpenHonorsDeadlineWhileLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	conn, err := db.pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := conn.ExecContext(t.Context(), "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := conn.ExecContext(t.Context(), "ROLLBACK"); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	other, err := Open(ctx, path)
	if other != nil {
		if err := other.Close(); err != nil {
			t.Error(err)
		}
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("初始化没有保留调用方的超时错误: %v", err)
	}
}

func TestExistingDataIsNotRebuilt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "callum.db")
	original := []byte("existing data that is not a SQLite database")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("接受了无效数据库")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("初始化失败后原文件被改写")
	}
}

func TestExistingPermissionsAreNotChanged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 访问控制使用 ACL")
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "callum.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("接受了权限过宽的文件")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatal("擅自改变了已有文件权限")
	}
}

func TestRejectsDatabaseAndDirectorySymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("创建 Windows 符号链接需要额外系统权限")
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(dir, "actual.db")
	if err := os.WriteFile(actual, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.db")
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), link); err == nil {
		t.Fatal("接受了数据库文件符号链接")
	}
	dirLink := filepath.Join(t.TempDir(), "linked-data")
	if err := os.Symlink(dir, dirLink); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), filepath.Join(dirLink, "callum.db")); err == nil {
		t.Fatal("接受了数据目录末级符号链接")
	}
}

func TestOpenAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		for p, mode := range map[string]os.FileMode{filepath.Dir(path): 0700, path: 0600} {
			info, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != mode {
				t.Fatalf("%s 权限为 %o", p, info.Mode().Perm())
			}
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Open(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("初始化取消: %v", err)
	}
	if _, err := Open(t.Context(), "relative.db"); err == nil {
		t.Fatal("接受了相对路径")
	}
}

func TestConcurrentInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			db, err := Open(t.Context(), path)
			if err != nil {
				t.Error(err)
				return
			}
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}
