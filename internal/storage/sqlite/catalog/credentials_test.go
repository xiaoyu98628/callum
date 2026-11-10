package catalog_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/credentials"
	"github.com/xiaoyu98628/callum/internal/credentials/credentialstest"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

type failingCipher struct {
	credentials.Cipher
	calls int
}

func (f *failingCipher) Seal(ctx context.Context, id, text string) (string, error) {
	f.calls++
	if f.calls == 2 {
		return "", errors.New("受控加密失败")
	}
	return f.Cipher.Seal(ctx, id, text)
}

func TestCredentialMigrationRollsBackAndRemovesControlledPlaintext(t *testing.T) {
	for _, mode := range []string{"delete", "wal"} {
		t.Run(mode, func(t *testing.T) { testCredentialMigration(t, mode) })
	}
}

func testCredentialMigration(t *testing.T, mode string) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.DB.Exec("PRAGMA journal_mode=" + mode).Error; err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"legacy-first-private-value", "legacy-second-private-value"} {
		row := schema.Provider{ID: uuid.Must(uuid.NewV7()).String(), Name: "旧厂商", Protocol: "chatcompletions", BaseURL: "https://example.invalid", APIKey: key}
		if err := db.DB.Select("ID", "Name", "Protocol", "BaseURL", "APIKey", "CredentialFormat").Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.DB.Model(&row).Update("credential_format", 0).Error; err != nil {
			t.Fatal(err)
		}
	}
	v, _ := credentials.New(credentialstest.Keyring{}, path)
	defer v.Close()
	if err := catalog.MigrateCredentials(t.Context(), db.DB, &failingCipher{Cipher: v}); err == nil {
		t.Fatal("未产生受控迁移失败")
	}
	var count int64
	if err := db.DB.Model(&schema.Provider{}).Where("credential_format=0 AND api_key LIKE 'legacy-%'").Count(&count).Error; err != nil || count != 2 {
		t.Fatal("迁移失败未整体回滚", count, err)
	}
	if err := catalog.MigrateCredentials(t.Context(), db.DB, v); err != nil {
		t.Fatal(err)
	}
	if err := catalog.MigrateCredentials(t.Context(), db.DB, v); err != nil {
		t.Fatal("重复迁移失败", err)
	}
	var rows []schema.Provider
	if err := db.DB.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.CredentialFormat != 1 {
			t.Fatal("未记录密文格式")
		}
		value, err := v.Open(t.Context(), row.ID, row.APIKey)
		if err != nil || !bytes.HasPrefix([]byte(value), []byte("legacy-")) {
			t.Fatal("迁移丢失凭据", err)
		}
	}
	// 连接仍打开时检查主文件及 WAL，避免仅在关闭连接后才碰巧清除旧页。
	for _, file := range []string{path, path + "-wal"} {
		raw, err := os.ReadFile(file)
		if errors.Is(err, os.ErrNotExist) && file != path {
			continue
		}
		if err != nil || bytes.Contains(raw, []byte("legacy-first-private-value")) || bytes.Contains(raw, []byte("legacy-second-private-value")) {
			t.Fatal("数据库仍含可控的明文残留", file, err)
		}
	}
}

type lockedExistingCipher struct {
	credentials.Cipher
	reads    int
	prepared bool
}

func (f *lockedExistingCipher) Open(ctx context.Context, id, value string) (string, error) {
	f.reads++
	if f.reads == 2 {
		return "", credentials.ErrUnavailable
	}
	return f.Cipher.Open(ctx, id, value)
}
func (f *lockedExistingCipher) Prepare(context.Context) error {
	f.prepared = true
	return credentials.ErrUnavailable
}

func TestMigrationChecksEveryExistingKeyBeforePreparingLegacyCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault, _ := credentials.New(credentialstest.Keyring{}, path)
	defer vault.Close()
	for n := range 3 {
		row := schema.Provider{ID: uuid.Must(uuid.NewV7()).String(), Name: "迁移保护", Protocol: "chatcompletions", BaseURL: "https://example.invalid", APIKey: "legacy-kept-on-failure"}
		if n < 2 {
			row.APIKey, err = vault.Seal(t.Context(), row.ID, "existing-private-value")
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := db.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if n == 2 {
			if err := db.DB.Model(&row).Update("credential_format", 0).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	locked := &lockedExistingCipher{Cipher: vault}
	if err := catalog.MigrateCredentials(t.Context(), db.DB, locked); !errors.Is(err, credentials.ErrUnavailable) {
		t.Fatal(err)
	}
	if locked.prepared || locked.reads != 2 {
		t.Fatal("已有密钥不可用时尝试创建替代密钥")
	}
	var count int64
	if err := db.DB.Model(&schema.Provider{}).Where("credential_format=0 AND api_key=?", "legacy-kept-on-failure").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("迁移失败改变旧凭据", count, err)
	}
}
