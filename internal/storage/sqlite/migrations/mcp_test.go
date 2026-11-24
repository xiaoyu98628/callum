package migrations_test

import (
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestVersionSixAddsCommentedMCPStorage(t *testing.T) {
	path, pool := fixtureDatabase(t)
	ctx := t.Context()
	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"DROP TABLE mcp_credentials",
		"DELETE FROM app_settings WHERE key='mcp'",
		"UPDATE app_settings SET revision=8 WHERE key='general'",
		"PRAGMA user_version=6",
	} {
		if _, err := pool.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	db, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ddl string
	if err := pool.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE name='mcp_credentials'").Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	for _, comment := range []string{"MCP 服务凭据表", "服务标识", "AES-GCM 密文", "Unix 毫秒"} {
		if !strings.Contains(ddl, comment) {
			t.Fatalf("建表语句缺少中文注释 %q", comment)
		}
	}
	var version, revision int
	var cfg string
	if err := pool.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key='mcp'").Scan(&cfg); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRowContext(ctx, "SELECT revision FROM app_settings WHERE key='general'").Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if cfg != `{"servers":[]}` || version != 8 || revision != 8 {
		t.Fatalf("版本 6 升级未完整迁移或覆盖了其他设置: %q %d %d", cfg, version, revision)
	}
}

func TestVersionSevenPreservesMCPScopeDuringUpgrade(t *testing.T) {
	path, pool := fixtureDatabase(t)
	ctx := t.Context()
	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	id := "019a4f39-0000-7000-8000-000000000001"
	value := `{"servers":[{"id":"` + id + `","name":"OAuth","enabled":false,"transport":"streamable_http","auth":"oauth","url":"https://example.invalid/mcp","scope":"read","allowed_tools":[]}]}`
	for _, exec := range []struct {
		query string
		args  []any
	}{
		{"UPDATE app_settings SET value=?,revision=3 WHERE key='mcp'", []any{value}},
		{"INSERT INTO mcp_credentials(server_id,ciphertext,updated_at) VALUES(?,?,1)", []any{id, "sealed-fixture"}},
		{"PRAGMA user_version=7", nil},
	} {
		if _, err := pool.ExecContext(ctx, exec.query, exec.args...); err != nil {
			t.Fatal(err)
		}
	}
	db, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var version, revision int
	var saved, secret string
	if err := pool.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRowContext(ctx, "SELECT value,revision FROM app_settings WHERE key='mcp'").Scan(&saved, &revision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRowContext(ctx, "SELECT ciphertext FROM mcp_credentials WHERE server_id=?", id).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	if version != 8 || saved != value || revision != 3 || secret != "sealed-fixture" {
		t.Fatalf("v7 升级未保留 MCP 设置和凭据: version=%d revision=%d", version, revision)
	}
}
