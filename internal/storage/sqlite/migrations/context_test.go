package migrations_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestLegacyJSONSummarySchemaRequiresNewDataDirectory(t *testing.T) {
	path, pool := fixtureDatabase(t)
	_, err := pool.ExecContext(t.Context(), `CREATE TABLE conversation_context_versions (
		summary TEXT NOT NULL CONSTRAINT "ck_context_summary" CHECK (summary = '' OR (json_valid(summary) AND json_type(summary) = 'object'))
	) STRICT`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.ExecContext(t.Context(), "PRAGMA user_version = 4"); err != nil {
		t.Fatal(err)
	}
	const previous = `{"facts":["保留旧数据"]}`
	if _, err := pool.ExecContext(t.Context(), "INSERT INTO conversation_context_versions VALUES (?)", previous); err != nil {
		t.Fatal(err)
	}
	opened, err := sqlite.Open(t.Context(), path)
	if opened != nil {
		if closeErr := opened.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if err == nil || !strings.Contains(err.Error(), "旧版上下文摘要结构，请使用独立的新数据目录") {
		t.Fatalf("未提示旧摘要结构不受支持: %v", err)
	}
	var saved string
	if err := pool.QueryRowContext(t.Context(), "SELECT summary FROM conversation_context_versions").Scan(&saved); err != nil || saved != previous {
		t.Fatalf("拒绝启动时改写了旧数据: %q %v", saved, err)
	}
}

func TestContextSettingsInitializationAndValidation(t *testing.T) {
	custom := settings.DefaultCompaction()
	custom.Enabled, custom.TokenBudget, custom.RetainTurns = false, 3000, 2
	encoded, err := json.Marshal(custom)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "保留用户设置", value: string(encoded), valid: true},
		{name: "缺少设置"},
		{name: "缺少字段", value: `{}`},
		{name: "旧阈值格式", value: `{"enabled":true,"message_threshold":128,"byte_threshold":4194304,"retain_turns":4}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "data", "callum.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			var row schema.Settings
			if err := db.DB.Where("key=?", settings.CompactionKey).Take(&row).Error; err != nil {
				t.Fatal(err)
			}
			cfg, err := settings.DecodeCompaction(row.Value)
			if err != nil || cfg != settings.DefaultCompaction() || row.Revision != 1 {
				t.Fatalf("新库没有直接初始化 Token 设置: %+v %v", row, err)
			}
			if test.value == "" {
				err = db.DB.Exec("DELETE FROM app_settings WHERE key=?", settings.CompactionKey).Error
			} else {
				err = db.DB.Exec("UPDATE app_settings SET value=?,revision=7 WHERE key=?", test.value, settings.CompactionKey).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			opened, err := sqlite.Open(t.Context(), path)
			if opened != nil {
				t.Cleanup(func() {
					if err := opened.Close(); err != nil {
						t.Error(err)
					}
				})
			}
			if !test.valid {
				if err == nil {
					t.Fatal("打开数据库接受了无效压缩设置")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := opened.DB.Where("key=?", settings.CompactionKey).Take(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.Revision != 7 || string(row.Value) != test.value {
				t.Fatalf("重复打开改写了用户设置: %+v", row)
			}
		})
	}
}
