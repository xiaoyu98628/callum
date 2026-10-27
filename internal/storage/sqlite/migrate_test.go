package sqlite

import (
	"bytes"
	"context"
	"database/sql"
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
	"testing/fstest"
	"time"
)

func migrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func discardMigrationLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestMigrateSchemaAndPreserveData(t *testing.T) {
	db := migrationDB(t)
	ctx := t.Context()
	logger := discardMigrationLogger()
	if err := Migrate(ctx, db, logger); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"workspaces": 7, "conversations": 10} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info(?)", table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s columns=%d, want=%d, error=%v", table, count, want, err)
		}
	}
	for _, statement := range []string{
		"INSERT INTO workspaces(id, name, root_path, created_at, updated_at) VALUES ('workspace', 'Project', '/project', 1, 1)",
		"INSERT INTO conversations(id, workspace_id, model, created_at, updated_at) VALUES ('linked', 'workspace', 'model-a', 1, 1)",
		"INSERT INTO conversations(id, model, created_at, updated_at) VALUES ('standalone', 'model-a', 1, 1)",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	var title string
	var source sql.NullString
	var messageAt, pinnedAt, archivedAt sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT title, title_source, last_message_at, pinned_at, archived_at FROM conversations WHERE id='standalone'").Scan(
		&title, &source, &messageAt, &pinnedAt, &archivedAt,
	); err != nil || title != "" || source.Valid || messageAt.Valid || pinnedAt.Valid || archivedAt.Valid {
		t.Fatalf("new conversation defaults: title=%q, source=%v, times=%v/%v/%v, error=%v", title, source, messageAt, pinnedAt, archivedAt, err)
	}
	for _, statement := range []string{
		"INSERT INTO workspaces(id, name, root_path, created_at, updated_at) VALUES ('duplicate', 'Other', '/project', 1, 1)",
		"INSERT INTO conversations(id, workspace_id, model, created_at, updated_at) VALUES ('orphan', 'missing', 'model-a', 1, 1)",
		"INSERT INTO conversations(id, created_at, updated_at) VALUES ('no-model', 1, 1)",
		"UPDATE conversations SET title_source='unknown' WHERE id='linked'",
	} {
		if _, err := db.ExecContext(ctx, statement); err == nil {
			t.Fatalf("accepted a constraint violation: %s", statement)
		}
	}
	for _, source := range []string{"auto", "manual"} {
		if _, err := db.ExecContext(ctx, "UPDATE conversations SET title='Title', title_source=? WHERE id='linked'", source); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE conversations SET model='model-b' WHERE id='linked'"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM workspaces WHERE id='workspace'"); err != nil {
		t.Fatal(err)
	}
	var workspaceID sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT workspace_id FROM conversations WHERE id='linked'").Scan(&workspaceID); err != nil || workspaceID.Valid {
		t.Fatalf("deleted workspace reference: %v, error=%v", workspaceID, err)
	}
	var before int64
	if err := db.QueryRowContext(ctx, "SELECT applied_at FROM schema_migrations WHERE version=1").Scan(&before); err != nil || before <= 0 {
		t.Fatalf("migration timestamp=%d, error=%v", before, err)
	}
	if err := Migrate(ctx, db, logger); err != nil {
		t.Fatal(err)
	}
	var versions, conversations int
	var after int64
	if err := db.QueryRowContext(ctx, "SELECT count(*), max(applied_at) FROM schema_migrations").Scan(&versions, &after); err != nil || versions != 1 || before != after {
		t.Fatalf("migration replay changed history: versions=%d, before=%d, after=%d, error=%v", versions, before, after, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM conversations").Scan(&conversations); err != nil || conversations != 2 {
		t.Fatalf("conversations=%d, error=%v", conversations, err)
	}
	var model string
	if err := db.QueryRowContext(ctx, "SELECT model FROM conversations WHERE id='linked'").Scan(&model); err != nil || model != "model-b" {
		t.Fatalf("saved model=%q, error=%v", model, err)
	}
}

func TestMigrationUpgradeRollbackAndHistory(t *testing.T) {
	db := migrationDB(t)
	ctx := t.Context()
	logger := discardMigrationLogger()
	source := fstest.MapFS{
		"migrations/001_probe.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE probe(value TEXT NOT NULL); INSERT INTO probe VALUES ('saved');")},
		"migrations/002_extend.sql": &fstest.MapFile{Data: []byte("CREATE TABLE partial(value TEXT); INSERT INTO missing VALUES (1);")},
	}
	if err := migrate(ctx, db, source, logger); err == nil {
		t.Fatal("accepted broken SQL")
	}
	var tables int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("partially committed initial migration: tables=%d, error=%v", tables, err)
	}
	broken := source["migrations/002_extend.sql"]
	delete(source, "migrations/002_extend.sql")
	if err := migrate(ctx, db, source, logger); err != nil {
		t.Fatal(err)
	}
	source["migrations/002_extend.sql"] = broken
	if err := migrate(ctx, db, source, logger); err == nil {
		t.Fatal("accepted broken upgrade")
	}
	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM probe").Scan(&value); err != nil || value != "saved" {
		t.Fatalf("upgrade lost data: value=%q, error=%v", value, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name='partial'").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("upgrade was not rolled back: partial tables=%d, error=%v", tables, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed migration was recorded: count=%d, error=%v", count, err)
	}
	source["migrations/002_extend.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE probe ADD COLUMN extra TEXT;")}
	if err := migrate(ctx, db, source, logger); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT value FROM probe").Scan(&value); err != nil || value != "saved" {
		t.Fatalf("successful upgrade lost data: value=%q, error=%v", value, err)
	}
	original := source["migrations/001_probe.sql"]
	source["migrations/001_probe.sql"] = &fstest.MapFile{Data: append([]byte("-- changed\n"), original.Data...)}
	if err := migrate(ctx, db, source, logger); err == nil || !strings.Contains(err.Error(), "被修改") {
		t.Fatalf("accepted an edited migration: %v", err)
	}
	source["migrations/001_probe.sql"] = original
	delete(source, "migrations/002_extend.sql")
	if err := migrate(ctx, db, source, logger); err == nil || !strings.Contains(err.Error(), "不兼容") {
		t.Fatalf("accepted a database newer than the program: %v", err)
	}
}

func TestMigrationInvalidFiles(t *testing.T) {
	for _, names := range [][]string{
		{}, {"wrong.sql"}, {"000_zero.sql"}, {"001_.sql"}, {"002_gap.sql"}, {"001_a.sql", "001_b.sql"},
	} {
		t.Run(fmt.Sprint(names), func(t *testing.T) {
			source := fstest.MapFS{"migrations": &fstest.MapFile{Mode: os.ModeDir | 0700}}
			for _, name := range names {
				source["migrations/"+name] = &fstest.MapFile{Data: []byte("SELECT 1;")}
			}
			if _, err := loadMigrations(source); err == nil {
				t.Fatal("accepted invalid migration files")
			}
		})
	}
}

func TestMigrationCancellationAndLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger := discardMigrationLogger()
	if err := Migrate(nil, db, logger); err == nil {
		t.Fatal("accepted nil context")
	}
	if err := Migrate(t.Context(), nil, logger); err == nil {
		t.Fatal("accepted nil database")
	}
	if err := Migrate(t.Context(), db, nil); err == nil {
		t.Fatal("accepted nil logger")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Migrate(ctx, db, logger); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	holder, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	tx, err := holder.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	if _, err := tx.ExecContext(t.Context(), "CREATE TABLE held(value TEXT)"); err != nil {
		t.Fatal(err)
	}
	blocked, stop := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer stop()
	if err := Migrate(blocked, db, logger); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock cancellation: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), db, logger); err != nil {
		t.Fatalf("migration did not recover after cancellation: %v", err)
	}
}

func TestMigrationLogs(t *testing.T) {
	db := migrationDB(t)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil)).With("request_id", "migration-test")
	type record struct {
		Level          string `json:"level"`
		Message        string `json:"msg"`
		Component      string `json:"component"`
		RequestID      string `json:"request_id"`
		File           string `json:"file"`
		Version        int64  `json:"version"`
		CurrentVersion int64  `json:"current_version"`
		AppliedCount   int    `json:"applied_count"`
		DurationMS     *int64 `json:"duration_ms"`
	}
	read := func() []record {
		t.Helper()
		var records []record
		decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
		for {
			var current record
			if err := decoder.Decode(&current); errors.Is(err, io.EOF) {
				return records
			} else if err != nil {
				t.Fatal(err)
			}
			if current.Level != "INFO" || current.Component != "sqlite.migration" || current.RequestID != "migration-test" {
				t.Fatalf("missing injected log context: %+v", current)
			}
			records = append(records, current)
		}
	}
	if err := Migrate(t.Context(), db, logger); err != nil {
		t.Fatal(err)
	}
	records := read()
	if len(records) != 3 || records[0].Message != "开始检查数据库迁移" || records[1].Message != "数据库迁移已提交" || records[2].Message != "数据库迁移完成" {
		t.Fatalf("initial migration logs: %+v", records)
	}
	if records[1].Version != 1 || records[1].File != "001_create_workspaces_conversations.sql" {
		t.Fatalf("committed migration details: %+v", records[1])
	}
	summary := records[2]
	if summary.CurrentVersion != 1 || summary.AppliedCount != 1 || summary.DurationMS == nil || *summary.DurationMS < 0 {
		t.Fatalf("migration summary: %+v", summary)
	}
	output.Reset()
	if err := Migrate(t.Context(), db, logger); err != nil {
		t.Fatal(err)
	}
	records = read()
	if len(records) != 2 || records[1].Message != "数据库迁移无需更新" || records[1].AppliedCount != 0 || records[1].CurrentVersion != 1 || records[1].DurationMS == nil {
		t.Fatalf("repeated migration logs: %+v", records)
	}
	output.Reset()
	broken := fstest.MapFS{
		"migrations/001_probe.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE probe(value TEXT);")},
		"migrations/002_broken.sql": &fstest.MapFile{Data: []byte("INSERT INTO missing VALUES (1);")},
	}
	if err := migrate(t.Context(), migrationDB(t), broken, logger); err == nil {
		t.Fatal("accepted broken migration")
	}
	records = read()
	if len(records) != 1 || records[0].Message != "开始检查数据库迁移" {
		t.Fatalf("rolled-back transaction logged success or duplicated the error: %+v", records)
	}
}

// TestMigrationProcess 使用真实子进程竞争同一数据库的迁移写锁。
func TestMigrationProcess(t *testing.T) {
	if os.Getenv("CALLUM_MIGRATION_HELPER") != "1" {
		return
	}
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := Open(ctx, os.Args[len(os.Args)-1])
	if err != nil {
		fail(err)
	}
	logger := discardMigrationLogger()
	for range 3 {
		if err := Migrate(ctx, db, logger); err != nil {
			fail(err)
		}
	}
	if err := db.Close(); err != nil {
		fail(err)
	}
	os.Exit(0)
}

func TestMultipleProcessesMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callum.db")
	commands := make([]*exec.Cmd, 4)
	outputs := make([]bytes.Buffer, len(commands))
	for index := range commands {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMigrationProcess$", "--", path)
		cmd.Env = append(os.Environ(), "CALLUM_MIGRATION_HELPER=1")
		cmd.Stderr = &outputs[index]
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		commands[index] = cmd
	}
	for index, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("migration process %d: %v\n%s", index, err, outputs[index].String())
		}
	}
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent migration history: count=%d, error=%v", count, err)
	}
	var integrity string
	if err := db.QueryRowContext(t.Context(), "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("database integrity=%q, error=%v", integrity, err)
	}
}
