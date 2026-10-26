package sqlite

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteProcess(t *testing.T) {
	if os.Getenv("CALLUM_SQLITE_HELPER") != "1" {
		return
	}
	args := os.Args[len(os.Args)-3:]
	mode, path, worker := args[0], args[1], args[2]
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := Open(ctx, path)
	if err != nil {
		fail(err)
	}
	if mode == "write" {
		for index := 0; index < 80; index++ {
			if _, err := db.ExecContext(ctx, "INSERT INTO process_probe(worker, sequence) VALUES (?, ?)", worker, index); err != nil {
				fail(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		fail(err)
	}
	os.Exit(0)
}

func runSQLiteProcesses(t *testing.T, mode, path string) {
	t.Helper()
	commands := make([]*exec.Cmd, 4)
	outputs := make([]bytes.Buffer, len(commands))
	for index := range commands {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSQLiteProcess$", "--", mode, path, fmt.Sprint(index))
		cmd.Env = append(os.Environ(), "CALLUM_SQLITE_HELPER=1")
		cmd.Stderr = &outputs[index]
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		commands[index] = cmd
	}
	for index, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("SQLite process %d: %v\n%s", index, err, outputs[index].String())
		}
	}
}

func TestMultipleProcessesOpenAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.db")
	// 验证空文件的并发初始化，然后验证共享 WAL 数据库的并发提交。
	runSQLiteProcesses(t, "open", path)
	ctx := context.Background()
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "CREATE TABLE process_probe(worker TEXT NOT NULL, sequence INTEGER NOT NULL, PRIMARY KEY(worker, sequence))"); err != nil {
		t.Fatal(err)
	}
	runSQLiteProcesses(t, "write", path)
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM process_probe").Scan(&count); err != nil || count != 4*80 {
		t.Fatalf("committed rows=%d, error=%v", count, err)
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("database integrity=%s, error=%v", integrity, err)
	}
}
