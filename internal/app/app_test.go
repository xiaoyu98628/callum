package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestRunAndClose(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	application, err := Compose(context.Background(), config.Config{DataDir: t.TempDir()}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	if !strings.Contains(logs.String(), "数据库迁移完成") {
		t.Fatalf("application did not pass its logger to migrations: %s", logs.String())
	}
	var migrations int
	if err := application.db.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&migrations); err != nil || migrations != 1 {
		t.Fatalf("application started before migration: count=%d, error=%v", migrations, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := application.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	if err := application.db.PingContext(context.Background()); err == nil {
		t.Fatal("database remained open after App.Close")
	}
}

func TestComposeMigrationFailurePreservesData(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	db, err := sqlite.Open(t.Context(), cfg.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE workspaces(value TEXT); INSERT INTO workspaces VALUES ('existing');"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	application, err := Compose(t.Context(), cfg, logger)
	if err == nil || application != nil {
		if application != nil {
			_ = application.Close()
		}
		t.Fatal("started with an incompatible existing schema")
	}
	reopened, err := sqlite.Open(t.Context(), cfg.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var value string
	if err := reopened.QueryRowContext(t.Context(), "SELECT value FROM workspaces").Scan(&value); err != nil || value != "existing" {
		t.Fatalf("failed startup changed existing data: value=%q, error=%v", value, err)
	}
	var records int
	if err := reopened.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE name='schema_migrations'").Scan(&records); err != nil || records != 0 {
		t.Fatalf("failed startup partially committed migration: count=%d, error=%v", records, err)
	}
}
