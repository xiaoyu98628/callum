package filelock

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLockHelper(t *testing.T) {
	path := os.Getenv("CALLUM_TEST_LOCK_PATH")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := Try(file); err != nil {
		t.Fatal(err)
	}
	fmt.Println("locked")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func TestLockExcludesAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	first, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := Try(first); err != nil {
		t.Fatal(err)
	}
	if err := Try(second); !errors.Is(err, ErrLocked) {
		t.Fatalf("未互斥: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Try(second); err != nil {
		t.Fatal(err)
	}
	if err := Try(nil); !errors.Is(err, os.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestLockReleasedAfterProcessKilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.lock")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestLockHelper$")
	cmd.Env = append(os.Environ(), "CALLUM_TEST_LOCK_PATH="+path)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("子进程未持锁: %q %v", line, err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := Try(file); !errors.Is(err, ErrLocked) {
		t.Fatalf("未保持互斥: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("强制退出未生效")
	}
	if err := Try(file); err != nil {
		t.Fatalf("崩溃后未释放锁: %v", err)
	}
}
