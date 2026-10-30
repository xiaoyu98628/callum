package migrations

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const modelConfigVersion = 2

// createModelConfig 创建 model_providers 和 models 的 UUIDv7 结构，包括外键、索引和检查约束。
// 表映射由 SQLite 存储包传入；执行使用 Run 提供的事务，不单独提交或关闭连接。
func createModelConfig(tx *gorm.DB, models []any) error {
	// 已有的无版本表不能被自动覆盖或悄悄改写。
	for _, model := range models {
		if tx.Migrator().HasTable(model) {
			return errors.New("数据库已有模型配置表但缺少结构版本，无法安全初始化")
		}
	}
	if err := tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(models...); err != nil {
		return fmt.Errorf("创建模型配置表: %w", err)
	}
	return nil
}
