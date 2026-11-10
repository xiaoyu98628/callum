package catalog

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/credentials"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

// MigrateCredentials 先在事务外取得系统主密钥，再以一个事务加密所有旧凭据。
// 迁移失败保留旧值供重新尝试；secure_delete 清除 SQLite 可控制的旧页内容。
func MigrateCredentials(ctx context.Context, db *gorm.DB, cipher credentials.Cipher) (retErr error) {
	if db == nil || cipher == nil {
		return credentials.ErrInvalid
	}
	defer func() {
		if retErr != nil {
			return
		}
		var mode string
		if err := db.WithContext(ctx).Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
			retErr = err
			return
		}
		if mode == "wal" {
			var result struct{ Busy, Log, Checkpointed int }
			if err := db.WithContext(ctx).Raw("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&result).Error; err != nil {
				retErr = err
				return
			}
			if result.Busy != 0 || result.Log > 0 {
				retErr = errors.New("凭据迁移的 WAL 清理尚未完成")
			}
		}
	}()
	// 已有密文时，缺失的系统主密钥不能被自动创建的新密钥替代。
	var after string
	for {
		var existing []schema.Provider
		if err := db.WithContext(ctx).Where("credential_format=1 AND api_key<>'' AND id>?", after).Order("id").Limit(64).Find(&existing).Error; err != nil {
			return err
		}
		for _, row := range existing {
			if _, err := cipher.Open(ctx, row.ID, row.APIKey); err != nil {
				return err
			}
			after = row.ID
		}
		if len(existing) < 64 {
			break
		}
	}
	var count int64
	if err := db.WithContext(ctx).Model(&schema.Provider{}).Where("credential_format=0 AND api_key<>''").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	if err := cipher.Prepare(ctx); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var after string
		for {
			var rows []schema.Provider
			if err := tx.Where("credential_format=0 AND api_key<>'' AND id>?", after).Order("id").Limit(64).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if len(row.APIKey) > credentials.MaxBytes {
					return credentials.ErrInvalid
				}
				value, err := cipher.Seal(ctx, row.ID, row.APIKey)
				if err != nil {
					return err
				}
				result := tx.Model(&schema.Provider{}).Where("id=? AND credential_format=0", row.ID).Updates(map[string]any{"api_key": value, "credential_format": 1})
				if result.Error != nil || result.RowsAffected != 1 {
					return errors.Join(credentials.ErrInvalid, result.Error)
				}
				after = row.ID
			}
			if len(rows) < 64 {
				return nil
			}
		}
	})
}
