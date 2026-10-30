package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"gorm.io/gorm"
)

func migrate(ctx context.Context, db *gorm.DB) (err error) {
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("开始数据库迁移: %w", tx.Error)
	}
	defer func() {
		rollbackErr := tx.Rollback().Error
		if !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	var version int
	if err := tx.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		return err
	}
	switch version {
	case 0:
		// 已有的无版本表不能被自动覆盖或悄悄改写。
		if tx.Migrator().HasTable(&provider.ProviderRecord{}) || tx.Migrator().HasTable(&provider.ModelRecord{}) {
			return errors.New("数据库已有模型配置表但缺少结构版本，需要确认兼容迁移")
		}
		if err := tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(&provider.ProviderRecord{}, &provider.ModelRecord{}); err != nil {
			return fmt.Errorf("创建模型配置表: %w", err)
		}
	case 1:
		if err := migrateUUIDs(tx); err != nil {
			return fmt.Errorf("迁移 UUIDv7 主键: %w", err)
		}
	case 2:
	default:
		return fmt.Errorf("不支持数据库结构版本 %d", version)
	}
	if version != 2 {
		if err := tx.Exec("PRAGMA user_version = 2").Error; err != nil {
			return err
		}
	}
	return tx.Commit().Error
}
