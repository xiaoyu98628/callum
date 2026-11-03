package migrations

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/xiaoyu98628/callum/internal/settings"
)

const settingsVersion = 4

// createAppSettings 创建设置组并初始化默认执行设置；不覆盖已有版本中的用户配置。
// 旧执行的快照保留 NULL，不把当前配置推断成历史执行当时的设置。
func createAppSettings(tx *gorm.DB, run, mapping any) error {
	if tx.Migrator().HasTable(mapping) {
		return errors.New("已有应用设置表与待执行迁移冲突")
	}
	if err := tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(mapping); err != nil {
		return err
	}
	if !tx.Migrator().HasColumn(run, "SettingsSnapshot") {
		if err := addSettingsSnapshot(tx, run); err != nil {
			return err
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	value, err := json.Marshal(settings.DefaultExecution())
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	return tx.Table("app_settings").Create(map[string]any{"id": id.String(), "key": settings.ExecutionKey, "value": string(value), "revision": 1, "created_at": now, "updated_at": now}).Error
}

func addSettingsSnapshot(tx *gorm.DB, run any) error {
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(run); err != nil {
		return err
	}
	field := statement.Schema.LookUpField("SettingsSnapshot")
	check, ok := statement.Schema.ParseCheckConstraints()["ck_run_settings"]
	if field == nil || !ok {
		return errors.New("执行快照映射缺少字段或检查约束")
	}
	// GORM AddColumn 不附加 CHECK；从同一映射生成带约束的列，避免重建有外键的执行表。
	return tx.Exec("ALTER TABLE ? ADD COLUMN ? ? CONSTRAINT ? CHECK (?)",
		clause.Table{Name: statement.Schema.Table}, clause.Column{Name: field.DBName}, tx.Migrator().FullDataTypeOf(field),
		clause.Column{Name: check.Name}, clause.Expr{SQL: check.Constraint}).Error
}

func validateAppSettings(tx *gorm.DB, run, mapping any) error {
	if err := validateCoreTable(tx, mapping); err != nil {
		return err
	}
	if err := validateUniqueIndex(tx, "app_settings", "uq_settings_key", []string{"key"}); err != nil {
		return err
	}
	return validateCoreTable(tx, run)
}
