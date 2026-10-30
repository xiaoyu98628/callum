package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Run 在事务中初始化当前 UUIDv7 结构，并检查已有数据库的结构版本。
// models 必须包含当前完整的 GORM 表映射，由存储包传入，本包不依赖存储实现。
// 调用方持有连接，初始化上下文只用于本次操作，不绑定后续配置查询。
func Run(ctx context.Context, db *gorm.DB, models ...any) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || len(models) == 0 {
		return errors.New("迁移数据库和表映射不能为空")
	}
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
		if err := createModelConfig(tx, models); err != nil {
			return err
		}
		// 具体迁移成功后，在同一事务内记录版本，失败时与表结构一起回滚。
		if err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", modelConfigVersion)).Error; err != nil {
			return err
		}
	case modelConfigVersion:
	default:
		return fmt.Errorf("不支持数据库结构版本 %d", version)
	}
	return tx.Commit().Error
}
