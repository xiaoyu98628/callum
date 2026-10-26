package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistenceAndConnectionSettings(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "with # and ?", "callum.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "CREATE TABLE probe (value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO probe(value) VALUES (?)", "saved"); err != nil {
		t.Fatal(err)
	}
	// 关闭空闲连接，确保配置也会应用到随后新建的物理连接。
	db.SetMaxIdleConns(0)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for name, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 5000, "synchronous": 2} {
		var got int
		if err := conn.QueryRowContext(ctx, "PRAGMA "+name).Scan(&got); err != nil || got != want {
			t.Fatalf("%s=%d, want %d, error %v", name, got, want, err)
		}
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var value string
	if err := reopened.QueryRowContext(ctx, "SELECT value FROM probe").Scan(&value); err != nil || value != "saved" {
		t.Fatalf("value=%s, error=%v", value, err)
	}
	var mode string
	if err := reopened.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal mode=%s, error=%v", mode, err)
	}
}

func TestForeignKeysAndCancelledQueries(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		"CREATE TABLE parent (id TEXT PRIMARY KEY)",
		"CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES parent(id) ON DELETE SET NULL)",
		"INSERT INTO parent(id) VALUES ('parent')",
		"INSERT INTO child(id, parent_id) VALUES ('child', 'parent')",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO child(id, parent_id) VALUES ('bad', 'missing')"); err == nil {
		t.Fatal("foreign key violation was accepted")
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM parent WHERE id='parent'"); err != nil {
		t.Fatal(err)
	}
	var parentID sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT parent_id FROM child WHERE id='child'").Scan(&parentID); err != nil || parentID.Valid {
		t.Fatalf("parent_id=%+v, error=%v", parentID, err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	queryCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := db.ExecContext(queryCtx, "SELECT 1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting for connection: %v", err)
	}
}

func TestOpenFailures(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(dir, "cancelled.db")
	if _, err := Open(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled open created a file: %v", err)
	}
	if _, err := Open(context.Background(), "relative.db"); err == nil {
		t.Fatal("accepted a relative path")
	}
	if _, err := Open(context.Background(), dir); err == nil {
		t.Fatal("accepted a directory as database")
	}
}
