package logging

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func testPolicy() rotationPolicy {
	return rotationPolicy{maxBytes: 2048, maxBackups: 64, maxAge: 14 * 24 * time.Hour, lockTimeout: 5 * time.Second}
}

// TestLoggingProcess 以真实子进程测试文件锁，而不是以 goroutine 代替进程。
func TestLoggingProcess(t *testing.T) {
	if os.Getenv("CALLUM_LOGGING_HELPER") != "1" {
		return
	}
	args := os.Args[len(os.Args)-3:]
	mode, path, worker := args[0], args[1], args[2]
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
	if mode == "hold" {
		lock := flock.New(path + ".lock")
		if err := lock.Lock(); err != nil {
			fail(err)
		}
		if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
			fail(err)
		}
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			fail(err)
		}
		if err := lock.Close(); err != nil {
			fail(err)
		}
		os.Exit(0)
	}
	logger, closeLog, err := newLogger(path, slog.LevelInfo, os.Stderr, testPolicy())
	if err != nil {
		fail(err)
	}
	for sequence := 0; sequence < 120; sequence++ {
		logger.Info("record", "worker", worker, "sequence", sequence)
	}
	if err := closeLog(); err != nil {
		fail(err)
	}
	os.Exit(0)
}

func loggingCommand(t *testing.T, mode, path, worker string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLoggingProcess$", "--", mode, path, worker)
	cmd.Env = append(os.Environ(), "CALLUM_LOGGING_HELPER=1")
	return cmd
}

func TestMultipleProcessesWriteAndRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.log")
	const processes = 4
	commands := make([]*exec.Cmd, processes)
	outputs := make([]bytes.Buffer, processes)
	for index := range commands {
		cmd := loggingCommand(t, "write", path, fmt.Sprint(index))
		cmd.Stderr = &outputs[index]
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		commands[index] = cmd
	}
	for index, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("process %d: %v\n%s", index, err, outputs[index].String())
		}
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), "callum*.log"))
	if err != nil || len(files) < 2 {
		t.Fatalf("rotation did not run: %v, %v", files, err)
	}
	seen := make(map[string]bool)
	workers := make(map[string]int)
	for _, name := range files {
		file, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var record struct {
				Message  string `json:"msg"`
				Worker   string `json:"worker"`
				Sequence int    `json:"sequence"`
				PID      int    `json:"pid"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
				_ = file.Close()
				t.Fatalf("mixed or truncated JSON: %s, %v", scanner.Bytes(), err)
			}
			if record.Message != "record" || record.PID <= 0 || record.Sequence < 0 || record.Sequence >= 120 {
				t.Fatal(record)
			}
			if pid, ok := workers[record.Worker]; ok && pid != record.PID {
				t.Fatal("worker PID changed")
			}
			workers[record.Worker] = record.PID
			key := fmt.Sprintf("%s/%d", record.Worker, record.Sequence)
			if seen[key] {
				t.Fatalf("duplicate record: %s", key)
			}
			seen[key] = true
		}
		if err := errors.Join(scanner.Err(), file.Close()); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != processes*120 || len(workers) != processes {
		t.Fatalf("records=%d, workers=%d", len(seen), len(workers))
	}
}

func TestProcessCrashReleasesLogLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.log")
	writer, err := newRotatingWriter(path, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	cmd := loggingCommand(t, "hold", path, "")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && line != "ready\n" {
			err = fmt.Errorf("unexpected ready message %q", line)
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("holder: %v %s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lock holder did not start")
	}
	policy := testPolicy()
	policy.lockTimeout = 50 * time.Millisecond
	if _, err := newRotatingWriter(path, policy); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded lock wait, got %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	recovered, err := newRotatingWriter(path, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.Close() })
	if _, err := recovered.Write([]byte("{\"msg\":\"recovered\"}\n")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path + ".lock")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("lock file must remain in place across process exits")
	}
}

func TestRetentionAndClosedWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "callum.log")
	legacy := filepath.Join(dir, "callum-2000-01-01T00-00-00.000.log")
	unrelated := filepath.Join(dir, "callum-notes.log")
	for _, name := range []string{legacy, unrelated} {
		if err := os.WriteFile(name, []byte("existing"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	policy := testPolicy()
	policy.maxBytes, policy.maxBackups = 128, 2
	writer, err := newRotatingWriter(path, policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	for index := 0; index < 30; index++ {
		if _, err := writer.Write([]byte(strings.Repeat("x", 64) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "callum-*.log"))
	if err != nil || len(files) != 3 { // 两份备份和一份不属于轮转命名规则的文件。
		t.Fatalf("retained files: %v, %v", files, err)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired legacy backup survived: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("closed")); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
}
