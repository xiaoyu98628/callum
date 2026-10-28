package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/workspace"
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
	if err := application.Run(ctx, io.NopCloser(strings.NewReader("")), &appOutput{}); !errors.Is(err, context.Canceled) {
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

type appOutput struct {
	bytes.Buffer
}

func (w *appOutput) Close() error { return nil }

func TestStartupContextDoesNotCancelWorkspaceRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	application, err := Compose(ctx, config.Config{DataDir: t.TempDir()}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Error(err)
		}
	})
	cancel()
	var output appOutput
	if err := application.Run(t.Context(), io.NopCloser(strings.NewReader(`{"id":"runtime","method":"workspace.list"}`)), &output); err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Result []workspace.Workspace `json:"result"`
		Error  json.RawMessage       `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &reply); err != nil || len(reply.Error) != 0 || reply.Result == nil {
		t.Fatalf("startup cancellation broke runtime ORM queries: %s, %v", output.String(), err)
	}
}

func TestWorkspaceRequestsAndReopen(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	project := t.TempDir()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	application, err := Compose(t.Context(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Error(err)
		}
	})
	var requests bytes.Buffer
	encoder := json.NewEncoder(&requests)
	for index, params := range []any{
		workspace.CreateInput{RootPath: project, Name: "项目"},
		workspace.CreateInput{RootPath: project, Name: "不会覆盖"},
		workspace.ListOptions{},
	} {
		method := "workspace.create"
		if index == 2 {
			method = "workspace.list"
		}
		if err := encoder.Encode(map[string]any{"id": index, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
	}
	var output appOutput
	if err := application.Run(t.Context(), io.NopCloser(&requests), &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var created workspace.Workspace
	for range 2 {
		var reply struct {
			Result workspace.Workspace `json:"result"`
			Error  json.RawMessage     `json:"error"`
		}
		if err := decoder.Decode(&reply); err != nil || len(reply.Error) != 0 {
			t.Fatalf("create response: %+v, %v", reply, err)
		}
		if created.ID == "" {
			created = reply.Result
		} else if reply.Result != created {
			t.Fatalf("duplicate request changed workspace: %+v", reply.Result)
		}
	}
	var list struct {
		Result []workspace.Workspace `json:"result"`
		Error  json.RawMessage       `json:"error"`
	}
	if err := decoder.Decode(&list); err != nil || len(list.Error) != 0 || len(list.Result) != 1 || list.Result[0] != created {
		t.Fatalf("list response: %+v, %v", list, err)
	}
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil || created.RootPath != canonical || created.Name != "项目" {
		t.Fatalf("persisted workspace: %+v, %v", created, err)
	}
	if entries, err := os.ReadDir(project); err != nil || len(entries) != 0 {
		t.Fatalf("registration modified project directory: %v, %v", entries, err)
	}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Compose(t.Context(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	})
	output.Reset()
	if err := reopened.Run(t.Context(), io.NopCloser(strings.NewReader(`{"id":"reopen","method":"workspace.list"}`)), &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &list); err != nil || len(list.Result) != 1 || list.Result[0] != created {
		t.Fatalf("reopened list: %+v, %v", list, err)
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
