package migrations_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestVersionThreeAddsCredentialFormatWithoutChangingLegacyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	var ddl string
	if err := db.DB.Raw("SELECT sql FROM sqlite_schema WHERE name='model_providers'").Scan(&ddl).Error; err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(ddl, "`credential_format` integer NOT NULL DEFAULT 1,", "", 1)
	legacy = strings.Replace(legacy, ",CONSTRAINT `ck_provider_credential_format` CHECK (credential_format IN (0,1))", "", 1)
	if strings.Contains(legacy, "credential_format") {
		t.Fatal("无法构造完整 v3 表结构", legacy)
	}
	for _, query := range []string{
		"DROP TABLE model_providers",
		legacy,
		"INSERT INTO model_providers(id,name,protocol,base_url,api_key,created_at,updated_at) VALUES ('019a4f39-0000-7000-8000-000000000001','旧厂商','chatcompletions','https://example.invalid','legacy-private-value',1,1)",
		"PRAGMA user_version=3",
	} {
		if err := db.DB.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var value struct {
		APIKey           string
		CredentialFormat int
	}
	if err := upgraded.DB.Table("model_providers").Select("api_key,credential_format").Take(&value).Error; err != nil || value.APIKey != "legacy-private-value" || value.CredentialFormat != 0 {
		t.Fatal("结构升级丢失旧值或错误标记密文", err)
	}
	if err := upgraded.DB.Exec("UPDATE model_providers SET credential_format=2").Error; err == nil {
		t.Fatal("密文格式缺少结构校验")
	}
}
