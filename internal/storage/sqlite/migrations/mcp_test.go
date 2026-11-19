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
	if cfg != `{"servers":[]}` || version != 7 || revision != 8 {
		t.Fatalf("版本 6 升级未完整迁移或覆盖了其他设置: %q %d %d", cfg, version, revision)
	}
}
