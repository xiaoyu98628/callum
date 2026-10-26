package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONOutputAndLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "callum.log")
	var stderr bytes.Buffer
	logger, closeLog, err := New(path, slog.LevelInfo, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLog() })
	logger.Debug("hidden")
	logger.With("component", "database").Info("connected", "request_id", "req-1")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range [][]byte{data, stderr.Bytes()} {
		var record map[string]any
		if err := json.Unmarshal(output, &record); err != nil {
			t.Fatalf("invalid JSON: %q, %v", output, err)
		}
		if record["msg"] != "connected" || record["component"] != "database" || record["request_id"] != "req-1" {
			t.Fatal(record)
		}
	}
}

func TestRotationAndInvalidPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "callum.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate((10 << 20) - 128); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	logger, closeLog, err := New(path, slog.LevelInfo, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLog() })
	logger.Info("rotate", "value", strings.Repeat("x", 1024))
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(dir, "callum-*.log"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("rotation backups: %v, %v", backups, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !json.Valid(bytes.TrimSpace(data)) {
		t.Fatalf("current log: %q, %v", data, err)
	}
	if _, _, err := New(dir, slog.LevelInfo, io.Discard); err == nil {
		t.Fatal("accepted a directory as log file")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("log initialization changed the existing directory: %v", err)
	}
}

func TestFileWriteFailureIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.log")
	var stderr bytes.Buffer
	logger, closeLog, err := New(path, slog.LevelInfo, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	logger = logger.With("component", "agent").WithGroup("request").With("id", "req-1")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	logger.Info("after close")
	decoder := json.NewDecoder(&stderr)
	var original, failure map[string]any
	if err := decoder.Decode(&original); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if original["msg"] != "after close" || failure["msg"] != "写入日志文件失败" || failure["error"] == nil {
		t.Fatalf("original=%v, failure=%v", original, failure)
	}
	if original["pid"] != float64(os.Getpid()) || failure["pid"] != float64(os.Getpid()) {
		t.Fatalf("missing process identity: original=%v, failure=%v", original, failure)
	}
}
