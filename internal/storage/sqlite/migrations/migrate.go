package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Run 在事务中顺序迁移到当前结构，并验证已有数据库的版本和必要约束。
// models 顺序为厂商、模型、工作区、会话、执行、消息、步骤、审批，由存储包传入。
// 调用方持有连接，初始化上下文只用于本次操作，不绑定后续配置查询。
func Run(ctx context.Context, db *gorm.DB, models ...any) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || len(models) != 8 {
		return errors.New("迁移需要数据库及完整的八张表映射")
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
		if err := createModelConfig(tx, models[:2]); err != nil {
			return err
		}
		fallthrough
	case modelConfigVersion:
		if err := validateModelConfig(tx, models[:2]); err != nil {
			return err
		}
		if err := createConversationCore(tx, models[2:7]); err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", conversationVersion)).Error; err != nil {
			return err
		}
		fallthrough
	case conversationVersion:
		if err := validateModelConfig(tx, models[:2]); err != nil {
			return err
		}
		if err := validateConversationCore(tx, models[2:7]); err != nil {
			return err
		}
		if err := createRunApprovals(tx, models[6], models[7]); err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", approvalVersion)).Error; err != nil {
			return err
		}
		fallthrough
	case approvalVersion:
		if err := validateModelConfig(tx, models[:2]); err != nil {
			return err
		}
		if err := validateConversationCore(tx, models[2:7]); err != nil {
			return err
		}
		if err := validateRunApprovals(tx, models[6], models[7]); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持数据库结构版本 %d", version)
	}
	return tx.Commit().Error
}
